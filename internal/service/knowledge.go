package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kelvins-io/eino-repository-rag/internal/memory"
	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/rag"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

// ErrForbidden 资源存在但不属于当前 user_id
var ErrForbidden = errors.New("无权访问该资源")

type KnowledgeService struct {
	docRepo *repository.DocumentRepo
	kbRepo  *repository.KnowledgeBaseRepo
	dirRepo *repository.DirectoryRepo
	msgRepo *repository.MessageRepo
	mem     *memory.Manager
	rag     *rag.Pipeline
}

func NewKnowledgeService(
	docRepo *repository.DocumentRepo,
	kbRepo *repository.KnowledgeBaseRepo,
	dirRepo *repository.DirectoryRepo,
	msgRepo *repository.MessageRepo,
	mem *memory.Manager,
	pipeline *rag.Pipeline,
) *KnowledgeService {
	return &KnowledgeService{
		docRepo: docRepo,
		kbRepo:  kbRepo,
		dirRepo: dirRepo,
		msgRepo: msgRepo,
		mem:     mem,
		rag:     pipeline,
	}
}

func requireUserID(userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", fmt.Errorf("user_id is required")
	}
	return userID, nil
}

func (s *KnowledgeService) requireKBOwner(kbID uint, userID string) (*model.KnowledgeBase, error) {
	kb, err := s.kbRepo.GetByID(kbID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("知识库不存在")
		}
		return nil, err
	}
	if kb.UserID != userID {
		return nil, ErrForbidden
	}
	return kb, nil
}

func (s *KnowledgeService) requireDocOwner(docID uint, userID string) (*model.Document, error) {
	doc, err := s.docRepo.GetByID(docID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("文档不存在")
		}
		return nil, err
	}
	if doc.UserID != userID {
		return nil, ErrForbidden
	}
	return doc, nil
}

func (s *KnowledgeService) requireDirOwner(dirID uint, userID string) (*model.Directory, error) {
	dir, err := s.dirRepo.GetByID(dirID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("目录不存在")
		}
		return nil, err
	}
	if _, err := s.requireKBOwner(dir.KnowledgeBaseID, userID); err != nil {
		return nil, err
	}
	return dir, nil
}

// ImportFileResult 单个文件的导入结果
type ImportFileResult struct {
	FileName   string          `json:"file_name"`
	Document   *model.Document `json:"document"`
	Duplicated bool            `json:"duplicated"`
	Message    string          `json:"message"`
}

// ImportResult 批量导入结果
type ImportResult struct {
	Items      []ImportFileResult `json:"items"`
	Total      int                `json:"total"`
	Imported   int                `json:"imported"`
	Duplicated int                `json:"duplicated"`
	Message    string             `json:"message"`
}

type ImportOptions struct {
	UserID          string
	Title           string
	KnowledgeBaseID uint
	DirectoryID     *uint
}

// CreateKnowledgeBase 创建知识库
func (s *KnowledgeService) CreateKnowledgeBase(userID, name, description string) (*model.KnowledgeBase, error) {
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	kb := &model.KnowledgeBase{
		UserID:      userID,
		Name:        name,
		Description: description,
	}
	if err := s.kbRepo.Create(kb); err != nil {
		return nil, err
	}
	return kb, nil
}

func (s *KnowledgeService) ListKnowledgeBases(userID string) ([]model.KnowledgeBase, error) {
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.kbRepo.ListByUser(userID)
}

func (s *KnowledgeService) GetKnowledgeBase(id uint, userID string) (*model.KnowledgeBase, error) {
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.requireKBOwner(id, userID)
}

func (s *KnowledgeService) UpdateKnowledgeBase(id uint, userID, name, description string) (*model.KnowledgeBase, error) {
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}
	kb, err := s.requireKBOwner(id, userID)
	if err != nil {
		return nil, err
	}
	if name != "" {
		kb.Name = name
	}
	kb.Description = description
	if err := s.kbRepo.Update(kb); err != nil {
		return nil, err
	}
	return kb, nil
}

func (s *KnowledgeService) DeleteKnowledgeBase(id uint, userID string) error {
	userID, err := requireUserID(userID)
	if err != nil {
		return err
	}
	if _, err := s.requireKBOwner(id, userID); err != nil {
		return err
	}
	n, err := s.docRepo.CountByKnowledgeBase(id)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("知识库下仍有 %d 个文档，无法删除", n)
	}
	dirs, err := s.dirRepo.ListByKnowledgeBase(id)
	if err != nil {
		return err
	}
	if len(dirs) > 0 {
		return fmt.Errorf("知识库下仍有 %d 个目录，请先删除目录", len(dirs))
	}
	return s.kbRepo.Delete(id)
}

