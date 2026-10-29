package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseYoudaoNoteLink(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		fileName string
		ok       bool
		invalid  bool
	}{
		{
			name:     "short share",
			raw:      "https://share.note.youdao.com/s/SAGUBzGP",
			fileName: "youdao-SAGUBzGP.md",
			ok:       true,
		},
		{
			name:     "note.youdao short",
			raw:      "https://note.youdao.com/s/SAGUBzGP",
			fileName: "youdao-SAGUBzGP.md",
			ok:       true,
		},
		{
			name:     "noteshare query",
			raw:      "https://share.note.youdao.com/noteshare?id=dc6529bf6c8b3eca8abfbf6915cc6547",
			fileName: "youdao-dc6529bf6c8b3eca8abfbf6915cc6547.md",
			ok:       true,
		},
		{
			name:     "ynoteshare",
			raw:      "https://share.note.youdao.com/ynoteshare/index.html?id=dc6529bf6c8b3eca8abfbf6915cc6547&type=note",
			fileName: "youdao-dc6529bf6c8b3eca8abfbf6915cc6547.md",
			ok:       true,
		},
		{
			name:     "legacy share path",
			raw:      "https://note.youdao.com/share/?id=dc6529bf6c8b3eca8abfbf6915cc6547&type=note",
			fileName: "youdao-dc6529bf6c8b3eca8abfbf6915cc6547.md",
			ok:       true,
		},
		{
			name:    "youdao host but missing id",
			raw:     "https://share.note.youdao.com/noteshare",
			ok:      true,
			invalid: true,
		},
		{
			name: "normal pdf not youdao",
			raw:  "https://example.com/a.pdf",
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := parseYoudaoNoteLink(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v err=%v name=%q", ok, tt.ok, err, got)
			}
			if !tt.ok {
				return
			}
			if tt.invalid {
				if err == nil {
					t.Fatalf("expected invalid, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.fileName {
				t.Fatalf("fileName=%q want %q", got, tt.fileName)
			}
			if _, err := parseDocumentLink(tt.raw); err != nil {
				t.Fatalf("parseDocumentLink: %v", err)
			}
		})
	}
}

func TestValidateDocumentLinksAcceptsYoudao(t *testing.T) {
	got, err := validateDocumentLinks([]LinkDocumentItem{
		{URL: "https://share.note.youdao.com/s/SAGUBzGP", Title: "有道笔记"},
		{URL: "https://example.com/a.pdf", Title: "普通文档"},
	}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].FileName != "youdao-SAGUBzGP.md" || got[1].FileName != "a.pdf" {
		t.Fatalf("%+v", got)
	}
}

func TestYoudaoEditorJSONToMarkdown(t *testing.T) {
	content := `{
	  "5":[
	    {"4":{"l":"h1"},"5":[{"7":[{"8":"标题"}]}],"6":"h"},
	    {"4":{"l":"h3"},"5":[{"7":[{"8":"小节"}]}],"6":"h"},
	    {"4":{"lt":"unordered"},"5":[{"7":[{"8":"条目一"}]}],"6":"l"},
	    {"5":[{"7":[{"8":"正文一段"}]}],"6":"p"},
	    {"4":{"fn":"说明.pdf"},"6":"a"}
	  ]
	}`
	md, err := youdaoEditorJSONToMarkdown(content)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# 标题", "### 小节", "- 条目一", "正文一段", "- 附件：说明.pdf"} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in %q", want, md)
		}
	}
}

