package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	"github.com/kelvins-io/eino-repository-rag/internal/model"
)

const defaultMaxImportBatch = 20

// 与知识库导入弹窗 accept 列表一致。链接只看路径扩展名，查询参数不参与类型判断。
var allowedImportExtensions = []string{
	".pdf", ".docx", ".xlsx", ".pptx", ".html", ".htm",
	".md", ".markdown", ".txt", ".csv", ".json",
	".png", ".jpg", ".jpeg", ".webp", ".tif", ".tiff", ".bmp", ".gif",
}

var allowedImportExtSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(allowedImportExtensions))
	for _, ext := range allowedImportExtensions {
		set[ext] = struct{}{}
	}
	return set
}()

// AllowedImportExtensions 返回普通导入与链接导入共同支持的扩展名（含点、小写）。
func AllowedImportExtensions() []string {
	out := make([]string, len(allowedImportExtensions))
	copy(out, allowedImportExtensions)
	return out
}

// LinkDocumentItem 一条待导入的在线文档。
type LinkDocumentItem struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// ImportLinksOptions 链接导入参数。MaxPerRequest 与单次上传文件数上限相同，小于等于 0 时用默认 20。
type ImportLinksOptions struct {
	UserID          string
	TenantID        uint
	KnowledgeBaseID uint
	DirectoryID     *uint
	Items           []LinkDocumentItem
	MaxPerRequest   int
}

type preparedLink struct {
	URL      string
	Title    string
	FileName string
}

type linkTypeError struct {
	ext string
}

func (e *linkTypeError) Error() string {
	if e == nil {
		return "文档类型无效"
	}
	return "文档类型无效（" + e.ext + "）"
}

// remotePermanentError 表示重试也无法成功的在线文档错误（格式、类型、4xx、超限、内容重复）。
type remotePermanentError struct {
	msg string
}

func (e *remotePermanentError) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

// Permanent 供索引队列识别为不可重试错误。
func (e *remotePermanentError) Permanent() bool { return true }

func permanentRemote(format string, args ...any) error {
	return &remotePermanentError{msg: fmt.Sprintf(format, args...)}
}

var remoteHTTPClient = &http.Client{
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return permanentRemote("文档链接重定向次数过多")
		}
		scheme := strings.ToLower(req.URL.Scheme)
		if scheme != "http" && scheme != "https" {
			return permanentRemote("文档链接格式无效")
		}
		return nil
	},
}

// ImportDocumentLinks 校验链接与标题后立即落库，并由索引队列异步下载、建索引。
func (s *KnowledgeService) ImportDocumentLinks(_ context.Context, opts ImportLinksOptions) (*ImportResult, error) {
	prepared, err := validateDocumentLinks(opts.Items, opts.MaxPerRequest)
	if err != nil {
		return nil, err
	}

	actor, err := requireActor(opts.UserID, opts.TenantID)
	if err != nil {
		return nil, err
	}

	kbID := opts.KnowledgeBaseID
	var kb *model.KnowledgeBase
	if kbID == 0 {
		kb, err = s.kbRepo.GetOrCreateDefault(actor.TenantID, actor.UserID)
		if err != nil {
			return nil, fmt.Errorf("resolve default knowledge base: %w", err)
		}
		kbID = kb.ID
	} else {
		kb, err = s.requireKBAccess(kbID, actor)
		if err != nil {
			return nil, err
		}
	}

	if opts.DirectoryID != nil && *opts.DirectoryID == 0 {
		opts.DirectoryID = nil
	}
	if opts.DirectoryID != nil {
		dir, err := s.requireDirAccess(*opts.DirectoryID, actor)
		if err != nil {
			return nil, err
		}
		if dir.KnowledgeBaseID != kbID {
			return nil, fmt.Errorf("directory 不属于指定知识库")
		}
	}

	tenant, used, err := s.UploadPolicy(actor.TenantID)
	if err != nil {
		return nil, err
	}
	maxFiles := tenant.UploadMaxFiles()
	if used+int64(len(prepared)) > int64(maxFiles) {
		return nil, fmt.Errorf("已达到或将超过租户文件总数上限 %d（已有 %d）", maxFiles, used)
	}

	result := &ImportResult{
		Items: make([]ImportFileResult, 0, len(prepared)),
		Total: len(prepared),
	}
	for _, item := range prepared {
		doc := &model.Document{
			TenantID:        kb.TenantID,
			UserID:          actor.UserID,
			KnowledgeBaseID: kbID,
			DirectoryID:     opts.DirectoryID,
			Title:           item.Title,
			FileName:        item.FileName,
			FilePath:        "",
			Source:          item.URL,
			ContentType:     guessContentType(item.FileName),
			Status:          model.DocumentStatusPending,
		}
		if err := s.docRepo.Create(doc); err != nil {
			return nil, fmt.Errorf("保存文档记录失败: %w", err)
		}
		if s.rag != nil {
			s.rag.IndexDocumentAsync(doc.ID)
		}
		result.Items = append(result.Items, ImportFileResult{
			FileName: item.FileName,
			Document: doc,
			Message:  "已接收文档链接，正在后台读取并建立索引",
		})
		result.Imported++
	}
	result.Message = fmt.Sprintf("已接收 %d 个文档链接，正在后台读取并建立索引", result.Imported)
	return result, nil
}

