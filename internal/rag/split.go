package rag

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

var (
	rePageSection  = regexp.MustCompile(`(?m)^##\s*第\s*(\d+)\s*页\s*$`)
	reSheetSection = regexp.MustCompile(`(?m)^##\s*工作表:\s*(.+?)\s*$`)
	reMDHeading    = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*$`)
)

// textSection 结构切分得到的语义段落（页 / 工作表 / 标题）
type textSection struct {
	Title   string
	Page    int
	Content string
}

// splitDocument 按 format 做结构切分；超长段落再走 Recursive Splitter。
// structureEnabled=false 时整篇直接 Recursive。
func splitDocument(
	ctx context.Context,
	recursive document.Transformer,
	text, format string,
	chunkSize int,
	structureEnabled bool,
) ([]*schema.Document, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	if chunkSize <= 0 {
		chunkSize = 800
	}

	var sections []textSection
	if structureEnabled {
		sections = splitByStructure(text, format)
	}
	if len(sections) == 0 {
		sections = []textSection{{Title: "", Content: text}}
	}

	out := make([]*schema.Document, 0, len(sections))
	for _, sec := range sections {
		body := strings.TrimSpace(sec.Content)
		if body == "" {
			continue
		}
		parts, err := splitSectionParts(ctx, recursive, body, chunkSize)
		if err != nil {
			return nil, err
		}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			meta := map[string]any{}
			if sec.Page > 0 {
				meta["page"] = sec.Page
			}
			if sec.Title != "" {
				meta["section"] = sec.Title
			}
			// 页级切分时正文可能已不含页眉；补回标记便于展示与兜底推断
			content := part
			if sec.Page > 0 && inferPageFromContent(part) <= 0 {
				content = fmt.Sprintf("## 第 %d 页\n\n%s", sec.Page, part)
			}
			out = append(out, &schema.Document{Content: content, MetaData: meta})
		}
	}
	return out, nil
}

func splitSectionParts(ctx context.Context, recursive document.Transformer, body string, chunkSize int) ([]string, error) {
	if utf8.RuneCountInString(body) <= chunkSize || recursive == nil {
		return []string{body}, nil
	}
	docs, err := recursive.Transform(ctx, []*schema.Document{{Content: body}})
	if err != nil {
		return nil, fmt.Errorf("recursive split section: %w", err)
	}
	parts := make([]string, 0, len(docs))
	for _, d := range docs {
		if d == nil {
			continue
		}
		s := strings.TrimSpace(d.Content)
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return []string{body}, nil
	}
	return parts, nil
}

// splitByStructure 按文档类型选择切分边界；无结构时返回 nil（由调用方回退整篇）。
func splitByStructure(text, format string) []textSection {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "pdf", "pptx":
		if secs := splitByPageMarkers(text); len(secs) > 0 {
			return secs
		}
	case "xlsx":
		if secs := splitBySheetMarkers(text); len(secs) > 0 {
			return secs
		}
	case "markdown", "html", "docx", "text":
		if secs := splitByHeadings(text); len(secs) > 0 {
			return secs
		}
	default:
		if secs := splitByPageMarkers(text); len(secs) > 0 {
			return secs
		}
		if secs := splitBySheetMarkers(text); len(secs) > 0 {
			return secs
		}
		if secs := splitByHeadings(text); len(secs) > 0 {
			return secs
		}
	}
	return nil
}

func splitByPageMarkers(text string) []textSection {
	idxs := rePageSection.FindAllStringSubmatchIndex(text, -1)
	if len(idxs) == 0 {
		return nil
	}
	secs := make([]textSection, 0, len(idxs))
	for i, loc := range idxs {
		page := atoiSafe(text[loc[2]:loc[3]])
		start := loc[1] // 标题行之后
		end := len(text)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		body := strings.TrimSpace(text[start:end])
		if body == "" {
			continue
		}
		title := fmt.Sprintf("第 %d 页", page)
		secs = append(secs, textSection{Title: title, Page: page, Content: body})
	}
	return secs
}

func splitBySheetMarkers(text string) []textSection {
	idxs := reSheetSection.FindAllStringSubmatchIndex(text, -1)
	if len(idxs) == 0 {
		return nil
	}
	secs := make([]textSection, 0, len(idxs))
	for i, loc := range idxs {
		name := strings.TrimSpace(text[loc[2]:loc[3]])
		start := loc[1]
		end := len(text)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		body := strings.TrimSpace(text[start:end])
		if body == "" {
			continue
		}
		secs = append(secs, textSection{Title: "工作表: " + name, Content: body})
	}
	return secs
}

func splitByHeadings(text string) []textSection {
	idxs := reMDHeading.FindAllStringSubmatchIndex(text, -1)
	if len(idxs) == 0 {
		return nil
	}
	// 若首部在第一个标题之前有前言，保留为独立段
	secs := make([]textSection, 0, len(idxs)+1)
	if idxs[0][0] > 0 {
		pre := strings.TrimSpace(text[:idxs[0][0]])
		if pre != "" {
			secs = append(secs, textSection{Title: "", Content: pre})
		}
	}
	for i, loc := range idxs {
		title := strings.TrimSpace(text[loc[4]:loc[5]])
		start := loc[1]
		end := len(text)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		body := strings.TrimSpace(text[start:end])
		// 标题本身写入 content 开头，检索时保留结构语义
		content := body
		if title != "" {
			if content != "" {
				content = title + "\n\n" + content
			} else {
				content = title
			}
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		secs = append(secs, textSection{Title: title, Content: content})
	}
	// 仅 1 个标题且几乎等于全文时，结构切分收益不大，仍返回（调用方可再 recursive）
	if len(secs) <= 1 {
		// 单标题文档：若切不出多段，回退整篇 recursive 更稳
		return nil
	}
	return secs
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
