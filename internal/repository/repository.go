package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewPostgres(cfg config.PostgresConfig) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres sql db: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), cfg.ConnectTimeout())
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	if err := db.AutoMigrate(
		&model.Tenant{},
		&model.User{},
		&model.KnowledgeBase{},
		&model.Directory{},
		&model.Document{},
		&model.DocumentIndexBuild{},
		&model.Conversation{},
		&model.Message{},
		&model.TenantSpeechUsage{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}
	if err := MigrateIndexBuilds(db); err != nil {
		return nil, fmt.Errorf("migrate index builds: %w", err)
	}
	return db, nil
}

type DocumentListFilter struct {
	TenantID        uint
	UserID          string // 可选：仅列自己上传的文档
	KnowledgeBaseID uint
	DirectoryID     *uint
	Status          string // 可选：按文档状态筛选
}

type DocumentRepo struct {
	db *gorm.DB
}

func NewDocumentRepo(db *gorm.DB) *DocumentRepo {
	return &DocumentRepo{db: db}
}

func (r *DocumentRepo) Create(doc *model.Document) error {
	return r.db.Create(doc).Error
}

func (r *DocumentRepo) GetByID(id uint) (*model.Document, error) {
	var doc model.Document
	if err := r.db.First(&doc, id).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

// GetByKnowledgeBaseAndMD5 按知识库内内容 MD5 查找已导入文档
func (r *DocumentRepo) GetByKnowledgeBaseAndMD5(kbID uint, md5 string) (*model.Document, error) {
	var doc model.Document
	err := r.db.Where("knowledge_base_id = ? AND content_md5 = ?", kbID, md5).First(&doc).Error
	if err != nil {
		return nil, err
	}
	return &doc, nil
}

func (r *DocumentRepo) List(filter DocumentListFilter, limit, offset int) ([]model.Document, int64, error) {
	var (
		docs  []model.Document
		total int64
	)
	q := r.db.Model(&model.Document{})
	if filter.TenantID > 0 {
		q = q.Where("tenant_id = ?", filter.TenantID)
	}
	if filter.UserID != "" {
		q = q.Where("user_id = ?", filter.UserID)
	}
	if filter.KnowledgeBaseID > 0 {
		q = q.Where("knowledge_base_id = ?", filter.KnowledgeBaseID)
	}
	if filter.DirectoryID != nil {
		q = q.Where("directory_id = ?", *filter.DirectoryID)
	}
	if filter.Status != "" {
		q = q.Where("status = ?", filter.Status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("id desc").Limit(limit).Offset(offset).Find(&docs).Error; err != nil {
		return nil, 0, err
	}
	return docs, total, nil
}

func (r *DocumentRepo) CountByTenant(tenantID uint) (int64, error) {
	var n int64
	err := r.db.Model(&model.Document{}).Where("tenant_id = ?", tenantID).Count(&n).Error
	return n, err
}

func (r *DocumentRepo) CountByKnowledgeBase(kbID uint) (int64, error) {
	var n int64
	err := r.db.Model(&model.Document{}).Where("knowledge_base_id = ?", kbID).Count(&n).Error
	return n, err
}

func (r *DocumentRepo) CountByDirectory(dirID uint) (int64, error) {
	var n int64
	err := r.db.Model(&model.Document{}).Where("directory_id = ?", dirID).Count(&n).Error
	return n, err
}

func (r *DocumentRepo) UpdateStatus(id uint, status model.DocumentStatus, chunkCount int, errMsg string) error {
	updates := map[string]any{
		"status":      status,
		"chunk_count": chunkCount,
		"error_msg":   errMsg,
	}
	// 索引构建结束（就绪/失败）时记录操作时间；indexing/pending 不覆盖
	if status == model.DocumentStatusReady || status == model.DocumentStatusFailed {
		now := time.Now()
		updates["last_indexed_at"] = now
	}
	// UpdateColumns 不触发 updated_at 自动刷新，避免索引构建改动更新时间
	return r.db.Model(&model.Document{}).Where("id = ?", id).UpdateColumns(updates).Error
}

// ListIDsByStatuses 按状态列出文档 ID（启动时回收 pending/indexing）
func (r *DocumentRepo) ListIDsByStatuses(statuses ...model.DocumentStatus) ([]uint, error) {
	if len(statuses) == 0 {
		return nil, nil
	}
	var ids []uint
	err := r.db.Model(&model.Document{}).
		Where("status IN ?", statuses).
		Order("id asc").
		Pluck("id", &ids).Error
	return ids, err
}

func (r *DocumentRepo) Delete(id uint) error {
	if err := r.db.Where("document_id = ?", id).Delete(&model.DocumentIndexBuild{}).Error; err != nil {
		return err
	}
	return r.db.Delete(&model.Document{}, id).Error
}

type KnowledgeBaseRepo struct {
	db *gorm.DB
}

func NewKnowledgeBaseRepo(db *gorm.DB) *KnowledgeBaseRepo {
	return &KnowledgeBaseRepo{db: db}
}

func (r *KnowledgeBaseRepo) Create(kb *model.KnowledgeBase) error {
	return r.db.Create(kb).Error
}

func (r *KnowledgeBaseRepo) GetByID(id uint) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	if err := r.db.First(&kb, id).Error; err != nil {
		return nil, err
	}
	return &kb, nil
}

func (r *KnowledgeBaseRepo) ListByUser(userID string) ([]model.KnowledgeBase, error) {
	var list []model.KnowledgeBase
	q := r.db.Model(&model.KnowledgeBase{}).Order("id desc")
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ListByTenant 分页列出租户内知识库（同租户共享可读）
func (r *KnowledgeBaseRepo) ListByTenant(tenantID uint, limit, offset int) ([]model.KnowledgeBase, int64, error) {
	var (
		list  []model.KnowledgeBase
		total int64
	)
	q := r.db.Model(&model.KnowledgeBase{}).Where("tenant_id = ?", tenantID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("id desc").Limit(limit).Offset(offset).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *KnowledgeBaseRepo) Update(kb *model.KnowledgeBase) error {
	return r.db.Model(kb).Updates(map[string]any{
		"name":        kb.Name,
		"description": kb.Description,
	}).Error
}

func (r *KnowledgeBaseRepo) Delete(id uint) error {
	return r.db.Delete(&model.KnowledgeBase{}, id).Error
}

// GetOrCreateDefault 获取用户在租户下的默认知识库，不存在则创建
func (r *KnowledgeBaseRepo) GetOrCreateDefault(tenantID uint, userID string) (*model.KnowledgeBase, error) {
	var kb model.KnowledgeBase
	err := r.db.Where("tenant_id = ? AND user_id = ? AND name = ?", tenantID, userID, "默认知识库").First(&kb).Error
	if err == nil {
		return &kb, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	kb = model.KnowledgeBase{
		TenantID:    tenantID,
		UserID:      userID,
		Name:        "默认知识库",
		Description: "系统自动创建的默认知识库",
	}
	if err := r.db.Create(&kb).Error; err != nil {
		return nil, err
	}
	return &kb, nil
}

type DirectoryRepo struct {
	db *gorm.DB
}

func NewDirectoryRepo(db *gorm.DB) *DirectoryRepo {
	return &DirectoryRepo{db: db}
}

func (r *DirectoryRepo) Create(dir *model.Directory) error {
	return r.db.Create(dir).Error
}

func (r *DirectoryRepo) GetByID(id uint) (*model.Directory, error) {
	var dir model.Directory
	if err := r.db.First(&dir, id).Error; err != nil {
		return nil, err
	}
	return &dir, nil
}

func (r *DirectoryRepo) ListByKnowledgeBase(kbID uint) ([]model.Directory, error) {
	var list []model.Directory
	err := r.db.Where("knowledge_base_id = ?", kbID).
		Order("sort_order asc, id asc").
		Find(&list).Error
	return list, err
}

func (r *DirectoryRepo) Update(dir *model.Directory) error {
	return r.db.Model(dir).Updates(map[string]any{
		"name":        dir.Name,
		"description": dir.Description,
		"parent_id":   dir.ParentID,
		"sort_order":  dir.SortOrder,
	}).Error
}

func (r *DirectoryRepo) Delete(id uint) error {
	return r.db.Delete(&model.Directory{}, id).Error
}

func (r *DirectoryRepo) CountChildren(parentID uint) (int64, error) {
	var n int64
	err := r.db.Model(&model.Directory{}).Where("parent_id = ?", parentID).Count(&n).Error
	return n, err
}

// CollectSelfAndDescendantIDs 收集目录自身及全部子孙目录 ID
func (r *DirectoryRepo) CollectSelfAndDescendantIDs(rootID uint) ([]uint, error) {
	var root model.Directory
	if err := r.db.First(&root, rootID).Error; err != nil {
		return nil, err
	}
	dirs, err := r.ListByKnowledgeBase(root.KnowledgeBaseID)
	if err != nil {
		return nil, err
	}

	childrenMap := map[uint][]uint{}
	for _, d := range dirs {
		if d.ParentID != nil {
			childrenMap[*d.ParentID] = append(childrenMap[*d.ParentID], d.ID)
		}
	}

	var out []uint
	var walk func(id uint)
	walk = func(id uint) {
		out = append(out, id)
		for _, cid := range childrenMap[id] {
			walk(cid)
		}
	}
	walk(rootID)
	return out, nil
}

// BuildTree 将扁平目录列表组装为树
func BuildDirectoryTree(dirs []model.Directory) []*model.DirectoryNode {
	nodeMap := make(map[uint]*model.DirectoryNode, len(dirs))
	for i := range dirs {
		nodeMap[dirs[i].ID] = &model.DirectoryNode{Directory: dirs[i]}
	}
	var roots []*model.DirectoryNode
	for i := range dirs {
		n := nodeMap[dirs[i].ID]
		if dirs[i].ParentID == nil {
			roots = append(roots, n)
			continue
		}
		if parent, ok := nodeMap[*dirs[i].ParentID]; ok {
			parent.Children = append(parent.Children, n)
		} else {
			roots = append(roots, n)
		}
	}
	return roots
}

type ConversationRepo struct {
	db *gorm.DB
}

func NewConversationRepo(db *gorm.DB) *ConversationRepo {
	return &ConversationRepo{db: db}
}

func (r *ConversationRepo) GetOrCreate(
	tenantID uint,
	userID, sessionID, title string,
	knowledgeBaseID uint,
	directoryID *uint,
) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.Where("session_id = ?", sessionID).First(&conv).Error
	if err == nil {
		// 回填历史会话缺失的知识库/目录归属
		needUpdate := false
		if conv.KnowledgeBaseID == 0 && knowledgeBaseID > 0 {
			conv.KnowledgeBaseID = knowledgeBaseID
			needUpdate = true
		}
		if conv.DirectoryID == nil && directoryID != nil {
			conv.DirectoryID = directoryID
			needUpdate = true
		}
		if title != "" && conv.Title == "" {
			conv.Title = title
			needUpdate = true
		}
		if needUpdate {
			_ = r.db.Model(&conv).Updates(map[string]any{
				"knowledge_base_id": conv.KnowledgeBaseID,
				"directory_id":      conv.DirectoryID,
				"title":             conv.Title,
			}).Error
		}
		return &conv, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	conv = model.Conversation{
		TenantID:        tenantID,
		UserID:          userID,
		SessionID:       sessionID,
		KnowledgeBaseID: knowledgeBaseID,
		DirectoryID:     directoryID,
		Title:           title,
	}
	if err := r.db.Create(&conv).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}

func (r *ConversationRepo) CountCreatedSince(tenantID uint, since time.Time) (int64, error) {
	var n int64
	err := r.db.Model(&model.Conversation{}).
		Where("tenant_id = ? AND created_at >= ?", tenantID, since).
		Count(&n).Error
	return n, err
}

func (r *ConversationRepo) GetBySessionID(sessionID string) (*model.Conversation, error) {
	var conv model.Conversation
	if err := r.db.Where("session_id = ?", sessionID).First(&conv).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}

// ConversationListFilter 会话列表过滤
type ConversationListFilter struct {
	TenantID        uint
	UserID          string
	KnowledgeBaseID uint
	// DirectoryID 非 nil 时按目录精确匹配；nil 表示仅查未绑定目录的会话
	DirectoryID *uint
	// MatchNullDirectory 为 true 且 DirectoryID==nil 时，匹配 directory_id IS NULL
	MatchNullDirectory bool
}

func (r *ConversationRepo) List(filter ConversationListFilter, limit int) ([]model.Conversation, error) {
	q := r.db.Model(&model.Conversation{}).
		Where("tenant_id = ? AND user_id = ?", filter.TenantID, filter.UserID)
	if filter.KnowledgeBaseID > 0 {
		q = q.Where("knowledge_base_id = ?", filter.KnowledgeBaseID)
	}
	if filter.DirectoryID != nil {
		q = q.Where("directory_id = ?", *filter.DirectoryID)
	} else if filter.MatchNullDirectory {
		q = q.Where("directory_id IS NULL")
	}
	if limit <= 0 {
		limit = 100
	}
	var list []model.Conversation
	if err := q.Order("updated_at desc").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

type MessageRepo struct {
	db *gorm.DB
}

func NewMessageRepo(db *gorm.DB) *MessageRepo {
	return &MessageRepo{db: db}
}

func (r *MessageRepo) Create(msg *model.Message) error {
	return r.db.Create(msg).Error
}

func (r *MessageRepo) CountUserBySession(sessionID string) (int64, error) {
	var n int64
	err := r.db.Model(&model.Message{}).
		Where("session_id = ? AND role = ?", sessionID, model.RoleUser).
		Count(&n).Error
	return n, err
}

func (r *MessageRepo) ListBySession(sessionID string, limit int) ([]model.Message, error) {
	var msgs []model.Message
	q := r.db.Where("session_id = ?", sessionID).Order("id desc")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&msgs).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

type TenantRepo struct {
	db *gorm.DB
}

func NewTenantRepo(db *gorm.DB) *TenantRepo {
	return &TenantRepo{db: db}
}

func (r *TenantRepo) Create(t *model.Tenant) error {
	return r.db.Create(t).Error
}

// Transaction 在同一事务中操作租户与用户。
func (r *TenantRepo) Transaction(fn func(tenants *TenantRepo, users *UserRepo) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(&TenantRepo{db: tx}, &UserRepo{db: tx})
	})
}

func (r *TenantRepo) GetByCode(code string) (*model.Tenant, error) {
	var t model.Tenant
	if err := r.db.Where("code = ?", code).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TenantRepo) GetByID(id uint) (*model.Tenant, error) {
	var t model.Tenant
	if err := r.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TenantRepo) Count() (int64, error) {
	var n int64
	if err := r.db.Model(&model.Tenant{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// TenantUserCount 租户及其用户数。
type TenantUserCount struct {
	Code              string
	Name              string
	CreatedAt         time.Time
	UserCount         int64
	MaxFiles          int
	MaxFileSizeMB     int
	MaxSessions       int
	MaxTurns          int
	MaxVoiceInputs    int
	MaxTTS            int
	FileCount         int64
	TodaySessionCount int64
	TodayVoiceInputs  int
	TodayTTSCount     int
}

// ListWithUserCount 分页列出全部租户，并统计每个租户下的用户数。
func (r *TenantRepo) ListWithUserCount(limit, offset int) ([]TenantUserCount, int64, error) {
	var total int64
	if err := r.db.Model(&model.Tenant{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	day := model.TodayShanghai()
	since := model.StartOfTodayShanghai()
	var rows []TenantUserCount
	err := r.db.Table("tenants").
		Select(`tenants.code AS code, tenants.name AS name, tenants.created_at AS created_at, tenants.max_files AS max_files, tenants.max_file_size_mb AS max_file_size_mb, tenants.max_sessions AS max_sessions, tenants.max_turns AS max_turns, tenants.max_voice_inputs AS max_voice_inputs, tenants.max_tts AS max_tts, COUNT(users.id) AS user_count,
			COALESCE((SELECT COUNT(*) FROM documents WHERE documents.tenant_id = tenants.id), 0) AS file_count,
			COALESCE((SELECT COUNT(*) FROM conversations WHERE conversations.tenant_id = tenants.id AND conversations.created_at >= ?), 0) AS today_session_count,
			COALESCE((SELECT tenant_speech_usages.voice_inputs FROM tenant_speech_usages WHERE tenant_speech_usages.tenant_id = tenants.id AND tenant_speech_usages.day = ?), 0) AS today_voice_inputs,
			COALESCE((SELECT tenant_speech_usages.tts_count FROM tenant_speech_usages WHERE tenant_speech_usages.tenant_id = tenants.id AND tenant_speech_usages.day = ?), 0) AS today_tts_count`,
			since, day, day).
		Joins("LEFT JOIN users ON users.tenant_id = tenants.id").
		Group("tenants.id").
		Order("tenants.created_at DESC, tenants.id DESC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// UpdateUploadLimits 更新租户的文件、会话、轮次和语音次数上限。
func (r *TenantRepo) UpdateUploadLimits(code string, maxFiles, maxFileSizeMB, maxSessions, maxTurns, maxVoiceInputs, maxTTS int) error {
	res := r.db.Model(&model.Tenant{}).Where("code = ?", code).Updates(map[string]any{
		"max_files":        maxFiles,
		"max_file_size_mb": maxFileSizeMB,
		"max_sessions":     maxSessions,
		"max_turns":        maxTurns,
		"max_voice_inputs": maxVoiceInputs,
		"max_tts":          maxTTS,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// EnsureDefault 确保存在 code=default 的默认租户
func (r *TenantRepo) EnsureDefault() (*model.Tenant, error) {
	t, err := r.GetByCode("default")
	if err == nil {
		return t, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	t = &model.Tenant{
		Code:           "default",
		Name:           "默认租户",
		MaxFiles:       model.DefaultTenantMaxFiles,
		MaxFileSizeMB:  model.DefaultTenantMaxFileSizeMB,
		MaxSessions:    model.DefaultTenantMaxSessions,
		MaxTurns:       model.DefaultTenantMaxTurns,
		MaxVoiceInputs: model.DefaultTenantMaxVoiceInputs,
		MaxTTS:         model.DefaultTenantMaxTTS,
	}
	if err := r.Create(t); err != nil {
		return nil, err
	}
	return t, nil
}

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(u *model.User) error {
	return r.db.Create(u).Error
}

func (r *UserRepo) GetByTenantUsername(tenantID uint, username string) (*model.User, error) {
	var u model.User
	if err := r.db.Where("tenant_id = ? AND username = ?", tenantID, username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepo) GetByID(id uint) (*model.User, error) {
	var u model.User
	if err := r.db.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// ListByTenant 分页列出租户下的用户，按创建时间倒序。
func (r *UserRepo) ListByTenant(tenantID uint, limit, offset int) ([]model.User, int64, error) {
	var (
		list  []model.User
		total int64
	)
	q := r.db.Model(&model.User{}).Where("tenant_id = ?", tenantID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at desc, id desc").Limit(limit).Offset(offset).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// SetLoginEnabled 更新租户内指定用户的登录开关。
func (r *UserRepo) SetLoginEnabled(tenantID uint, username string, enabled bool) error {
	res := r.db.Model(&model.User{}).
		Where("tenant_id = ? AND username = ?", tenantID, username).
		Update("login_enabled", enabled)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// MapUsernameByAuthIDs 按业务 user_id（数字字符串）批量查用户名。
func (r *UserRepo) MapUsernameByAuthIDs(authIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(authIDs))
	ids := make([]uint, 0, len(authIDs))
	seen := make(map[uint]struct{}, len(authIDs))
	for _, raw := range authIDs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id64, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id64 == 0 {
			continue
		}
		id := uint(id64)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out, nil
	}
	var users []model.User
	if err := r.db.Select("id", "username").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		out[u.AuthUserID()] = u.Username
	}
	return out, nil
}