type CreateDirectoryInput struct {
	UserID          string
	KnowledgeBaseID uint
	ParentID        *uint
	Name            string
	Description     string
	SortOrder       int
}

func (s *KnowledgeService) CreateDirectory(in CreateDirectoryInput) (*model.Directory, error) {
	userID, err := requireUserID(in.UserID)
	if err != nil {
		return nil, err
	}
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if _, err := s.requireKBOwner(in.KnowledgeBaseID, userID); err != nil {
		return nil, err
	}

	if in.ParentID != nil {
		parent, err := s.requireDirOwner(*in.ParentID, userID)
		if err != nil {
			return nil, fmt.Errorf("parent directory: %w", err)
		}
		if parent.KnowledgeBaseID != in.KnowledgeBaseID {
			return nil, fmt.Errorf("parent directory 不属于该知识库")
		}
	}

	dir := &model.Directory{
		KnowledgeBaseID: in.KnowledgeBaseID,
		ParentID:        in.ParentID,
		Name:            in.Name,
		Description:     in.Description,
		SortOrder:       in.SortOrder,
	}
	if err := s.dirRepo.Create(dir); err != nil {
		return nil, err
	}
	return dir, nil
}

func (s *KnowledgeService) ListDirectoryTree(kbID uint, userID string) ([]*model.DirectoryNode, error) {
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireKBOwner(kbID, userID); err != nil {
		return nil, err
	}
	dirs, err := s.dirRepo.ListByKnowledgeBase(kbID)
	if err != nil {
		return nil, err
	}
	return repository.BuildDirectoryTree(dirs), nil
}

func (s *KnowledgeService) UpdateDirectory(id uint, userID, name, description string, parentID *uint, sortOrder *int) (*model.Directory, error) {
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}
	dir, err := s.requireDirOwner(id, userID)
	if err != nil {
		return nil, err
	}
	if name != "" {
		dir.Name = name
	}
	dir.Description = description
	if parentID != nil {
		if *parentID == 0 {
			dir.ParentID = nil
		} else {
			if *parentID == id {
				return nil, fmt.Errorf("目录不能将自己设为父目录")
			}
			parent, err := s.requireDirOwner(*parentID, userID)
			if err != nil {
				return nil, fmt.Errorf("parent directory: %w", err)
			}
			if parent.KnowledgeBaseID != dir.KnowledgeBaseID {
				return nil, fmt.Errorf("parent directory 不属于同一知识库")
			}
			// 防止环：parent 不能是自己的子孙
			desc, err := s.dirRepo.CollectSelfAndDescendantIDs(id)
			if err != nil {
				return nil, err
			}
			for _, did := range desc {
				if did == *parentID {
					return nil, fmt.Errorf("不能将目录移动到其子目录下")
				}
			}
			dir.ParentID = parentID
		}
	}
	if sortOrder != nil {
		dir.SortOrder = *sortOrder
	}
	if err := s.dirRepo.Update(dir); err != nil {
		return nil, err
	}
	return dir, nil
}

func (s *KnowledgeService) DeleteDirectory(id uint, userID string) error {
	userID, err := requireUserID(userID)
	if err != nil {
		return err
	}
	if _, err := s.requireDirOwner(id, userID); err != nil {
		return err
	}
	nDoc, err := s.docRepo.CountByDirectory(id)
	if err != nil {
		return err
	}
	if nDoc > 0 {
		return fmt.Errorf("目录下仍有 %d 个文档，无法删除", nDoc)
	}
	nChild, err := s.dirRepo.CountChildren(id)
	if err != nil {
		return err
	}
	if nChild > 0 {
		return fmt.Errorf("目录下仍有 %d 个子目录，请先删除子目录", nChild)
	}
	return s.dirRepo.Delete(id)
}

