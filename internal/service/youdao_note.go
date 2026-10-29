package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	youdaoShortSharePath = regexp.MustCompile(`(?i)^/s/([A-Za-z0-9_-]{4,64})/?$`)
	youdaoNoteIDRe       = regexp.MustCompile(`(?i)^[a-f0-9]{32}$`)
	youdaoShareTokenRe   = regexp.MustCompile(`(?i)^[A-Za-z0-9_-]{4,64}$`)

	// youdaoAPIBase 有道笔记开放接口根地址；测试可替换为 httptest。
	youdaoAPIBase = "https://note.youdao.com"
)

type youdaoNoteRef struct {
	Raw      string
	NoteID   string // 32 位 note id；短链解析后填入
	ShareKey string // 短链 /s/{key} 中的 key
	FileName string
}

// parseYoudaoNoteLink 识别有道云笔记公开分享链接，并给出待落库的 .md 文件名。
// ok=false 表示不是有道分享链接，应继续走普通文档直链校验。
func parseYoudaoNoteLink(raw string) (fileName string, ok bool, err error) {
	ref, ok, err := inspectYoudaoNoteLink(raw)
	if !ok || err != nil {
		return "", ok, err
	}
	return ref.FileName, true, nil
}

func inspectYoudaoNoteLink(raw string) (*youdaoNoteRef, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") {
		return nil, false, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, false, nil
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, false, nil
	}
	if !isYoudaoNoteHost(u.Host) {
		return nil, false, nil
	}

	ref := &youdaoNoteRef{Raw: raw}
	q := u.Query()
	if id := strings.TrimSpace(firstNonEmpty(q.Get("id"), q.Get("shareKey"), q.Get("noteid"))); id != "" {
		switch {
		case youdaoNoteIDRe.MatchString(id):
			ref.NoteID = strings.ToLower(id)
		case youdaoShareTokenRe.MatchString(id):
			ref.ShareKey = id
		default:
			return nil, true, fmt.Errorf("有道云笔记链接格式无效")
		}
	}

	pathName := strings.TrimSuffix(u.EscapedPath(), "/")
	if pathName == "" {
		pathName = "/"
	}
	if m := youdaoShortSharePath.FindStringSubmatch(pathName); len(m) == 2 {
		ref.ShareKey = m[1]
	} else {
		lowerPath := strings.ToLower(pathName)
		switch {
		case strings.Contains(lowerPath, "noteshare"),
			strings.Contains(lowerPath, "ynoteshare"),
			strings.HasSuffix(lowerPath, "/share"),
			strings.Contains(lowerPath, "/share/"):
			if ref.NoteID == "" && ref.ShareKey == "" {
				return nil, true, fmt.Errorf("有道云笔记链接缺少 id 参数")
			}
		case ref.NoteID != "" || ref.ShareKey != "":
			// query 已带出分享标识，路径形态可放宽
		default:
			return nil, true, fmt.Errorf("有道云笔记链接格式无效")
		}
	}

	key := ref.NoteID
	if key == "" {
		key = ref.ShareKey
	}
	if key == "" {
		return nil, true, fmt.Errorf("有道云笔记链接格式无效")
	}
	ref.FileName = fitFileName("youdao-" + sanitizeFileStem(key) + ".md")
	return ref, true, nil
}

func isYoudaoNoteHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	switch host {
	case "note.youdao.com", "share.note.youdao.com", "note.youdao.cn", "share.note.youdao.cn":
		return true
	default:
		return strings.HasSuffix(host, ".note.youdao.com") || strings.HasSuffix(host, ".note.youdao.cn")
	}
}

