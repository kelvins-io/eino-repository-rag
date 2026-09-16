package parser

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Result 文档解析结果
type Result struct {
	Text        string
	ContentType string
	Format      string // pdf|docx|xlsx|pptx|html|markdown|text|csv|json
	Pages       int    // 可读页数（PDF/PPTX 有意义）
}

// ExtractFile 按扩展名/Content-Type 从本地文件提取纯文本
func ExtractFile(path, contentType string) (*Result, error) {
	format := detectFormat(path, contentType)
	switch format {
	case "pdf":
		return extractPDF(path)
	case "docx":
		return extractDOCX(path)
	case "xlsx":
		return extractXLSX(path)
	case "pptx":
		return extractPPTX(path)
	case "html":
		return extractHTMLFile(path)
	case "csv", "markdown", "text", "json":
		return extractPlainFile(path, format)
	case "doc":
		return nil, fmt.Errorf("不支持旧版 .doc，请转换为 .docx 后导入")
	default:
		// 未知类型尝试按纯文本读取（兼容 .md/.txt 等）
		res, err := extractPlainFile(path, "text")
		if err != nil {
			return nil, fmt.Errorf("不支持的文件类型 %q（扩展名=%s）: %w", contentType, filepath.Ext(path), err)
		}
		// 若几乎全是不可打印二进制，判定失败
		if looksBinary(res.Text) {
			return nil, fmt.Errorf("不支持的二进制文件类型（扩展名=%s），请使用 PDF/DOCX/XLSX/PPTX/HTML/TXT/MD", filepath.Ext(path))
		}
		return res, nil
	}
}

func detectFormat(path, contentType string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "pdf"
	case ".docx":
		return "docx"
	case ".doc":
		return "doc"
	case ".xlsx", ".xlsm":
		return "xlsx"
	case ".pptx":
		return "pptx"
	case ".html", ".htm":
		return "html"
	case ".md", ".markdown":
		return "markdown"
	case ".csv":
		return "csv"
	case ".json":
		return "json"
	case ".txt", ".log", ".text":
		return "text"
	}

	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.Contains(ct, "pdf"):
		return "pdf"
	case strings.Contains(ct, "wordprocessingml") || strings.Contains(ct, "msword") && strings.Contains(ct, "openxml"):
		return "docx"
	case strings.Contains(ct, "msword") && !strings.Contains(ct, "openxml"):
		return "doc"
	case strings.Contains(ct, "spreadsheetml") || strings.Contains(ct, "excel"):
		return "xlsx"
	case strings.Contains(ct, "presentationml") || strings.Contains(ct, "powerpoint"):
		return "pptx"
	case strings.Contains(ct, "html"):
		return "html"
	case strings.Contains(ct, "markdown"):
		return "markdown"
	case strings.Contains(ct, "csv"):
		return "csv"
	case strings.Contains(ct, "json"):
		return "json"
	case strings.HasPrefix(ct, "text/"):
		return "text"
	}
	return ""
}

func looksBinary(s string) bool {
	if s == "" {
		return true
	}
	n := len(s)
	if n > 4096 {
		n = 4096
	}
	nul := 0
	for i := 0; i < n; i++ {
		if s[i] == 0 {
			nul++
		}
	}
	return nul > n/50
}

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	// 压缩过多空行，保留段落结构
	for strings.Contains(s, "\n\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n\n", "\n\n\n")
	}
	return strings.TrimSpace(s)
}
