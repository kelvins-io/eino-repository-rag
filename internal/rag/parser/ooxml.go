package parser

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var reSlideNum = regexp.MustCompile(`ppt/slides/slide(\d+)\.xml$`)

func extractDOCX(path string) (*Result, error) {
	text, err := readZipEntryText(path, "word/document.xml", extractWordML)
	if err != nil {
		return nil, fmt.Errorf("docx: %w", err)
	}
	text = normalizeText(text)
	if text == "" {
		return nil, permanentf("docx 未提取到文本")
	}
	return &Result{
		Text:        text,
		ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Format:      "docx",
	}, nil
}

func extractPPTX(path string) (*Result, error) {
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

	var b strings.Builder
	for _, s := range slides {
		rc, err := s.file.Open()
		if err != nil {
			continue
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			continue
		}
		text := strings.TrimSpace(extractDrawingML(body))
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## 第 %d 页\n\n%s", s.num, text)
	}
	plain := normalizeText(b.String())
	if plain == "" {
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