func fetchYoudaoNoteDocument(ctx context.Context, rawURL, preferredTitle string, maxBytes int64) (string, []byte, error) {
	ref, ok, err := inspectYoudaoNoteLink(rawURL)
	if err != nil {
		return "", nil, permanentRemote("%s", err.Error())
	}
	if !ok || ref == nil {
		return "", nil, permanentRemote("有道云笔记链接格式无效")
	}
	noteID, err := resolveYoudaoNoteID(ctx, ref)
	if err != nil {
		return "", nil, err
	}

	payload, err := loadYoudaoNotePayload(ctx, noteID)
	if err != nil {
		return "", nil, err
	}
	noteTitle := strings.TrimSpace(payload.Title)
	displayTitle := noteTitle
	if preferred := strings.TrimSpace(preferredTitle); preferred != "" {
		displayTitle = preferred
	}
	if displayTitle == "" {
		displayTitle = "有道云笔记"
	}

	// 附件型分享（PDF/Office 等）：content 为空，正文在 p 文件里，需按二进制下载。
	if strings.TrimSpace(payload.Content) == "" && strings.TrimSpace(payload.Path) != "" {
		if ext := youdaoTitleExt(noteTitle); youdaoIsBinaryImportExt(ext) {
			fileName, data, err := downloadYoudaoAttachment(ctx, noteID, payload.Path, noteTitle, maxBytes)
			if err != nil {
				return "", nil, err
			}
			return fileName, data, nil
		}
	}

	body, err := youdaoPayloadToMarkdown(ctx, noteID, payload)
	if err != nil {
		return "", nil, err
	}
	md := strings.TrimSpace(body)
	if md == "" {
		return "", nil, permanentRemote("有道云笔记内容为空")
	}
	heading := strings.TrimSuffix(strings.TrimSuffix(displayTitle, ".md"), ".markdown")
	if heading == "" {
		heading = displayTitle
	}
	// 正文本身已是完整 Markdown 时不再强行包一层标题，避免 README 类笔记重复套壳。
	if !strings.HasPrefix(md, "#") {
		md = "# " + heading + "\n\n" + md
	}
	data := []byte(md)
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return "", nil, permanentRemote("文件大小超过限制 %dMB", maxBytes/1024/1024)
	}
	fileName := fitFileName(sanitizeFileStem(displayTitle) + ".md")
	return fileName, data, nil
}

func resolveYoudaoNoteID(ctx context.Context, ref *youdaoNoteRef) (string, error) {
	if ref.NoteID != "" {
		return ref.NoteID, nil
	}
	if ref.ShareKey == "" {
		return "", permanentRemote("有道云笔记链接格式无效")
	}
	reqURL := strings.TrimRight(youdaoAPIBase, "/") + "/s/" + url.PathEscape(ref.ShareKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", permanentRemote("有道云笔记链接格式无效")
	}
	req.Header.Set("User-Agent", "eino-repository-rag")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")

	// 不跟随跳转，只读 Location 中的 note id
	client := *remoteHTTPClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("解析有道云笔记链接失败: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))

	loc := strings.TrimSpace(resp.Header.Get("Location"))
	if loc == "" && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// 少数情况下短链直接 200，尝试从最终 URL 或 body 无法拿到 id 时退回 shareKey 本身
		if youdaoNoteIDRe.MatchString(ref.ShareKey) {
			return strings.ToLower(ref.ShareKey), nil
		}
		return "", permanentRemote("无法解析有道云笔记分享 ID")
	}
	if loc == "" {
		return "", permanentRemote("无法解析有道云笔记分享 ID：HTTP %d", resp.StatusCode)
	}
	u, err := url.Parse(loc)
	if err != nil {
		return "", permanentRemote("无法解析有道云笔记分享 ID")
	}
	if !u.IsAbs() {
		base, _ := url.Parse(reqURL)
		u = base.ResolveReference(u)
	}
	id := strings.TrimSpace(u.Query().Get("id"))
	if !youdaoNoteIDRe.MatchString(id) {
		return "", permanentRemote("无法解析有道云笔记分享 ID")
	}
	return strings.ToLower(id), nil
}

func loadYoudaoNotePayload(ctx context.Context, noteID string) (*youdaoPublicNote, error) {
	base := strings.TrimRight(youdaoAPIBase, "/")
	// 新版 JSON 编辑器接口优先：旧 public/note 常返回“升级客户端”占位 XML。
	payload, err := fetchYoudaoNoteJSON(ctx, fmt.Sprintf(
		"%s/yws/api/note/%s?sev=j1&editorType=1&editorVersion=new-json-editor",
		base, url.PathEscape(noteID),
	))
	if err != nil {
		payload, err = fetchYoudaoNoteJSON(ctx, fmt.Sprintf(
			"%s/yws/public/note/%s",
			base, url.PathEscape(noteID),
		))
		if err != nil {
			return nil, err
		}
	}
	return payload, nil
}

