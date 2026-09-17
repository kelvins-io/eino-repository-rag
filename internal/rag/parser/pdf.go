package parser

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	gopdf "github.com/Detective-XH/gopdf"

	"github.com/kelvins-io/eino-repository-rag/internal/logger"
)

type pdfPageWork struct {
	num     int
	text    string
	needOCR bool
	ocrOK   bool
	signal  gopdf.ExtractionSignal
}

func extractPDF(path string, s *extractSettings) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	data = rewriteUTF16CMaps(data)

	r, err := gopdf.OpenBytes(data)
	if err != nil {
		return nil, permanentf("open pdf: %v", err)
	}

	totalPage := r.NumPage()
	if totalPage == 0 {
		return nil, permanentf("pdf 没有页面")
	}

	pages := make([]pdfPageWork, 0, totalPage)
	extractedPages := 0
	imageOnly := 0
	degraded := 0
	empty := 0
	for i := 1; i <= totalPage; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			empty++
			pages = append(pages, pdfPageWork{num: i, needOCR: s.ocrEnabled(), signal: gopdf.SignalEmpty})
			continue
		}
		text := sanitizeExtractedText(extractPDFPage(page))
		if strings.TrimSpace(text) == "" {
			sig := page.ExtractionSignal()
			work := pdfPageWork{num: i, signal: sig}
			switch sig {
			case gopdf.SignalImageOnly:
				imageOnly++
				work.needOCR = s.ocrEnabled()
			case gopdf.SignalDegraded:
				degraded++
			default:
				empty++
				work.needOCR = s.ocrEnabled()
			}
			pages = append(pages, work)
			continue
		}
		extractedPages++
		pages = append(pages, pdfPageWork{num: i, text: text, signal: gopdf.SignalText})
	}

	plain := joinPDFPages(pages)
	needOCR := false
	for _, p := range pages {
		if p.needOCR {
			needOCR = true
			break
		}
	}
	var ocrErr error
	if s.ocrEnabled() && (needOCR || looksGarbledPDF(plain, path)) {
		if looksGarbledPDF(plain, path) {
			for i := range pages {
				pages[i].needOCR = true
				pages[i].text = ""
			}
		}
		if ocrErr = ocrPDFPages(s.ctx, path, pages, s); ocrErr != nil {
			logger.S().Warnf("[parser] pdf ocr partial/fail path=%s err=%v", path, ocrErr)
		}
		extractedPages, imageOnly, empty, degraded = 0, 0, 0, 0
		ocrPages := 0
		for _, p := range pages {
			if p.ocrOK {
				ocrPages++
			}
			if strings.TrimSpace(p.text) != "" {
				extractedPages++
				continue
			}
			switch p.signal {
			case gopdf.SignalImageOnly:
				imageOnly++
			case gopdf.SignalDegraded:
				degraded++
			default:
				empty++
			}
		}
		plain = joinPDFPages(pages)
		logger.S().Infof("[parser] pdf ocr pages=%d ocr_ok=%d", totalPage, ocrPages)
	}

	plain = normalizeText(plain)
	if plain == "" {
		if ocrErr != nil && extractedPages == 0 {
			return nil, fmt.Errorf("pdf OCR 失败：共 %d 页: %w", totalPage, ocrErr)
		}
		if !s.ocrEnabled() && imageOnly > 0 && extractedPages == 0 {
			return nil, permanentf("pdf 未提取到文本：共 %d 页，其中扫描/图片页 %d 页，请开启 rag.ocr.enabled 并启动 OCR 服务（docker compose up -d ocr）", totalPage, imageOnly)
		}
		if s.ocrEnabled() && imageOnly > 0 && extractedPages == 0 {
			return nil, permanentf("pdf OCR 未识别到文本：共 %d 页，其中扫描/图片页 %d 页", totalPage, imageOnly)
		}
		if degraded > 0 && extractedPages == 0 {
			return nil, permanentf("pdf 文本提取失败（%d 页内容流损坏）", degraded)
		}
		return nil, permanentf("pdf 未提取到文本")
	}
	if looksGarbledPDF(plain, path) {
		return nil, permanentf("pdf 文本解码失败（字体/CMap 未正确映射为 Unicode）。请尝试导出为 Word/纯文本后再导入，或使用带 ToUnicode 的 PDF")
	}

	logger.S().Infof("[parser] pdf pages=%d extracted=%d image_only=%d degraded=%d empty=%d",
		totalPage, extractedPages, imageOnly, degraded, empty)

	return &Result{
		Text:        plain,
		ContentType: "application/pdf",
		Format:      "pdf",
		Pages:       totalPage,
	}, nil
}

func joinPDFPages(pages []pdfPageWork) string {
	var b strings.Builder
	for _, p := range pages {
		text := strings.TrimSpace(p.text)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## 第 %d 页\n\n%s", p.num, text)
	}
	return b.String()
}

func extractPDFPage(page gopdf.Page) string {
	if lines, err := page.Lines(); err == nil && len(lines) > 0 {
		var b strings.Builder
		for _, line := range lines {
			s := strings.TrimSpace(line.S)
			if s == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(s)
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	text, err := page.GetPlainText(nil)
	if err != nil {
		return ""
	}
	return text
}

// rewriteUTF16CMaps 将 Uni*-UTF16-* 预定义 CMap 改写为同长度的 Uni*-UCS2-*。
// gopdf v0.8.7 只识别 UCS2；UTF16 对 BMP 汉字与 UCS-2 相同，但未知编码会回退成单字节 PDFDoc，导致乱码。
// 必须等长替换，避免破坏 xref 偏移。
func rewriteUTF16CMaps(data []byte) []byte {
	repls := [][2]string{
		{"UniGB-UTF16-H", "UniGB-UCS2-H "},
		{"UniGB-UTF16-V", "UniGB-UCS2-V "},
		{"UniCNS-UTF16-H", "UniCNS-UCS2-H "},
		{"UniCNS-UTF16-V", "UniCNS-UCS2-V "},
		{"UniJIS-UTF16-H", "UniJIS-UCS2-H "},
		{"UniJIS-UTF16-V", "UniJIS-UCS2-V "},
		{"UniKS-UTF16-H", "UniKS-UCS2-H "},
		{"UniKS-UTF16-V", "UniKS-UCS2-V "},
	}
	out := data
	copied := false
	for _, pair := range repls {
		old, neu := []byte(pair[0]), []byte(pair[1])
		if len(old) != len(neu) || !bytes.Contains(out, old) {
			continue
		}
		if !copied {
			out = bytes.Clone(data)
			copied = true
		}
		out = bytes.ReplaceAll(out, old, neu)
	}
	return out
}

func sanitizeExtractedText(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t' || r == '\r':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			continue
		case unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cs, r):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// looksGarbledPDF 判断抽取结果是否像未解码的字体字节（中文 PDF 常见）
func looksGarbledPDF(text, path string) bool {
	sample := text
	if utf8.RuneCountInString(sample) > 4000 {
		sample = string([]rune(sample)[:4000])
	}
	var (
		total    int
		han      int
		letters  int
		controls int
		private  int
	)
	for _, r := range sample {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			letters++
		case r < 0x20:
			controls++
		case r >= 0xE000 && r <= 0xF8FF:
			private++
		}
	}
	if total == 0 {
		return true
	}
	if controls*20 > total || private*5 > total {
		return true
	}
	// 文件名含中文，正文几乎没有汉字：典型的 CJK 字体未映射
	if pathHasHan(path) && han*50 < total && total > 80 {
		return true
	}
	return false
}

func pathHasHan(path string) bool {
	for _, r := range path {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
