package parser

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	reSlideNum  = regexp.MustCompile(`ppt/slides/slide(\d+)\.xml$`)
	reSlideRels = regexp.MustCompile(`ppt/slides/_rels/slide(\d+)\.xml.rels$`)
	reRelTarget = regexp.MustCompile(`Target="([^"]+)"`)
)

func extractDOCX(path string, s *extractSettings) (*Result, error) {
	text, err := readZipEntryText(path, "word/document.xml", extractWordML)
	if err != nil {
		return nil, fmt.Errorf("docx: %w", err)
	}
	text = normalizeText(text)
	if text == "" && s.ocrEnabled() {
		ocrText, ocrErr := ocrOOXMLMedia(s, path, "word/")
		if ocrErr != nil {
			return nil, fmt.Errorf("docx OCR: %w", ocrErr)
		}
		text = normalizeText(ocrText)
	}
	if text == "" {
		if s.ocrEnabled() {
			return nil, permanentf("docx 未提取到文本（含 OCR）")
		}
		return nil, permanentf("docx 未提取到文本")
	}
	return &Result{
		Text:        text,
		ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Format:      "docx",
	}, nil
}

func extractPPTX(path string, s *extractSettings) (*Result, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open pptx: %w", err)
	}
	defer zr.Close()

	type slideFile struct {
		num  int
		file *zip.File
	}
	var slides []slideFile
	for _, f := range zr.File {
		m := reSlideNum.FindStringSubmatch(filepath.ToSlash(f.Name))
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		slides = append(slides, slideFile{num: n, file: f})
	}
	sort.Slice(slides, func(i, j int) bool { return slides[i].num < slides[j].num })
	if len(slides) == 0 {
		return nil, permanentf("pptx 未找到幻灯片")
	}

	rels := map[int]*zip.File{}
	for _, f := range zr.File {
		m := reSlideRels.FindStringSubmatch(filepath.ToSlash(f.Name))
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		rels[n] = f
	}

	var b strings.Builder
	for _, sl := range slides {
		rc, err := sl.file.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			continue
		}
		text := strings.TrimSpace(extractDrawingML(body))
		if text == "" && s.ocrEnabled() {
			if rel := rels[sl.num]; rel != nil {
				if ocrText, ocrErr := ocrPPTXSlideImages(s, zr, rel); ocrErr == nil {
					text = strings.TrimSpace(ocrText)
				}
			}
		}
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## 第 %d 页\n\n%s", sl.num, text)
	}
	plain := normalizeText(b.String())
	if plain == "" {
		if s.ocrEnabled() {
			return nil, permanentf("pptx 未提取到文本（含 OCR）")
		}
		return nil, permanentf("pptx 未提取到文本")
	}
	return &Result{
		Text:        plain,
		ContentType: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		Format:      "pptx",
		Pages:       len(slides),
	}, nil
}

func readZipEntryText(path, entry string, extract func([]byte) string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	var target *zip.File
	for _, f := range zr.File {
		if filepath.ToSlash(f.Name) == entry {
			target = f
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("missing %s", entry)
	}
	rc, err := target.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}
	return extract(body), nil
}

func extractWordML(data []byte) string {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var (
		b     strings.Builder
		inT   bool
		depth int // tc nesting
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch localName(t.Name) {
			case "t":
				inT = true
			case "tab":
				b.WriteByte('\t')
			case "br", "cr":
				b.WriteByte('\n')
			case "tc":
				depth++
			}
		case xml.EndElement:
			name := localName(t.Name)
			switch name {
			case "t":
				inT = false
			case "p":
				b.WriteByte('\n')
			case "tc":
				if depth > 0 {
					depth--
				}
				b.WriteByte('\t')
			case "tr":
				b.WriteByte('\n')
			}
		case xml.CharData:
			if inT {
				b.Write(t)
			}
		}
	}
	return b.String()
}

func extractDrawingML(data []byte) string {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var (
		b   strings.Builder
		inT bool
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch localName(t.Name) {
			case "t":
				inT = true
			case "br":
				b.WriteByte('\n')
			}
		case xml.EndElement:
			switch localName(t.Name) {
			case "t":
				inT = false
			case "p":
				b.WriteByte('\n')
			}
		case xml.CharData:
			if inT {
				b.Write(t)
			}
		}
	}
	return b.String()
}

func localName(n xml.Name) string {
	return n.Local
}

func ocrOOXMLMedia(s *extractSettings, path, prefix string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	var names []string
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if strings.HasPrefix(name, prefix) && strings.Contains(name, "/media/") && isImageZipEntry(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return ocrZipImages(s, zr, names)
}

func ocrPPTXSlideImages(s *extractSettings, zr *zip.ReadCloser, rel *zip.File) (string, error) {
	rc, err := rel.Open()
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return "", err
	}
	base := filepath.ToSlash(filepath.Dir(rel.Name))
	var names []string
	seen := map[string]bool{}
	for _, m := range reRelTarget.FindAllSubmatch(body, -1) {
		target := filepath.ToSlash(string(m[1]))
		if strings.HasPrefix(target, "/") {
			target = strings.TrimPrefix(target, "/")
		} else {
			target = filepath.ToSlash(filepath.Clean(filepath.Join(base, target)))
		}
		if !isImageZipEntry(target) || seen[target] {
			continue
		}
		seen[target] = true
		names = append(names, target)
	}
	return ocrZipImages(s, zr, names)
}

func ocrZipImages(s *extractSettings, zr *zip.ReadCloser, names []string) (string, error) {
	if len(names) == 0 || !s.ocrEnabled() {
		return "", nil
	}
	if _, err := s.recognizer(); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "rag-ocr-zip-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	files := zipFileIndex(zr)
	var b strings.Builder
	var firstErr error
	for _, name := range names {
		f := files[name]
		if f == nil {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		text, err := ocrImageBytes(s.ctx, s, dir, name, data)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(text)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", firstErr
	}
	return out, nil
}

func zipFileIndex(zr *zip.ReadCloser) map[string]*zip.File {
	out := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		out[filepath.ToSlash(f.Name)] = f
	}
	return out
}
