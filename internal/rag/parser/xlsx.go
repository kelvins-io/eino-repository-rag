package parser

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

func extractXLSX(path string) (*Result, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("xlsx 无工作表")
	}

	var b strings.Builder
	for _, name := range sheets {
		rows, err := f.GetRows(name)
		if err != nil {
			continue
		}
		if len(rows) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "## 工作表: %s\n\n", name)
		for _, row := range rows {
			// 跳过全空行
			empty := true
			for _, c := range row {
				if strings.TrimSpace(c) != "" {
					empty = false
					break
				}
			}
			if empty {
				continue
			}
			b.WriteString(strings.Join(row, "\t"))
			b.WriteByte('\n')
		}
	}

	plain := normalizeText(b.String())
	if plain == "" {
		return nil, fmt.Errorf("xlsx 未提取到文本")
	}
	return &Result{
		Text:        plain,
		ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		Format:      "xlsx",
		Pages:       len(sheets),
	}, nil
}