func youdaoPayloadToMarkdown(ctx context.Context, noteID string, payload *youdaoPublicNote) (string, error) {
	if payload == nil {
		return "", permanentRemote("有道云笔记内容为空")
	}
	title := strings.TrimSpace(payload.Title)
	content := strings.TrimSpace(payload.Content)
	// Markdown 笔记（domain=1）常把正文放在 p 指向的文件里，content 为空。
	if content == "" {
		if strings.TrimSpace(payload.Path) == "" {
			return "", permanentRemote("有道云笔记内容为空")
		}
		// 文本类先用 read=true；失败再回退二进制下载（部分附件接口不支持 read）。
		md, err := downloadYoudaoPersonalText(ctx, noteID, payload.Path, 8<<20)
		if err == nil {
			return md, nil
		}
		fileName, data, derr := downloadYoudaoAttachment(ctx, noteID, payload.Path, title, 8<<20)
		if derr != nil {
			return "", err
		}
		ext := strings.ToLower(path.Ext(fileName))
		if ext == ".md" || ext == ".markdown" || ext == ".txt" {
			return string(data), nil
		}
		return "", permanentRemote("有道云笔记附件类型暂不支持解析为文本（%s）", ext)
	}

	switch {
	case strings.HasPrefix(content, "{"):
		if youdaoLooksLikeTableJSON(content) || payload.Domain == 3 || strings.HasSuffix(strings.ToLower(title), ".lxtable") {
			return youdaoTableJSONToMarkdown(content)
		}
		md, err := youdaoEditorJSONToMarkdown(content)
		if err != nil {
			if md2, err2 := youdaoTableJSONToMarkdown(content); err2 == nil {
				return md2, nil
			}
			return "", err
		}
		return md, nil
	case strings.HasPrefix(content, "<?xml") || strings.HasPrefix(content, "<note"):
		return youdaoNoteXMLToMarkdown(content)
	case payload.Domain == 1:
		return content, nil
	default:
		return content, nil
	}
}

func youdaoTitleExt(title string) string {
	title = strings.TrimSpace(title)
	ext := strings.ToLower(path.Ext(title))
	if ext == ".lxtable" {
		return ""
	}
	return ext
}

func youdaoIsBinaryImportExt(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	switch ext {
	case ".pdf", ".docx", ".xlsx", ".pptx",
		".png", ".jpg", ".jpeg", ".webp", ".tif", ".tiff", ".bmp", ".gif":
		return true
	default:
		return false
	}
}

func downloadYoudaoPersonalText(ctx context.Context, noteID, filePath string, maxBytes int64) (string, error) {
	data, _, err := downloadYoudaoPersonalBytes(ctx, noteID, filePath, maxBytes, true)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", permanentRemote("有道云笔记内容为空")
	}
	if strings.HasPrefix(text, "{") && strings.Contains(text, `"error"`) {
		return "", permanentRemote("读取有道云笔记正文失败")
	}
	return text, nil
}

func downloadYoudaoAttachment(ctx context.Context, noteID, filePath, title string, maxBytes int64) (string, []byte, error) {
	data, disposition, err := downloadYoudaoPersonalBytes(ctx, noteID, filePath, maxBytes, false)
	if err != nil {
		return "", nil, err
	}
	if len(data) == 0 {
		return "", nil, permanentRemote("有道云笔记内容为空")
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return "", nil, permanentRemote("文件大小超过限制 %dMB", maxBytes/1024/1024)
	}
	// JSON 错误体
	if len(data) < 4096 && data[0] == '{' && strings.Contains(string(data), `"error"`) {
		return "", nil, permanentRemote("读取有道云笔记附件失败")
	}
	fileName := youdaoFileNameFromTitleOrDisposition(title, disposition)
	if fileName == "" {
		fileName = "youdao-attachment.bin"
	}
	return fitFileName(fileName), data, nil
}

