package parser

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	gopdf "github.com/Detective-XH/gopdf"
)

func extractPDF(path string) (*Result, error) {
	f, r, err := gopdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pdf: %w", err)
	}
	defer f.Close()

	totalPage := r.NumPage()
	if totalPage == 0 {
		return nil, fmt.Errorf("pdf has no pages")
	}

	ctx := context.Background()
	var b strings.Builder
	extractedPages := 0
	for i := 1; i <= totalPage; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text := extractPDFPage(page)
		text = sanitizeExtractedText(text)
		if text == "" {
			continue
		}
		extractedPages++
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## 第 %d 页\n\n%s", i, text)
	}

	plain := normalizeText(b.String())
	if plain == "" {
		reader, err := r.GetPlainText(ctx)
		if err != nil {
			return nil, fmt.Errorf("pdf extract text empty (可能是扫描件/加密PDF，暂不支持 OCR): %w", err)
		}
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(reader); err != nil {
			return nil, fmt.Errorf("read pdf plain text: %w", err)
		}
		plain = normalizeText(sanitizeExtractedText(buf.String()))
	}
	if plain == "" {
		return nil, fmt.Errorf("pdf 未提取到文本（可能是扫描件，暂不支持 OCR）")
	}
	if looksGarbledPDF(plain, path) {
		return nil, fmt.Errorf("pdf 文本解码失败（字体/CMap 未正确映射为 Unicode）。请尝试导出为 Word/纯文本后再导入，或使用带 ToUnicode 的 PDF")
	}

	return &Result{
		Text:        plain,
		ContentType: "application/pdf",
		Format:      "pdf",
		Pages:       totalPage,
	}, nil
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
			// PDF 未解码码点常以 NUL/SOH 等形式出现，直接丢弃
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
