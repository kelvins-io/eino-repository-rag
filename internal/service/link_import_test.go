package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseDocumentLink(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		fileName string
		typeExt  string
		invalid  bool
	}{
		{name: "pdf with query", raw: "https://example.com/docs/a.pdf?token=abc&x=1", fileName: "a.pdf"},
		{name: "uppercase ext and scheme", raw: "HTTPS://Example.com/A.PDF?X=1", fileName: "A.PDF"},
		{name: "encoded name", raw: "https://example.com/%E6%8A%A5%E5%91%8A.md?download=1", fileName: "报告.md"},
		{name: "html query", raw: "http://example.com/page.html?id=1#top", fileName: "page.html"},
		{name: "image", raw: "https://cdn.example.com/a/b.png?w=100", fileName: "b.png"},
		{name: "port", raw: "https://example.com:8443/file.docx?sig=1", fileName: "file.docx"},
		{name: "query looks like other type", raw: "https://example.com/a.pdf?file=b.exe", fileName: "a.pdf"},
		{name: "extension only in query", raw: "https://example.com/download?file=a.pdf", invalid: true},
		{name: "unsupported", raw: "https://example.com/a.exe?x=1", typeExt: ".exe"},
		{name: "legacy doc", raw: "https://example.com/a.doc", typeExt: ".doc"},
		{name: "ftp", raw: "ftp://example.com/a.pdf", invalid: true},
		{name: "no host", raw: "https:///a.pdf", invalid: true},
		{name: "space", raw: "https://example.com/a file.pdf", invalid: true},
		{name: "empty", raw: "  ", invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDocumentLink(tt.raw)
			if tt.typeExt != "" {
				var typeErr *linkTypeError
				if !errors.As(err, &typeErr) || typeErr.ext != tt.typeExt {
					t.Fatalf("got name=%q err=%v", got, err)
				}
				return
			}
			if tt.invalid {
				if err == nil {
					t.Fatalf("expected invalid, got %q", got)
				}
				var typeErr *linkTypeError
				if errors.As(err, &typeErr) {
					t.Fatalf("unexpected type error %s", typeErr.ext)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.fileName {
				t.Fatalf("file name=%q want %q", got, tt.fileName)
			}
		})
	}
}

func TestValidateDocumentLinks(t *testing.T) {
	ok, err := validateDocumentLinks([]LinkDocumentItem{
		{URL: "https://example.com/a.pdf?token=1", Title: " 年报 "},
		{URL: "https://example.com/b.docx", Title: "说明"},
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(ok) != 2 || ok[0].Title != "年报" || ok[0].FileName != "a.pdf" || ok[0].URL != "https://example.com/a.pdf?token=1" {
		t.Fatalf("prepared=%+v", ok)
	}

	if _, err := validateDocumentLinks(nil, 20); err == nil || !strings.Contains(err.Error(), "缺少") {
		t.Fatalf("empty: %v", err)
	}
	items := make([]LinkDocumentItem, 3)
	for i := range items {
		items[i] = LinkDocumentItem{URL: "https://example.com/a.pdf", Title: "t"}
	}
	if _, err := validateDocumentLinks(items, 2); err == nil || !strings.Contains(err.Error(), "不能超过") {
		t.Fatalf("count: %v", err)
	}
	if _, err := validateDocumentLinks([]LinkDocumentItem{{URL: "https://example.com/a.pdf", Title: ""}}, 20); err == nil || !strings.Contains(err.Error(), "标题不能为空") {
		t.Fatalf("title: %v", err)
	}
	longTitle := strings.Repeat("题", 200)
	if _, err := validateDocumentLinks([]LinkDocumentItem{{URL: "https://example.com/a.pdf", Title: longTitle}}, 20); err == nil || !strings.Contains(err.Error(), "标题过长") {
		t.Fatalf("long title: %v", err)
	}
	if _, err := validateDocumentLinks([]LinkDocumentItem{
		{URL: "https://example.com/a.pdf?x=1", Title: "a"},
		{URL: "https://example.com/a.pdf?x=1", Title: "b"},
	}, 20); err == nil || !strings.Contains(err.Error(), "不能重复提交") {
		t.Fatalf("dup: %v", err)
	}
	if _, err := validateDocumentLinks([]LinkDocumentItem{{URL: "https://example.com/a.exe", Title: "a"}}, 20); err == nil || !strings.Contains(err.Error(), "文档类型无效") {
		t.Fatalf("type: %v", err)
	}
	if got := len(AllowedImportExtensions()); got != len(allowedImportExtensions) {
		t.Fatalf("extensions=%d", got)
	}
}

func TestDownloadRemoteDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.pdf":
			if r.URL.RawQuery != "token=abc&x=1" {
				http.Error(w, "bad query", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte("%PDF-1.4"))
		case "/big":
			_, _ = w.Write(bytesOf(8))
		case "/missing":
			http.NotFound(w, r)
		case "/busy":
			w.WriteHeader(http.StatusBadGateway)
		case "/go":
			http.Redirect(w, r, "/a.pdf?token=abc&x=1", http.StatusFound)
		case "/file":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	data, err := downloadRemoteDocument(context.Background(), srv.URL+"/a.pdf?token=abc&x=1", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "%PDF-1.4" {
		t.Fatalf("body=%q", data)
	}

	redirected, err := downloadRemoteDocument(context.Background(), srv.URL+"/go", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(redirected) != "%PDF-1.4" {
		t.Fatalf("redirect body=%q", redirected)
	}

	_, err = downloadRemoteDocument(context.Background(), srv.URL+"/big", 4)
	if !isPermanentTestErr(err) || !strings.Contains(err.Error(), "超过限制") {
		t.Fatalf("size err=%v", err)
	}
	_, err = downloadRemoteDocument(context.Background(), srv.URL+"/missing", 1024)
	if !isPermanentTestErr(err) || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("404 err=%v", err)
	}
	_, err = downloadRemoteDocument(context.Background(), srv.URL+"/busy", 1024)
	if err == nil || isPermanentTestErr(err) || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("502 err=%v", err)
	}
	_, err = downloadRemoteDocument(context.Background(), srv.URL+"/file", 1024)
	if !isPermanentTestErr(err) {
		t.Fatalf("file redirect err=%v", err)
	}
}

func TestFitFileNameKeepsExtension(t *testing.T) {
	name := strings.Repeat("a", 300) + ".pdf"
	got := fitFileName(name)
	if len(got) != 256 || !strings.HasSuffix(got, ".pdf") {
		t.Fatalf("got len=%d name=%q", len(got), got)
	}
}

func isPermanentTestErr(err error) bool {
	var marked interface{ Permanent() bool }
	return errors.As(err, &marked) && marked.Permanent()
}

func bytesOf(n int) []byte {
	return []byte(strings.Repeat("x", n))
}