// ImportDocuments 批量导入文档：计算 MD5，同知识库内相同内容直接返回成功且不触发索引
func (s *KnowledgeService) ImportDocuments(
	_ context.Context,
	opts ImportOptions,
	fileHeaders []*multipart.FileHeader,
) (*ImportResult, error) {
	if len(fileHeaders) == 0 {
		return nil, fmt.Errorf("缺少上传文件")
	}

	userID, err := requireUserID(opts.UserID)
	if err != nil {
		return nil, err
	}

	kbID := opts.KnowledgeBaseID
	if kbID == 0 {
		kb, err := s.kbRepo.GetOrCreateDefault(userID)
		if err != nil {
			return nil, fmt.Errorf("resolve default knowledge base: %w", err)
		}
		kbID = kb.ID
	} else if _, err := s.requireKBOwner(kbID, userID); err != nil {
		return nil, err
	}

	if opts.DirectoryID != nil {
		dir, err := s.requireDirOwner(*opts.DirectoryID, userID)
		if err != nil {
			return nil, err
		}
		if dir.KnowledgeBaseID != kbID {
			return nil, fmt.Errorf("directory 不属于指定知识库")
		}
	}

	result := &ImportResult{
		Items: make([]ImportFileResult, 0, len(fileHeaders)),
		Total: len(fileHeaders),
	}
	// 同一请求内相同 MD5 也去重，避免重复落库/建索引
	seenMD5 := make(map[string]*model.Document, len(fileHeaders))

	for _, fh := range fileHeaders {
		title := ""
		// 单文件时可使用外部传入的 title
		if len(fileHeaders) == 1 {
			title = opts.Title
		}
		item, err := s.importOneFile(userID, kbID, opts.DirectoryID, title, fh, seenMD5)
		if err != nil {
			return nil, fmt.Errorf("导入文件 %q 失败: %w", fh.Filename, err)
		}
		result.Items = append(result.Items, *item)
		if item.Duplicated {
			result.Duplicated++
		} else {
			result.Imported++
		}
	}

	switch {
	case result.Imported > 0 && result.Duplicated > 0:
		result.Message = fmt.Sprintf("导入完成：新增 %d 个，重复跳过 %d 个", result.Imported, result.Duplicated)
	case result.Duplicated > 0:
		result.Message = fmt.Sprintf("全部为重复文件，已跳过索引构建（%d 个）", result.Duplicated)
	default:
		result.Message = fmt.Sprintf("文档已导入，索引构建已自动触发（%d 个）", result.Imported)
	}
	return result, nil
}

func (s *KnowledgeService) importOneFile(
	userID string,
	kbID uint,
	directoryID *uint,
	title string,
	fileHeader *multipart.FileHeader,
	seenMD5 map[string]*model.Document,
) (*ImportFileResult, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("open upload: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read upload: %w", err)
	}

	sum := md5.Sum(data)
	contentMD5 := hex.EncodeToString(sum[:])

	if existing, ok := seenMD5[contentMD5]; ok {
		return &ImportFileResult{
			FileName:   fileHeader.Filename,
			Document:   existing,
			Duplicated: true,
			Message:    "文件已存在，跳过导入与索引构建",
		}, nil
	}

	if existing, err := s.docRepo.GetByKnowledgeBaseAndMD5(kbID, contentMD5); err == nil {
		seenMD5[contentMD5] = existing
		return &ImportFileResult{
			FileName:   fileHeader.Filename,
			Document:   existing,
			Duplicated: true,
			Message:    "文件已存在，跳过导入与索引构建",
		}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("lookup by md5: %w", err)
	}

	if title == "" {
		title = fileHeader.Filename
	}

	path, err := s.rag.SaveUpload(userID, title, fileHeader.Filename, data)
	if err != nil {
		return nil, fmt.Errorf("save upload: %w", err)
	}

	doc := &model.Document{
		UserID:          userID,
		KnowledgeBaseID: kbID,
		DirectoryID:     directoryID,
		Title:           title,
		FileName:        fileHeader.Filename,
		FilePath:        path,
		ContentMD5:      contentMD5,
		ContentType:     fileHeader.Header.Get("Content-Type"),
		FileSize:        int64(len(data)),
		Status:          model.DocumentStatusPending,
	}
	if doc.ContentType == "" {
		doc.ContentType = guessContentType(fileHeader.Filename)
	}

	if err := s.docRepo.Create(doc); err != nil {
		return nil, fmt.Errorf("save document record: %w", err)
	}

	seenMD5[contentMD5] = doc
	s.rag.IndexDocumentAsync(doc.ID)

	return &ImportFileResult{
		FileName:   fileHeader.Filename,
		Document:   doc,
		Duplicated: false,
		Message:    "文档已导入，索引构建已自动触发",
	}, nil
}

