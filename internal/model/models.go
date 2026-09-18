package model

import (
	"strconv"
	"time"
)

// DocumentStatus 文档索引状态
type DocumentStatus string

const (
	DocumentStatusPending  DocumentStatus = "pending"
	DocumentStatusIndexing DocumentStatus = "indexing"
	DocumentStatusReady    DocumentStatus = "ready"
	DocumentStatusFailed   DocumentStatus = "failed"
)

// KnowledgeBase 知识库（顶层容器，租户内共享可读）
type KnowledgeBase struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	TenantID    uint      `gorm:"index;not null;default:0" json:"tenant_id"`
	UserID      string    `gorm:"size:64;index;not null" json:"user_id"` // 创建者/属主
	Username    string    `gorm:"-" json:"username,omitempty"`           // 创建者用户名（查询时填充）
	Name        string    `gorm:"size:128;not null" json:"name"`
	Description string    `gorm:"size:512" json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (KnowledgeBase) TableName() string { return "knowledge_bases" }

// Directory 知识库分类目录（树形，parent_id 为空表示根目录）
type Directory struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TenantID        uint      `gorm:"index;not null;default:0" json:"tenant_id"`
	KnowledgeBaseID uint      `gorm:"index;not null" json:"knowledge_base_id"`
	ParentID        *uint     `gorm:"index" json:"parent_id"`
	Name            string    `gorm:"size:128;not null" json:"name"`
	Description     string    `gorm:"size:512" json:"description"`
	SortOrder       int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (Directory) TableName() string { return "directories" }

// DirectoryNode 目录树节点（API 返回）
type DirectoryNode struct {
	Directory
	Children []*DirectoryNode `json:"children,omitempty"`
}

// Document 用户导入的知识库文档记录
type Document struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	TenantID        uint           `gorm:"index;not null;default:0" json:"tenant_id"`
	UserID          string         `gorm:"size:64;index;not null" json:"user_id"` // 上传者/属主
	Username        string         `gorm:"-" json:"username,omitempty"`           // 上传者用户名（查询时填充）
	KnowledgeBaseID uint           `gorm:"index;index:idx_kb_content_md5,priority:1;not null;default:0" json:"knowledge_base_id"`
	DirectoryID     *uint          `gorm:"index" json:"directory_id"`
	Title           string         `gorm:"size:256;not null" json:"title"`
	FileName        string         `gorm:"size:256;not null" json:"file_name"`
	FilePath        string         `gorm:"size:512;not null" json:"file_path"`
	ContentMD5      string         `gorm:"size:32;index:idx_kb_content_md5,priority:2;not null;default:''" json:"content_md5"`
	ContentType     string         `gorm:"size:128" json:"content_type"`
	FileSize        int64          `json:"file_size"`
	Status          DocumentStatus `gorm:"size:32;index;not null;default:pending" json:"status"`
	ChunkCount      int            `json:"chunk_count"`
	ErrorMsg        string         `gorm:"type:text" json:"error_msg,omitempty"`
	LastIndexedAt   *time.Time     `json:"last_indexed_at,omitempty"` // 上次索引构建完成时间（成功或失败）
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (Document) TableName() string { return "documents" }

// Conversation 会话（长期记忆的会话维度）
type Conversation struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TenantID        uint      `gorm:"index;not null;default:0" json:"tenant_id"`
	UserID          string    `gorm:"size:64;index;not null" json:"user_id"`
	SessionID       string    `gorm:"size:64;uniqueIndex;not null" json:"session_id"`
	KnowledgeBaseID uint      `gorm:"index;not null;default:0" json:"knowledge_base_id"`
	DirectoryID     *uint     `gorm:"index" json:"directory_id"`
	Title           string    `gorm:"size:256" json:"title"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (Conversation) TableName() string { return "conversations" }

// MessageRole 消息角色
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
)

// Message 长期对话记忆（PostgreSQL）
type Message struct {
	ID             uint        `gorm:"primaryKey" json:"id"`
	TenantID       uint        `gorm:"index;not null;default:0" json:"tenant_id"`
	ConversationID uint        `gorm:"index;not null" json:"conversation_id"`
	UserID         string      `gorm:"size:64;index;not null" json:"user_id"`
	SessionID      string      `gorm:"size:64;index;not null" json:"session_id"`
	Role           MessageRole `gorm:"size:32;not null" json:"role"`
	Content        string      `gorm:"type:text;not null" json:"content"`
	CreatedAt      time.Time   `json:"created_at"`
}

func (Message) TableName() string { return "messages" }

// Tenant 租户（注册/登录时填写的租户 ID 对应 Code）
type Tenant struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Code          string    `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name          string    `gorm:"size:128;not null" json:"name"`
	MaxFiles      int       `gorm:"not null;default:5" json:"max_files"`
	MaxFileSizeMB int       `gorm:"not null;default:5" json:"max_file_size_mb"`
	MaxSessions   int       `gorm:"not null;default:5" json:"max_sessions"`
	MaxTurns      int       `gorm:"not null;default:5" json:"max_turns"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (Tenant) TableName() string { return "tenants" }

const (
	// DefaultTenantMaxFiles 租户默认可上传的文件总数
	DefaultTenantMaxFiles = 5
	// DefaultTenantMaxFileSizeMB 租户默认单个文件大小上限（MB）
	DefaultTenantMaxFileSizeMB = 5
	// DefaultTenantMaxSessions 租户默认每天可新建的知识库问答会话数
	DefaultTenantMaxSessions = 5
	// DefaultTenantMaxTurns 租户默认单条会话的提问轮次上限
	DefaultTenantMaxTurns = 5
)

// UploadMaxFiles 有效的文件总数上限。未配置时用默认值。
func (t Tenant) UploadMaxFiles() int {
	if t.MaxFiles <= 0 {
		return DefaultTenantMaxFiles
	}
	return t.MaxFiles
}

// UploadMaxFileSizeMB 有效的单文件大小上限（MB）。未配置时用默认值。
func (t Tenant) UploadMaxFileSizeMB() int {
	if t.MaxFileSizeMB <= 0 {
		return DefaultTenantMaxFileSizeMB
	}
	return t.MaxFileSizeMB
}

// UploadMaxFileSizeBytes 有效的单文件字节上限。
func (t Tenant) UploadMaxFileSizeBytes() int64 {
	return int64(t.UploadMaxFileSizeMB()) * 1024 * 1024
}

// SessionMax 有效的每日新建会话上限。未配置时用默认值。
func (t Tenant) SessionMax() int {
	if t.MaxSessions <= 0 {
		return DefaultTenantMaxSessions
	}
	return t.MaxSessions
}

// TurnMax 有效的单条会话提问轮次上限。未配置时用默认值。
func (t Tenant) TurnMax() int {
	if t.MaxTurns <= 0 {
		return DefaultTenantMaxTurns
	}
	return t.MaxTurns
}

// User 租户下的登录用户
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TenantID     uint      `gorm:"index;not null;uniqueIndex:idx_tenant_username,priority:1" json:"tenant_id"`
	Username     string    `gorm:"size:64;not null;uniqueIndex:idx_tenant_username,priority:2" json:"username"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	DisplayName  string    `gorm:"size:128" json:"display_name"`
	LoginEnabled bool      `gorm:"not null;default:true" json:"login_enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string { return "users" }

// AuthUserID 业务表 user_id 字段使用的稳定字符串标识
func (u User) AuthUserID() string {
	return strconv.FormatUint(uint64(u.ID), 10)
}
