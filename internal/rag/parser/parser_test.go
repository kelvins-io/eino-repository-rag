package parser

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectFormat(t *testing.T) {
	cases := map[string]string{
		"a.PDF":  "pdf",
		"a.docx": "docx",
		"a.xlsx": "xlsx",
		"a.pptx": "pptx",
		"a.html": "html",
		"a.md":   "markdown",
		"a.txt":  "text",
		"a.doc":  "doc",
		"a.png":  "image",
		"a.jpg":  "image",
	}
	for name, want := range cases {
		if got := detectFormat(name, ""); got != want {
			t.Fatalf("%s: got %s want %s", name, got, want)
		}
	}
	if got := detectFormat("x.bin", "application/pdf"); got != "pdf" {
		t.Fatalf("content-type detect got %s", got)
	}
}

func TestExtractPlainAndHTML(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(txtPath, []byte("你好企业知识库\n第二行"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ExtractFile(txtPath, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "企业知识库") {
		t.Fatalf("text=%q", res.Text)
	}

	htmlPath := filepath.Join(dir, "a.html")
	html := `<html><head><script>bad()</script><style>.x{}</style></head><body><h1>标题</h1><p>正文内容</p></body></html>`
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = ExtractFile(htmlPath, "text/html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Text, "bad()") {
		t.Fatalf("script leaked: %q", res.Text)
	}
	if !strings.Contains(res.Text, "正文内容") {
		t.Fatalf("html text=%q", res.Text)
	}
}

func TestExtractDOCX(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.docx")
	if err := writeMinimalDOCX(path, "合规制度正文"); err != nil {
		t.Fatal(err)
	}
	res, err := ExtractFile(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Format != "docx" {
		t.Fatalf("format=%s", res.Format)
	}
	if !strings.Contains(res.Text, "合规制度正文") {
		t.Fatalf("docx text=%q", res.Text)
	}
}

func TestSanitizeAndGarbledPDF(t *testing.T) {
	raw := "A\u0001B\u0002你好"
	got := sanitizeExtractedText(raw)
	if got != "AB你好" {
		t.Fatalf("sanitize=%q", got)
	}
	garbled := strings.Repeat("\u0001HBUFXBZ 5PLFO ©ÖG>Â", 20)
	if !looksGarbledPDF(garbled, "AI网关优化方案.pdf") {
		t.Fatal("expected garbled detection for CJK-named PDF")
	}
	clean := strings.Repeat("网关首包延迟优化方案 TTFT 需要缩短 Prompt 处理时间。", 5)
	if looksGarbledPDF(clean, "AI网关优化方案.pdf") {
		t.Fatal("clean CJK text should not be garbled")
	}
}

func TestRewriteUTF16CMaps(t *testing.T) {
	pairs := [][2]string{
		{"UniGB-UTF16-H", "UniGB-UCS2-H "},
		{"UniGB-UTF16-V", "UniGB-UCS2-V "},
		{"UniCNS-UTF16-H", "UniCNS-UCS2-H "},
		{"UniJIS-UTF16-H", "UniJIS-UCS2-H "},
		{"UniKS-UTF16-H", "UniKS-UCS2-H "},
	}
	for _, p := range pairs {
		if len(p[0]) != len(p[1]) {
			t.Fatalf("rewrite must keep length: %q (%d) vs %q (%d)", p[0], len(p[0]), p[1], len(p[1]))
		}
	}
	in := []byte("<</Type/Font/Encoding/UniGB-UTF16-H/BaseFont/SimSun>>")
	out := rewriteUTF16CMaps(in)
	if len(out) != len(in) {
		t.Fatalf("xref length changed %d -> %d", len(in), len(out))
	}
	if bytes.Contains(out, []byte("UniGB-UTF16-H")) {
		t.Fatalf("utf16 cmap not rewritten: %s", out)
	}
	if !bytes.Contains(out, []byte("UniGB-UCS2-H")) {
		t.Fatalf("ucs2 cmap missing: %s", out)
	}
}

func TestPermanentExtractErrors(t *testing.T) {
	_, err := ExtractFile("old.doc", "application/msword")
	if err == nil || !IsPermanent(err) {
		t.Fatalf("expected permanent .doc error, got %v", err)
	}
}

func TestExtractLocalFailedPDFs(t *testing.T) {
	root := filepath.Join("..", "..", "..", "storage", "uploads", "1", "9")
	cases := []struct {
		name      string
		wantOK    bool
		permanent bool
	}{
		{"1789618268553295000_短线交易大师：工具和策略(高清).pdf", true, false},
		{"1789618268634323000_通向金融王国的自由之路.pdf", false, true},
		{"1789618268670966000_笑傲股市.pdf", false, true},
	}
	ran := false
	for _, tc := range cases {
		path := filepath.Join(root, tc.name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		ran = true
		res, err := ExtractFile(path, "application/pdf")
		if tc.wantOK {
			if err != nil {
				t.Fatalf("%s: unexpected err=%v", tc.name, err)
			}
			if !strings.Contains(res.Text, "手续费") && !strings.Contains(res.Text, "交易") {
				t.Fatalf("%s: extracted text looks wrong: %q", tc.name, truncateRunes(res.Text, 80))
			}
			continue
		}
		if err == nil {
			t.Fatalf("%s: expected extract error", tc.name)
		}
		if tc.permanent && !IsPermanent(err) {
			t.Fatalf("%s: expected permanent err, got %v", tc.name, err)
		}
	}
	if !ran {
		t.Skip("local upload samples not present")
	}
}

func TestExtractDOCRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.doc")
	if err := os.WriteFile(path, []byte("binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ExtractFile(path, "application/msword")
	if err == nil {
		t.Fatal("expected error for .doc")
	}
}

func writeMinimalDOCX(path, text string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>` + text + `</w:t></w:r></w:p>
  </w:body>
</w:document>`,
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(body)); err != nil {
			return err
		}
	}
	return zw.Close()
}