func (s *KnowledgeService) ListDocuments(filter repository.DocumentListFilter, page, pageSize int) ([]model.Document, int64, error) {
	userID, err := requireUserID(filter.UserID)
	if err != nil {
		return nil, 0, err
	}
	filter.UserID = userID
	if filter.KnowledgeBaseID > 0 {
		if _, err := s.requireKBOwner(filter.KnowledgeBaseID, userID); err != nil {
			return nil, 0, err
		}
	}
	if filter.DirectoryID != nil && *filter.DirectoryID > 0 {
		if _, err := s.requireDirOwner(*filter.DirectoryID, userID); err != nil {
			return nil, 0, err
		}
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	return s.docRepo.List(filter, pageSize, (page-1)*pageSize)
}

func (s *KnowledgeService) GetDocument(id uint, userID string) (*model.Document, error) {
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.requireDocOwner(id, userID)
}

// DeleteDocument 删除文档并级联清理向量索引与本地文件
func (s *KnowledgeService) DeleteDocument(ctx context.Context, id uint, userID string) error {
	if id == 0 {
		return fmt.Errorf("无效的文档 ID")
	}
	userID, err := requireUserID(userID)
	if err != nil {
		return err
	}
	if _, err := s.requireDocOwner(id, userID); err != nil {
		return err
	}
	return s.rag.DeleteDocument(ctx, id)
}

// DeleteDocuments 批量删除文档（单项失败不中断，结果汇总返回）
func (s *KnowledgeService) DeleteDocuments(ctx context.Context, ids []uint, userID string) (*DeleteDocumentsResult, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("ids 不能为空")
	}
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}

	result := &DeleteDocumentsResult{
		Items: make([]DeleteDocumentItemResult, 0, len(ids)),
		Total: len(ids),
	}
	seen := make(map[uint]struct{}, len(ids))

	for _, id := range ids {
		if id == 0 {
			result.Items = append(result.Items, DeleteDocumentItemResult{
				ID:      id,
				Skipped: true,
				Message: "无效的文档 ID",
			})
			result.Skipped++
			continue
		}
		if _, ok := seen[id]; ok {
			result.Items = append(result.Items, DeleteDocumentItemResult{
				ID:      id,
				Skipped: true,
				Message: "请求内重复 ID，已跳过",
			})
			result.Skipped++
			continue
		}
		seen[id] = struct{}{}

		if err := s.DeleteDocument(ctx, id, userID); err != nil {
			result.Items = append(result.Items, DeleteDocumentItemResult{
				ID:      id,
				Skipped: true,
				Message: err.Error(),
			})
			result.Skipped++
			continue
		}
		result.Items = append(result.Items, DeleteDocumentItemResult{
			ID:      id,
			Skipped: false,
			Message: "已删除（含向量与文件）",
		})
		result.Deleted++
	}

	switch {
	case result.Deleted > 0 && result.Skipped > 0:
		result.Message = fmt.Sprintf("已删除 %d 个，跳过 %d 个", result.Deleted, result.Skipped)
	case result.Deleted > 0:
		result.Message = fmt.Sprintf("已删除 %d 个文档", result.Deleted)
	default:
		result.Message = fmt.Sprintf("未删除任何文档（跳过 %d 个）", result.Skipped)
	}
	return result, nil
}

// DeleteDocumentItemResult 单个文档删除结果
type DeleteDocumentItemResult struct {
	ID      uint   `json:"id"`
	Skipped bool   `json:"skipped"`
	Message string `json:"message"`
}

// DeleteDocumentsResult 批量删除结果
type DeleteDocumentsResult struct {
	Items   []DeleteDocumentItemResult `json:"items"`
	Total   int                        `json:"total"`
	Deleted int                        `json:"deleted"`
	Skipped int                        `json:"skipped"`
	Message string                     `json:"message"`
}

// ReindexItemResult 单个文档重新索引结果
type ReindexItemResult struct {
	ID       uint            `json:"id"`
	Document *model.Document `json:"document,omitempty"`
	Skipped  bool            `json:"skipped"`
	Message  string          `json:"message"`
}

// ReindexResult 批量重新索引结果
type ReindexResult struct {
	Items     []ReindexItemResult `json:"items"`
	Total     int                 `json:"total"`
	Triggered int                 `json:"triggered"`
	Skipped   int                 `json:"skipped"`
	Message   string              `json:"message"`
}

