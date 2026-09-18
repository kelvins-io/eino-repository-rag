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

// ErrForbidden 资源存在但当前主体无权访问
var ErrForbidden = errors.New("无权访问该资源")

// Actor 当前请求身份（来自 JWT）
type Actor struct {
	UserID   string
	TenantID uint
}

type KnowledgeService struct {
	docRepo     *repository.DocumentRepo
	kbRepo      *repository.KnowledgeBaseRepo
	dirRepo     *repository.DirectoryRepo
	msgRepo     *repository.MessageRepo
	userRepo    *repository.UserRepo
	tenantRepo  *repository.TenantRepo
	speechUsage *repository.SpeechUsageRepo
	mem         *memory.Manager
	rag         *rag.Pipeline
}

func NewKnowledgeService(
	docRepo *repository.DocumentRepo,
	kbRepo *repository.KnowledgeBaseRepo,
	dirRepo *repository.DirectoryRepo,
	msgRepo *repository.MessageRepo,
	userRepo *repository.UserRepo,
	tenantRepo *repository.TenantRepo,
	mem *memory.Manager,
	pipeline *rag.Pipeline,
) *KnowledgeService {
	return &KnowledgeService{
		docRepo:    docRepo,
		kbRepo:     kbRepo,
		dirRepo:    dirRepo,
		msgRepo:    msgRepo,
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
		mem:        mem,
		rag:        pipeline,
	}
}

// SetSpeechUsage 注入语音用量仓储，用于限制每天的语音输入和文字转语音次数。
func (s *KnowledgeService) SetSpeechUsage(repo *repository.SpeechUsageRepo) {
	if s != nil {
		s.speechUsage = repo
	}
}

func requireActor(userID string, tenantID uint) (Actor, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return Actor{}, fmt.Errorf("user_id is required")
	}
	if tenantID == 0 {
		return Actor{}, fmt.Errorf("tenant_id is required")
	}
	return Actor{UserID: userID, TenantID: tenantID}, nil
}

// UploadPolicy 返回租户上传配额和已有文件数。
func (s *KnowledgeService) UploadPolicy(tenantID uint) (model.Tenant, int64, error) {
	if s.tenantRepo == nil || tenantID == 0 {
		return model.Tenant{}, 0, nil
	}
	t, err := s.tenantRepo.GetByID(tenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.Tenant{}, 0, fmt.Errorf("租户不存在")
		}
		return model.Tenant{}, 0, err
	}
	n, err := s.docRepo.CountByTenant(tenantID)
	if err != nil {
		return model.Tenant{}, 0, err
	}
	return *t, n, nil
}

func (s *KnowledgeService) fillKBUsernames(kbs []*model.KnowledgeBase) {
	if s.userRepo == nil || len(kbs) == 0 {
		return
	}
	ids := make([]string, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil || kb.UserID == "" {
			continue
		}
		ids = append(ids, kb.UserID)
	}
	names, err := s.userRepo.MapUsernameByAuthIDs(ids)
	if err != nil || len(names) == 0 {
		return
	}
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		if name, ok := names[kb.UserID]; ok {
			kb.Username = name
		}
	}
}

func (s *KnowledgeService) fillDocUsernames(docs []*model.Document) {
	if s.userRepo == nil || len(docs) == 0 {
		return
	}
	ids := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc == nil || doc.UserID == "" {
			continue
		}
		ids = append(ids, doc.UserID)
	}
	names, err := s.userRepo.MapUsernameByAuthIDs(ids)
	if err != nil || len(names) == 0 {
		return
	}
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		if name, ok := names[doc.UserID]; ok {
			doc.Username = name
		}
	}
}

// requireKBAccess 同租户可读
func (s *KnowledgeService) requireKBAccess(kbID uint, actor Actor) (*model.KnowledgeBase, error) {
	kb, err := s.kbRepo.GetByID(kbID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("知识库不存在")
		}
		return nil, err
	}
	if kb.TenantID != actor.TenantID {
		return nil, ErrForbidden
	}
	return kb, nil
}

// requireKBWrite 同租户且属主可写/删
func (s *KnowledgeService) requireKBWrite(kbID uint, actor Actor) (*model.KnowledgeBase, error) {
	kb, err := s.requireKBAccess(kbID, actor)
	if err != nil {
		return nil, err
	}
	if kb.UserID != actor.UserID {
		return nil, ErrForbidden
	}
	return kb, nil
}