// MaterializeRemoteDocument 把已登记的在线文档下载到本地，供索引队列解析。
// 本地文件仍在，或来源不是受支持的文档链接时直接返回。
func (s *KnowledgeService) MaterializeRemoteDocument(ctx context.Context, doc *model.Document) error {
	if s == nil || doc == nil || doc.ID == 0 {
		return fmt.Errorf("document is required")
	}
	if sourceUploadExists(doc.FilePath) {
		return nil
	}
	fileName, err := parseDocumentLink(doc.Source)
	if err != nil {
		return nil
	}
	fileName = fitFileName(fileName)

	maxBytes, err := s.remoteFetchLimit(doc.TenantID)
	if err != nil {
		return err
	}
	data, err := downloadRemoteDocument(ctx, strings.TrimSpace(doc.Source), maxBytes)
	if err != nil {
		return err
	}

	sum := md5.Sum(data)
	contentMD5 := hex.EncodeToString(sum[:])
	if existing, err := s.docRepo.GetByKnowledgeBaseAndMD5(doc.KnowledgeBaseID, contentMD5); err == nil {
		if existing.ID != doc.ID {
			return permanentRemote("文件内容与已有文档重复，已跳过索引")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lookup by md5: %w", err)
	}

	if s.rag == nil {
		return fmt.Errorf("rag pipeline is required")
	}
	saved, err := s.rag.SaveUpload(doc.TenantID, doc.KnowledgeBaseID, fileName, data)
	if err != nil {
		return fmt.Errorf("save remote document: %w", err)
	}
	contentType := guessContentType(fileName)
	if err := s.docRepo.UpdateFetchedFile(doc.ID, fileName, saved, contentMD5, contentType, int64(len(data))); err != nil {
		_ = removeLocalFile(saved)
		return fmt.Errorf("update fetched document: %w", err)
	}
	logger.S().Infof("[kb] fetched remote document id=%d file=%s bytes=%d", doc.ID, fileName, len(data))
	return nil
}

func (s *KnowledgeService) remoteFetchLimit(tenantID uint) (int64, error) {
	tenant, _, err := s.UploadPolicy(tenantID)
	if err != nil {
		return 0, err
	}
	maxBytes := tenant.UploadMaxFileSizeBytes()
	if s != nil && s.rag != nil {
		if sys := s.rag.MaxUploadFileSizeBytes(); sys > 0 && (maxBytes <= 0 || sys < maxBytes) {
			maxBytes = sys
		}
	}
	if maxBytes <= 0 {
		return 0, permanentRemote("文件大小超过限制")
	}
	return maxBytes, nil
}

func validateDocumentLinks(items []LinkDocumentItem, maxPerRequest int) ([]preparedLink, error) {
	if maxPerRequest <= 0 {
		maxPerRequest = defaultMaxImportBatch
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("缺少文档链接")
	}
	if len(items) > maxPerRequest {
		return nil, fmt.Errorf("单次导入的文档链接不能超过 %d 个，当前 %d 个", maxPerRequest, len(items))
	}

	out := make([]preparedLink, 0, len(items))
	seen := make(map[string]int, len(items))
	for i, item := range items {
		n := i + 1
		title := strings.TrimSpace(item.Title)
		if title == "" {
			return nil, fmt.Errorf("第 %d 个文档标题不能为空", n)
		}
		if len(title) > importTitleMaxLen {
			return nil, fmt.Errorf("第 %d 个文档标题过长", n)
		}
		raw := strings.TrimSpace(item.URL)
		if raw == "" {
			return nil, fmt.Errorf("第 %d 个文档链接不能为空", n)
		}
		if len(raw) > importSourceMaxLen {
			return nil, fmt.Errorf("第 %d 个文档链接过长", n)
		}
		fileName, err := parseDocumentLink(raw)
		if err != nil {
			var typeErr *linkTypeError
			if errors.As(err, &typeErr) {
				return nil, fmt.Errorf("第 %d 个文档类型无效（%s）", n, typeErr.ext)
			}
			return nil, fmt.Errorf("第 %d 个文档链接格式无效，路径需要以支持的文档扩展名结尾（可以带查询参数）", n)
		}
		if prev, ok := seen[raw]; ok {
			return nil, fmt.Errorf("第 %d 个文档链接与第 %d 个重复，不能重复提交", n, prev)
		}
		seen[raw] = n
		out = append(out, preparedLink{
			URL:      raw,
			Title:    title,
			FileName: fitFileName(fileName),
		})
	}
	return out, nil
}

// parseDocumentLink 校验 http(s) 文档链接。扩展名取自路径，查询参数和片段被保留但不参与判断。
func parseDocumentLink(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") {
		return "", fmt.Errorf("文档链接格式无效")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("文档链接格式无效")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return "", fmt.Errorf("文档链接格式无效")
	}
	decoded := u.Path
	if unescaped, err := url.PathUnescape(u.Path); err == nil {
		decoded = unescaped
	}
	base := path.Base(decoded)
	base = strings.Map(func(r rune) rune {
		if r < 32 || r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, base)
	if base == "" || base == "." || base == ".." || base == "/" {
		return "", fmt.Errorf("文档链接格式无效")
	}
	ext := strings.ToLower(path.Ext(base))
	if ext == "" || ext == "." {
		return "", fmt.Errorf("文档链接格式无效")
	}
	if _, ok := allowedImportExtSet[ext]; !ok {
		return "", &linkTypeError{ext: ext}
	}
	return base, nil
}

func fitFileName(name string) string {
	const maxLen = 256
	if len(name) <= maxLen {
		return name
	}
	ext := path.Ext(name)
	if len(ext) >= maxLen {
		return name[:maxLen]
	}
	stem := strings.TrimSuffix(name, ext)
	keep := maxLen - len(ext)
	if len(stem) > keep {
		stem = stem[:keep]
	}
	return stem + ext
}

func downloadRemoteDocument(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, permanentRemote("文件大小超过限制")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, permanentRemote("文档链接格式无效")
	}
	req.Header.Set("User-Agent", "eino-repository-rag")
	req.Header.Set("Accept", "*/*")

	resp, err := remoteHTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		var permanent *remotePermanentError
		if errors.As(err, &permanent) {
			return nil, permanent
		}
		return nil, fmt.Errorf("读取在线文档失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		return nil, fmt.Errorf("读取在线文档失败：HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		return nil, permanentRemote("读取在线文档失败：HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, permanentRemote("文件大小超过限制 %dMB", maxBytes/1024/1024)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("读取在线文档失败: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, permanentRemote("文件大小超过限制 %dMB", maxBytes/1024/1024)
	}
	return data, nil
}

func removeLocalFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	err := os.Remove(path)
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