// ReindexDocuments 批量触发已导入文档的重新索引构建（异步）
func (s *KnowledgeService) ReindexDocuments(ids []uint, userID string) (*ReindexResult, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("ids 不能为空")
	}
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}

	result := &ReindexResult{
		Items: make([]ReindexItemResult, 0, len(ids)),
		Total: len(ids),
	}
	seen := make(map[uint]struct{}, len(ids))

	for _, id := range ids {
		if id == 0 {
			result.Items = append(result.Items, ReindexItemResult{
				ID:      id,
				Skipped: true,
				Message: "无效的文档 ID",
			})
			result.Skipped++
			continue
		}
		if _, ok := seen[id]; ok {
			result.Items = append(result.Items, ReindexItemResult{
				ID:      id,
				Skipped: true,
				Message: "请求内重复 ID，已跳过",
			})
			result.Skipped++
			continue
		}
		seen[id] = struct{}{}

		doc, err := s.requireDocOwner(id, userID)
		if err != nil {
			result.Items = append(result.Items, ReindexItemResult{
				ID:      id,
				Skipped: true,
				Message: err.Error(),
			})
			result.Skipped++
			continue
		}
		if doc.Status == model.DocumentStatusIndexing {
			result.Items = append(result.Items, ReindexItemResult{
				ID:       id,
				Document: doc,
				Skipped:  true,
				Message:  "文档正在索引中，已跳过",
			})
			result.Skipped++
			continue
		}

		s.rag.IndexDocumentAsync(doc.ID)
		result.Items = append(result.Items, ReindexItemResult{
			ID:       id,
			Document: doc,
			Skipped:  false,
			Message:  "已触发重新索引构建",
		})
		result.Triggered++
	}

	switch {
	case result.Triggered > 0 && result.Skipped > 0:
		result.Message = fmt.Sprintf("已触发 %d 个重新索引，跳过 %d 个", result.Triggered, result.Skipped)
	case result.Triggered > 0:
		result.Message = fmt.Sprintf("已触发 %d 个重新索引构建", result.Triggered)
	default:
		result.Message = fmt.Sprintf("未触发任何重新索引（跳过 %d 个）", result.Skipped)
	}
	return result, nil
}

func (s *KnowledgeService) prepareQueryRequest(req *rag.QueryRequest) error {
	if req.Query == "" {
		return fmt.Errorf("query is required")
	}
	userID, err := requireUserID(req.UserID)
	if err != nil {
		return err
	}
	req.UserID = userID
	if req.SessionID == "" {
		req.SessionID = uuid.NewString()
	}

	filter := &rag.RetrieveFilter{UserID: userID}
	if req.KnowledgeBaseID > 0 {
		if _, err := s.requireKBOwner(req.KnowledgeBaseID, userID); err != nil {
			return err
		}
		filter.KnowledgeBaseID = strconv.FormatUint(uint64(req.KnowledgeBaseID), 10)
	}
	if req.DirectoryID != nil && *req.DirectoryID > 0 {
		if _, err := s.requireDirOwner(*req.DirectoryID, userID); err != nil {
			return err
		}
		ids, err := s.dirRepo.CollectSelfAndDescendantIDs(*req.DirectoryID)
		if err != nil {
			return fmt.Errorf("directory not found: %w", err)
		}
		for _, id := range ids {
			filter.DirectoryIDs = append(filter.DirectoryIDs, strconv.FormatUint(uint64(id), 10))
		}
	}
	req.Filter = filter
	return nil
}

func (s *KnowledgeService) Query(ctx context.Context, req rag.QueryRequest) (*rag.QueryResponse, error) {
	if err := s.prepareQueryRequest(&req); err != nil {
		return nil, err
	}
	return s.rag.Query(ctx, req)
}

func (s *KnowledgeService) QueryStream(ctx context.Context, req rag.QueryRequest, onEvent rag.StreamHandler) error {
	if err := s.prepareQueryRequest(&req); err != nil {
		return err
	}
	return s.rag.QueryStream(ctx, req, onEvent)
}

func (s *KnowledgeService) GetHistory(sessionID, userID string) ([]model.Message, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	userID, err := requireUserID(userID)
	if err != nil {
		return nil, err
	}
	msgs, err := s.mem.GetLongTerm(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return msgs, nil
	}
	// 会话首条消息即归属用户；拒绝跨用户读取
	if msgs[0].UserID != userID {
		return nil, ErrForbidden
	}
	out := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.UserID == userID {
			out = append(out, m)
		}
	}
	return out, nil
}

func guessContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return "text/markdown"
	case ".txt", ".log", ".text":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".html", ".htm":
		return "text/html"
	case ".csv":
		return "text/csv"
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".doc":
		return "application/msword"
	case ".xlsx", ".xlsm":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		return "application/octet-stream"
	}
}