// requireDocAccess 同租户可读
func (s *KnowledgeService) requireDocAccess(docID uint, actor Actor) (*model.Document, error) {
	doc, err := s.docRepo.GetByID(docID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("文档不存在")
		}
		return nil, err
	}
	if doc.TenantID != actor.TenantID {
		return nil, ErrForbidden
	}
	return doc, nil
}

// requireDocWrite 同租户且上传者可删
func (s *KnowledgeService) requireDocWrite(docID uint, actor Actor) (*model.Document, error) {
	doc, err := s.requireDocAccess(docID, actor)
	if err != nil {
		return nil, err
	}
	if doc.UserID != actor.UserID {
		return nil, ErrForbidden
	}
	return doc, nil
}

// requireDirAccess 同租户可读（经所属 KB）
func (s *KnowledgeService) requireDirAccess(dirID uint, actor Actor) (*model.Directory, error) {
	dir, err := s.dirRepo.GetByID(dirID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("目录不存在")
		}
		return nil, err
	}
	if dir.TenantID != actor.TenantID {
		// 兼容旧数据 tenant_id=0：回退校验 KB
		if dir.TenantID != 0 {
			return nil, ErrForbidden
		}
		if _, err := s.requireKBAccess(dir.KnowledgeBaseID, actor); err != nil {
			return nil, err
		}
		return dir, nil
	}
	return dir, nil
}