func TestFetchYoudaoNoteDocument(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/s/SAGUBzGP", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/noteshare?id=dc6529bf6c8b3eca8abfbf6915cc6547", http.StatusFound)
	})
	mux.HandleFunc("/yws/api/note/dc6529bf6c8b3eca8abfbf6915cc6547", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tl":"测试提交",
			"domain":0,
			"content":"{\"5\":[{\"4\":{\"l\":\"h1\"},\"5\":[{\"7\":[{\"8\":\"测试提交\"}]}],\"6\":\"h\"},{\"5\":[{\"7\":[{\"8\":\"更新说明\"}]}],\"6\":\"p\"}]}"
		}`))
	})
	mux.HandleFunc("/s/VYbxjXKH", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/noteshare?id=b22f5aac168ebbacd321375477b6710f", http.StatusFound)
	})
	mux.HandleFunc("/yws/api/note/b22f5aac168ebbacd321375477b6710f", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tl":"eino-repository-rag-README.md","domain":1,"p":"/WEB9e37c50804ff6438c724901edd9416c5"}`))
	})
	mux.HandleFunc("/yws/api/personal/file/WEB9e37c50804ff6438c724901edd9416c5", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("method") != "download" || r.URL.Query().Get("shareKey") != "b22f5aac168ebbacd321375477b6710f" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/markdown;charset=UTF-8")
		_, _ = w.Write([]byte("# eino-repository-rag\n\n知识库 README\n"))
	})
	mux.HandleFunc("/s/QFZvpxtT", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/noteshare?id=332f7adc111111111111111111111111", http.StatusFound)
	})
	mux.HandleFunc("/yws/api/note/332f7adc111111111111111111111111", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tl":"CloudWeGo-white-paper.pdf","domain":1,"p":"/WEBfpdfattachment001"}`))
	})
	mux.HandleFunc("/yws/api/personal/file/WEBfpdfattachment001", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("method") != "download" || r.URL.Query().Get("shareKey") != "332f7adc111111111111111111111111" {
			http.Error(w, "bad query", http.StatusBadRequest)
			return
		}
		// 有道对 PDF 开 read=true 会 500，必须走二进制下载。
		if r.URL.Query().Get("read") == "true" {
			http.Error(w, "read not supported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="CloudWeGo-white-paper.pdf"`)
		_, _ = w.Write([]byte("%PDF-1.4 mock pdf body"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	oldBase := youdaoAPIBase
	oldClient := remoteHTTPClient
	youdaoAPIBase = srv.URL
	remoteHTTPClient = srv.Client()
	defer func() {
		youdaoAPIBase = oldBase
		remoteHTTPClient = oldClient
	}()

	fileName, data, err := fetchYoudaoNoteDocument(context.Background(), "https://share.note.youdao.com/s/SAGUBzGP", "我的笔记", 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if fileName != "我的笔记.md" {
		t.Fatalf("fileName=%q", fileName)
	}
	text := string(data)
	if !strings.Contains(text, "# 测试提交") || !strings.Contains(text, "更新说明") {
		t.Fatalf("markdown=%q", text)
	}

	fileName, data, err = fetchYoudaoNoteDocument(context.Background(), "https://share.note.youdao.com/s/VYbxjXKH", "【有道云笔记】README", 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if fileName != "【有道云笔记】README.md" {
		t.Fatalf("md fileName=%q", fileName)
	}
	text = string(data)
	if !strings.Contains(text, "# eino-repository-rag") || !strings.Contains(text, "知识库 README") {
		t.Fatalf("markdown note=%q", text)
	}

	fileName, data, err = fetchYoudaoNoteDocument(context.Background(), "https://share.note.youdao.com/s/QFZvpxtT", "PDF", 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if fileName != "CloudWeGo-white-paper.pdf" {
		t.Fatalf("pdf fileName=%q", fileName)
	}
	if !strings.HasPrefix(string(data), "%PDF-1.4") {
		t.Fatalf("pdf body=%q", data)
	}
}

func TestSanitizeFileStem(t *testing.T) {
	if got := sanitizeFileStem(" 测试/提交.note "); got != "测试_提交" {
		t.Fatalf("got %q", got)
	}
	if got := sanitizeFileStem("22年行情异动记录.lxtable"); got != "22年行情异动记录" {
		t.Fatalf("lxtable stem=%q", got)
	}
}

func TestYoudaoTableJSONToMarkdown(t *testing.T) {
	content := `{
	  "type":"http://sharejs.org/types/JSONv0",
	  "data":{
	    "title":"demo.lxtable",
	    "tabs":["0"],
	    "sheets":{
	      "0":{
	        "title":"工作表1",
	        "rows":[0,1],
	        "cols":[0,1,2],
	        "cells":{
	          "0,0":{"0":"日期"},
	          "0,1":{"0":"板块"},
	          "0,2":{"0":"涨幅"},
	          "1,0":{"0":"7号"},
	          "1,1":{"0":"指数下跌"},
	          "1,2":{"0":-0.0217}
	        }
	      }
	    }
	  }
	}`
	if !youdaoLooksLikeTableJSON(content) {
		t.Fatal("expected table json")
	}
	md, err := youdaoTableJSONToMarkdown(content)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## 工作表1", "日期\t板块\t涨幅", "7号\t指数下跌\t-0.0217"} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in %q", want, md)
		}
	}
}
