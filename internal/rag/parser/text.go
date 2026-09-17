package parser

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func extractHTMLFile(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text, err := extractHTMLBytes(data)
	if err != nil {
		return nil, err
	}
	text = normalizeText(text)
	if text == "" {
		return nil, permanentf("html 未提取到文本")
	}
	return &Result{
		Text:        text,
		ContentType: "text/html",
		Format:      "html",
	}, nil
}

func extractHTMLBytes(data []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("parse html: %w", err)
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "script", "style", "noscript":
				return
			case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "section", "article":
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
					b.WriteByte('\n')
				}
			}
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") && !strings.HasSuffix(b.String(), " ") {
					b.WriteByte(' ')
				}
				b.WriteString(t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return b.String(), nil
}

func extractPlainFile(path, format string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text, enc := decodeText(data)
	text = normalizeText(text)
	if text == "" {
		return nil, fmt.Errorf("%s 文件为空", format)
	}
	ct := "text/plain"
	switch format {
	case "markdown":
		ct = "text/markdown"
	case "csv":
		ct = "text/csv"
	case "json":
		ct = "application/json"
	case "html":
		ct = "text/html"
	}
	_ = enc
	return &Result{
		Text:        text,
		ContentType: ct,
		Format:      format,
	}, nil
}

func decodeText(data []byte) (string, string) {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return string(data[3:]), "utf-8-bom"
	}
	if utf8.Valid(data) {
		return string(data), "utf-8"
	}
	// 企业内常见 GBK/GB18030
	readers := []struct {
		name string
		t    transform.Transformer
	}{
		{"gb18030", simplifiedchinese.GB18030.NewDecoder()},
		{"gbk", simplifiedchinese.GBK.NewDecoder()},
	}
	for _, r := range readers {
		out, err := io.ReadAll(transform.NewReader(bytes.NewReader(data), r.t))
		if err == nil && utf8.Valid(out) && len(bytes.TrimSpace(out)) > 0 {
			return string(out), r.name
		}
	}
	return string(data), "latin1-fallback"
}