// requireDirWrite 同租户且 KB 属主可改目录
func (s *KnowledgeService) requireDirWrite(dirID uint, actor Actor) (*model.Directory, error) {
	dir, err := s.requireDirAccess(dirID, actor)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireKBWrite(dir.KnowledgeBaseID, actor); err != nil {
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
	TenantID        uint
	Title           string
	KnowledgeBaseID uint
	DirectoryID     *uint
}

// CreateKnowledgeBase 创建知识库
func (s *KnowledgeService) CreateKnowledgeBase(userID string, tenantID uint, name, description string) (*model.KnowledgeBase, error) {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	kb := &model.KnowledgeBase{
		TenantID:    actor.TenantID,
		UserID:      actor.UserID,
		Name:        name,
		Description: description,
	}
	if err := s.kbRepo.Create(kb); err != nil {
		return nil, err
	}
	s.fillKBUsernames([]*model.KnowledgeBase{kb})
	return kb, nil
}

func (s *KnowledgeService) ListKnowledgeBases(userID string, tenantID uint, page, pageSize int) ([]model.KnowledgeBase, int64, error) {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 10
	}
	list, total, err := s.kbRepo.ListByTenant(actor.TenantID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	ptrs := make([]*model.KnowledgeBase, len(list))
	for i := range list {
		ptrs[i] = &list[i]
	}
	s.fillKBUsernames(ptrs)
	return list, total, nil
}

func (s *KnowledgeService) GetKnowledgeBase(id uint, userID string, tenantID uint) (*model.KnowledgeBase, error) {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	kb, err := s.requireKBAccess(id, actor)
	if err != nil {
		return nil, err
	}
	s.fillKBUsernames([]*model.KnowledgeBase{kb})
	return kb, nil
}

func (s *KnowledgeService) UpdateKnowledgeBase(id uint, userID string, tenantID uint, name, description string) (*model.KnowledgeBase, error) {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	kb, err := s.requireKBWrite(id, actor)
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
	s.fillKBUsernames([]*model.KnowledgeBase{kb})
	return kb, nil
}

func (s *KnowledgeService) DeleteKnowledgeBase(id uint, userID string, tenantID uint) error {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return err
	}
	if _, err := s.requireKBWrite(id, actor); err != nil {
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
	TenantID        uint
	KnowledgeBaseID uint
	ParentID        *uint
	Name            string
	Description     string
	SortOrder       int
}

func (s *KnowledgeService) CreateDirectory(in CreateDirectoryInput) (*model.Directory, error) {
	actor, err := requireActor(in.UserID, in.TenantID)
	if err != nil {
		return nil, err
	}
	if in.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	// 同租户成员均可在共享 KB 下建目录
	kb, err := s.requireKBAccess(in.KnowledgeBaseID, actor)
	if err != nil {
		return nil, err
	}

	if in.ParentID != nil {
		parent, err := s.requireDirAccess(*in.ParentID, actor)
		if err != nil {
			return nil, fmt.Errorf("parent directory: %w", err)
		}
		if parent.KnowledgeBaseID != in.KnowledgeBaseID {
			return nil, fmt.Errorf("parent directory 不属于该知识库")
		}
	}

	dir := &model.Directory{
		TenantID:        kb.TenantID,
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

func (s *KnowledgeService) ListDirectoryTree(kbID uint, userID string, tenantID uint) ([]*model.DirectoryNode, error) {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireKBAccess(kbID, actor); err != nil {
		return nil, err
	}
	dirs, err := s.dirRepo.ListByKnowledgeBase(kbID)
	if err != nil {
		return nil, err
	}
	return repository.BuildDirectoryTree(dirs), nil
}

func (s *KnowledgeService) UpdateDirectory(id uint, userID string, tenantID uint, name, description string, parentID *uint, sortOrder *int) (*model.Directory, error) {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	dir, err := s.requireDirWrite(id, actor)
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
			parent, err := s.requireDirAccess(*parentID, actor)
			if err != nil {
				return nil, fmt.Errorf("parent directory: %w", err)
			}
			if parent.KnowledgeBaseID != dir.KnowledgeBaseID {
				return nil, fmt.Errorf("parent directory 不属于同一知识库")
			}
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

func (s *KnowledgeService) DeleteDirectory(id uint, userID string, tenantID uint) error {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return err
	}
	if _, err := s.requireDirWrite(id, actor); err != nil {
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
	maxBytes := tenant.UploadMaxFileSizeBytes()
	for _, fh := range fileHeaders {
		if maxBytes > 0 && fh.Size > maxBytes {
			return nil, fmt.Errorf("文件 %q 大小超过限制 %dMB", fh.Filename, tenant.UploadMaxFileSizeMB())
		}
	}
	if used+int64(len(fileHeaders)) > int64(maxFiles) {
		return nil, fmt.Errorf("已达到或将超过租户文件总数上限 %d（已有 %d）", maxFiles, used)
	}

	result := &ImportResult{
		Items: make([]ImportFileResult, 0, len(fileHeaders)),
		Total: len(fileHeaders),
	}
	seenMD5 := make(map[string]*model.Document, len(fileHeaders))

	for _, fh := range fileHeaders {
		title := ""
		if len(fileHeaders) == 1 {
			title = opts.Title
		}
		item, err := s.importOneFile(actor, kb.TenantID, kbID, opts.DirectoryID, title, fh, seenMD5)
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
	actor Actor,
	tenantID uint,
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

	path, err := s.rag.SaveUpload(tenantID, kbID, fileHeader.Filename, data)
	if err != nil {
		return nil, fmt.Errorf("save upload: %w", err)
	}

	doc := &model.Document{
		TenantID:        tenantID,
		UserID:          actor.UserID,
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
	actor, err := requireActor(filter.UserID, filter.TenantID)
	if err != nil {
		return nil, 0, err
	}
	// 列表按租户共享，不再强制按上传者过滤
	filter.TenantID = actor.TenantID
	filter.UserID = ""
	if filter.KnowledgeBaseID > 0 {
		if _, err := s.requireKBAccess(filter.KnowledgeBaseID, actor); err != nil {
			return nil, 0, err
		}
	}
	if filter.DirectoryID != nil && *filter.DirectoryID > 0 {
		if _, err := s.requireDirAccess(*filter.DirectoryID, actor); err != nil {
			return nil, 0, err
		}
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	list, total, err := s.docRepo.List(filter, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	ptrs := make([]*model.Document, len(list))
	for i := range list {
		ptrs[i] = &list[i]
	}
	s.fillDocUsernames(ptrs)
	return list, total, nil
}

func (s *KnowledgeService) GetDocument(id uint, userID string, tenantID uint) (*model.Document, error) {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	doc, err := s.requireDocAccess(id, actor)
	if err != nil {
		return nil, err
	}
	s.fillDocUsernames([]*model.Document{doc})
	return doc, nil
}

// ListIndexBuilds 返回文档的索引构建历史（租户内可读）。
func (s *KnowledgeService) ListIndexBuilds(docID uint, userID string, tenantID uint) ([]model.DocumentIndexBuild, error) {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireDocAccess(docID, actor); err != nil {
		return nil, err
	}
	return s.docRepo.ListIndexBuilds(docID, 100)
}

// DeleteDocument 删除文档并级联清理向量索引与本地文件（属主）
func (s *KnowledgeService) DeleteDocument(ctx context.Context, id uint, userID string, tenantID uint) error {
	if id == 0 {
		return fmt.Errorf("无效的文档 ID")
	}
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return err
	}
	if _, err := s.requireDocWrite(id, actor); err != nil {
		return err
	}
	return s.rag.DeleteDocument(ctx, id)
}

// DeleteDocuments 批量删除文档（单项失败不中断，结果汇总返回）
func (s *KnowledgeService) DeleteDocuments(ctx context.Context, ids []uint, userID string, tenantID uint) (*DeleteDocumentsResult, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("ids 不能为空")
	}
	if _, err := requireActor(userID, tenantID); err != nil {
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

		if err := s.DeleteDocument(ctx, id, userID, tenantID); err != nil {
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

// ReindexDocuments 批量触发已导入文档的重新索引构建（异步）；同租户可读即可重建
func (s *KnowledgeService) ReindexDocuments(ids []uint, userID string, tenantID uint) (*ReindexResult, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("ids 不能为空")
	}
	actor, err := requireActor(userID, tenantID)
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

		doc, err := s.requireDocAccess(id, actor)
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
	actor, err := requireActor(req.UserID, req.TenantID)
	if err != nil {
		return err
	}
	req.UserID = actor.UserID
	req.TenantID = actor.TenantID
	if req.SessionID == "" {
		req.SessionID = uuid.NewString()
	}
	if s.mem != nil {
		if err := s.mem.EnsureNewSessionAllowed(actor.TenantID, req.SessionID); err != nil {
			return err
		}
		if err := s.mem.EnsureTurnAllowed(actor.TenantID, req.SessionID); err != nil {
			return err
		}
	}

	// 共享知识库：按租户过滤，不按上传者 user_id 收窄
	filter := &rag.RetrieveFilter{
		TenantID: strconv.FormatUint(uint64(actor.TenantID), 10),
	}
	if req.KnowledgeBaseID > 0 {
		if _, err := s.requireKBAccess(req.KnowledgeBaseID, actor); err != nil {
			return err
		}
		filter.KnowledgeBaseID = strconv.FormatUint(uint64(req.KnowledgeBaseID), 10)
	}
	if req.DirectoryID != nil && *req.DirectoryID > 0 {
		if _, err := s.requireDirAccess(*req.DirectoryID, actor); err != nil {
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

func (s *KnowledgeService) AgentQueryStream(ctx context.Context, req rag.QueryRequest, onEvent rag.StreamHandler) error {
	if err := s.prepareQueryRequest(&req); err != nil {
		return err
	}
	return s.rag.AgentQueryStream(ctx, req, onEvent)
}

func (s *KnowledgeService) GetHistory(sessionID, userID string, tenantID uint) ([]model.Message, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	actor, err := requireActor(userID, tenantID)
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
	// 会话仍按个人隔离；同时校验租户
	if msgs[0].UserID != actor.UserID {
		return nil, ErrForbidden
	}
	if msgs[0].TenantID != 0 && msgs[0].TenantID != actor.TenantID {
		return nil, ErrForbidden
	}
	out := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.UserID == actor.UserID && (m.TenantID == 0 || m.TenantID == actor.TenantID) {
			out = append(out, m)
		}
	}
	return out, nil
}

// ListSessions 列出当前用户在指定知识库/目录下的历史会话
func (s *KnowledgeService) ListSessions(userID string, tenantID, knowledgeBaseID uint, directoryID *uint) ([]model.Conversation, error) {
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	if knowledgeBaseID == 0 {
		return nil, fmt.Errorf("knowledge_base_id is required")
	}
	if _, err := s.requireKBAccess(knowledgeBaseID, actor); err != nil {
		return nil, err
	}
	if directoryID != nil && *directoryID > 0 {
		if _, err := s.requireDirAccess(*directoryID, actor); err != nil {
			return nil, err
		}
	}
	return s.mem.ListSessions(actor.TenantID, actor.UserID, knowledgeBaseID, directoryID)
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
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".bmp":
		return "image/bmp"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}