func downloadYoudaoPersonalBytes(ctx context.Context, noteID, filePath string, maxBytes int64, asText bool) ([]byte, string, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return nil, "", permanentRemote("有道云笔记内容为空")
	}
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}
	query := "?method=download&shareKey=" + url.QueryEscape(noteID)
	if asText {
		query = "?method=download&read=true&shareKey=" + url.QueryEscape(noteID) + "&cstk=false"
	}
	apiURL := strings.TrimRight(youdaoAPIBase, "/") + "/yws/api/personal/file" + filePath + query
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, "", permanentRemote("有道云笔记链接格式无效")
	}
	req.Header.Set("User-Agent", "eino-repository-rag")
	if asText {
		req.Header.Set("Accept", "text/markdown,text/plain,application/octet-stream;q=0.9,*/*;q=0.8")
	} else {
		req.Header.Set("Accept", "application/octet-stream,*/*;q=0.8")
	}

	limit := maxBytes
	if limit <= 0 {
		limit = 8 << 20
	}
	resp, err := remoteHTTPClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("读取有道云笔记正文失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("读取有道云笔记正文失败: %w", err)
	}
	if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, "", fmt.Errorf("读取有道云笔记正文失败：HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", permanentRemote("读取有道云笔记正文失败：HTTP %d", resp.StatusCode)
	}
	if int64(len(body)) > limit {
		return nil, "", permanentRemote("文件大小超过限制 %dMB", limit/1024/1024)
	}
	return body, resp.Header.Get("Content-Disposition"), nil
}

func youdaoFileNameFromTitleOrDisposition(title, disposition string) string {
	if name := fileNameFromContentDisposition(disposition); name != "" {
		return name
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	return title
}

func fileNameFromContentDisposition(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	// filename*=UTF-8''...
	if i := strings.Index(strings.ToLower(v), "filename*="); i >= 0 {
		rest := v[i+10:]
		if j := strings.IndexByte(rest, '\''); j >= 0 {
			if k := strings.IndexByte(rest[j+1:], '\''); k >= 0 {
				enc := rest[j+1+k+1:]
				if semi := strings.IndexByte(enc, ';'); semi >= 0 {
					enc = enc[:semi]
				}
				enc = strings.Trim(enc, `"`)
				if decoded, err := url.QueryUnescape(enc); err == nil && decoded != "" {
					return path.Base(decoded)
				}
			}
		}
	}
	if i := strings.Index(strings.ToLower(v), "filename="); i >= 0 {
		rest := strings.TrimSpace(v[i+9:])
		if semi := strings.IndexByte(rest, ';'); semi >= 0 {
			rest = rest[:semi]
		}
		rest = strings.Trim(rest, `"`)
		if rest != "" {
			return path.Base(rest)
		}
	}
	return ""
}

type youdaoPublicNote struct {
	Title   string
	Domain  int
	Path    string
	Content string
}

func fetchYoudaoNoteJSON(ctx context.Context, apiURL string) (*youdaoPublicNote, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, permanentRemote("有道云笔记链接格式无效")
	}
	req.Header.Set("User-Agent", "eino-repository-rag")
	req.Header.Set("Accept", "application/json,text/plain;q=0.9,*/*;q=0.8")

	resp, err := remoteHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("读取有道云笔记失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取有道云笔记失败: %w", err)
	}
	if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, fmt.Errorf("读取有道云笔记失败：HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, permanentRemote("读取有道云笔记失败：HTTP %d", resp.StatusCode)
	}

	var raw struct {
		Title   string          `json:"tl"`
		Domain  int             `json:"domain"`
		Path    string          `json:"p"`
		Content json.RawMessage `json:"content"`
		Error   string          `json:"error"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, permanentRemote("有道云笔记内容解析失败")
	}
	if raw.Error != "" || strings.Contains(strings.ToLower(raw.Message), "not shared") {
		msg := strings.TrimSpace(raw.Message)
		if msg == "" {
			msg = "笔记不存在或未公开分享"
		}
		return nil, permanentRemote("读取有道云笔记失败：%s", msg)
	}
	content := strings.TrimSpace(string(raw.Content))
	if len(content) >= 2 && content[0] == '"' {
		var s string
		if err := json.Unmarshal(raw.Content, &s); err == nil {
			content = s
		}
	}
	return &youdaoPublicNote{
		Title:   raw.Title,
		Domain:  raw.Domain,
		Path:    raw.Path,
		Content: content,
	}, nil
}

func youdaoLooksLikeTableJSON(content string) bool {
	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return false
	}
	if data, ok := root["data"].(map[string]any); ok {
		if _, ok := data["sheets"]; ok {
			return true
		}
	}
	if typ, _ := root["type"].(string); strings.Contains(strings.ToLower(typ), "jsonv0") {
		return true
	}
	return false
}

func youdaoTableJSONToMarkdown(content string) (string, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return "", permanentRemote("有道云笔记表格解析失败")
	}
	data, _ := root["data"].(map[string]any)
	if data == nil {
		return "", permanentRemote("有道云笔记表格内容为空")
	}
	sheets, _ := data["sheets"].(map[string]any)
	if len(sheets) == 0 {
		return "", permanentRemote("有道云笔记表格内容为空")
	}
	tabs, _ := data["tabs"].([]any)
	order := make([]string, 0, len(tabs))
	seen := map[string]struct{}{}
	for _, tab := range tabs {
		id := strings.TrimSpace(fmt.Sprint(tab))
		if id == "" {
			continue
		}
		if _, ok := sheets[id]; !ok {
			continue
		}
		order = append(order, id)
		seen[id] = struct{}{}
	}
	for id := range sheets {
		if _, ok := seen[id]; !ok {
			order = append(order, id)
		}
	}

	var b strings.Builder
	wrote := 0
	for _, id := range order {
		sheet, _ := sheets[id].(map[string]any)
		block := youdaoSheetToMarkdown(sheet)
		if strings.TrimSpace(block) == "" {
			continue
		}
		if wrote > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(block)
		wrote++
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", permanentRemote("有道云笔记表格内容为空")
	}
	return out, nil
}

func youdaoSheetToMarkdown(sheet map[string]any) string {
	if sheet == nil {
		return ""
	}
	title, _ := sheet["title"].(string)
	title = strings.TrimSpace(title)
	rowsAny, _ := sheet["rows"].([]any)
	colsAny, _ := sheet["cols"].([]any)
	cells, _ := sheet["cells"].(map[string]any)
	if len(rowsAny) == 0 || len(colsAny) == 0 || len(cells) == 0 {
		return ""
	}

	colIDs := make([]string, 0, len(colsAny))
	for _, c := range colsAny {
		colIDs = append(colIDs, fmt.Sprint(c))
	}

	matrix := make([][]string, 0, len(rowsAny))
	maxCol := 0
	for rpos := range rowsAny {
		row := make([]string, len(colIDs))
		lastNonEmpty := -1
		for cpos, colID := range colIDs {
			key := strconv.Itoa(rpos) + "," + colID
			text := ""
			if cell, ok := cells[key].(map[string]any); ok {
				text = youdaoCellText(cell)
			}
			row[cpos] = text
			if text != "" {
				lastNonEmpty = cpos
			}
		}
		if lastNonEmpty < 0 {
			continue
		}
		row = row[:lastNonEmpty+1]
		if len(row) > maxCol {
			maxCol = len(row)
		}
		matrix = append(matrix, row)
	}
	if len(matrix) == 0 || maxCol == 0 {
		return ""
	}
	for i := range matrix {
		if len(matrix[i]) < maxCol {
			padded := make([]string, maxCol)
			copy(padded, matrix[i])
			matrix[i] = padded
		}
	}

	var b strings.Builder
	if title != "" {
		b.WriteString("## ")
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	// 用 TSV 风格输出，避免单元格中的 | 破坏 Markdown 表格。
	for _, row := range matrix {
		for i, cell := range row {
			if i > 0 {
				b.WriteByte('\t')
			}
			b.WriteString(youdaoEscapeTableCell(cell))
		}
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func youdaoCellText(cell map[string]any) string {
	if cell == nil {
		return ""
	}
	v, ok := cell["0"]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(strings.ReplaceAll(t, "\r\n", "\n"))
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func youdaoEscapeTableCell(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func youdaoEditorJSONToMarkdown(content string) (string, error) {
	var root any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return "", permanentRemote("有道云笔记内容解析失败")
	}
	obj, _ := root.(map[string]any)
	blocks, _ := obj["5"].([]any)
	if len(blocks) == 0 {
		// 有些结构直接是块数组
		if arr, ok := root.([]any); ok {
			blocks = arr
		}
	}
	var b strings.Builder
	for _, block := range blocks {
		line := youdaoBlockToMarkdown(block)
		if line == "" {
			continue
		}
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", permanentRemote("有道云笔记内容为空")
	}
	return out, nil
}

func youdaoBlockToMarkdown(block any) string {
	obj, ok := block.(map[string]any)
	if !ok {
		return strings.TrimSpace(fmt.Sprint(block))
	}
	meta, _ := obj["4"].(map[string]any)
	kind, _ := obj["6"].(string)
	text := strings.TrimSpace(youdaoCollectText(obj["5"]))
	switch kind {
	case "h":
		level := 2
		if lv, ok := meta["l"].(string); ok {
			switch strings.ToLower(lv) {
			case "h1":
				level = 1
			case "h2":
				level = 2
			case "h3":
				level = 3
			case "h4":
				level = 4
			case "h5":
				level = 5
			case "h6":
				level = 6
			}
		}
		if text == "" {
			return ""
		}
		return strings.Repeat("#", level) + " " + text
	case "l":
		if text == "" {
			return ""
		}
		prefix := "- "
		if lt, _ := meta["lt"].(string); strings.EqualFold(lt, "ordered") {
			prefix = "1. "
		}
		return prefix + text
	case "a":
		name, _ := meta["fn"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return ""
		}
		return "- 附件：" + name
	default:
		if text == "" {
			return ""
		}
		return text
	}
}

func youdaoCollectText(v any) string {
	var parts []string
	var walk func(any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			if s, ok := n["8"].(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					parts = append(parts, s)
				}
			}
			// 链接文案可能在嵌套 list 里
			for k, child := range n {
				if k == "8" {
					continue
				}
				walk(child)
			}
		case []any:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(v)
	return strings.Join(parts, "")
}

func youdaoNoteXMLToMarkdown(content string) (string, error) {
	type xmlText struct {
		Text string `xml:",chardata"`
	}
	type xmlNode struct {
		XMLName xml.Name
		Text    []xmlText `xml:"text"`
		Nodes   []xmlNode `xml:",any"`
	}
	var root xmlNode
	if err := xml.Unmarshal([]byte(content), &root); err != nil {
		// 退化：正则抽 <text>
		re := regexp.MustCompile(`(?s)<text[^>]*>(.*?)</text>`)
		matches := re.FindAllStringSubmatch(content, -1)
		var parts []string
		for _, m := range matches {
			s := strings.TrimSpace(html.UnescapeString(m[1]))
			if s != "" {
				parts = append(parts, s)
			}
		}
		out := strings.Join(parts, "\n\n")
		if strings.TrimSpace(out) == "" {
			return "", permanentRemote("有道云笔记内容解析失败")
		}
		return out, nil
	}
	var parts []string
	var walk func(xmlNode)
	walk = func(n xmlNode) {
		for _, t := range n.Text {
			s := strings.TrimSpace(html.UnescapeString(t.Text))
			if s != "" {
				parts = append(parts, s)
			}
		}
		for _, c := range n.Nodes {
			walk(c)
		}
	}
	walk(root)
	out := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if out == "" {
		return "", permanentRemote("有道云笔记内容为空")
	}
	// 旧接口占位提示不算有效正文
	if strings.Contains(out, "当前笔记内容暂时无法查看") || strings.Contains(out, "请升级后用新版编辑器") {
		return "", permanentRemote("有道云笔记内容无法读取，请确认分享有效或改用新版笔记分享链接")
	}
	return out, nil
}

func sanitizeFileStem(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".note")
	name = strings.TrimSuffix(name, ".lxtable")
	name = strings.TrimSuffix(name, ".md")
	name = strings.TrimSuffix(name, ".markdown")
	if name == "" {
		return "youdao-note"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 32, r == '/', r == '\\', r == ':', r == '*', r == '?', r == '"', r == '<', r == '>', r == '|':
			b.WriteByte('_')
		case unicode.IsSpace(r):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	out = strings.Join(strings.Fields(out), " ")
	if out == "" {
		return "youdao-note"
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
