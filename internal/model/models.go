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
	ID              uint   `gorm:"primaryKey" json:"id"`
	TenantID        uint   `gorm:"index;not null;default:0" json:"tenant_id"`
	UserID          string `gorm:"size:64;index;not null" json:"user_id"` // 上传者/属主
	Username        string `gorm:"-" json:"username,omitempty"`           // 上传者用户名（查询时填充）
	KnowledgeBaseID uint   `gorm:"index;index:idx_kb_content_md5,priority:1;not null;default:0" json:"knowledge_base_id"`
	DirectoryID     *uint  `gorm:"index" json:"directory_id"`
	Title           string `gorm:"size:256;not null" json:"title"`
	FileName        string `gorm:"size:256;not null" json:"file_name"`
	FilePath        string `gorm:"size:512;not null" json:"file_path"`
	// SourceAvailable 本地上传文件是否仍在；索引成功后会删除源文件。不入库。
	SourceAvailable bool           `gorm:"-" json:"source_available"`
	ContentMD5      string         `gorm:"size:32;index:idx_kb_content_md5,priority:2;not null;default:''" json:"content_md5"`
	ContentType     string         `gorm:"size:128" json:"content_type"`
	FileSize        int64          `json:"file_size"`
	Status          DocumentStatus `gorm:"size:32;index;not null;default:pending" json:"status"`
	ChunkCount      int            `json:"chunk_count"`
	ErrorMsg        string         `gorm:"type:text" json:"error_msg,omitempty"`
	LastIndexedAt   *time.Time     `json:"last_indexed_at,omitempty"` // 上次索引构建完成时间（成功或失败）
	// Recall 是按配置的 TopK 落库的文档召回率：rank≤K 命中的已标注问题数 / 把本文档标为相关的问题数。
	// 没有相关标注时为 null，不是 0。不进入检索分数。
	Recall         *float64 `json:"recall"`
	RecallK        int      `gorm:"not null;default:0" json:"recall_k"`
	LabeledQueries int      `gorm:"not null;default:0" json:"labeled_queries"`
	HitQueries     int      `gorm:"not null;default:0" json:"hit_queries"`
	// CitedCount 是历史回答里用合法 [n] 引用过该文档的次数。同一条回答的多个分块只计 1 次。
	CitedCount int `gorm:"not null;default:0" json:"cited_count"`
	// CitedChunks 是被引用片段的前三名，按引用次数从高到低。片段标号是切分时的 chunk_index，从 0 起。
	CitedChunks []CitedChunk `gorm:"type:jsonb;serializer:json" json:"cited_chunks"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

func (Document) TableName() string { return "documents" }

// CitedChunk 文档某个片段在历史回答中被引用的名次。
// Rank 从 1 起；次数相同则片段标号更小的排在前面。
type CitedChunk struct {
	Rank       int `json:"rank"`
	ChunkIndex int `json:"chunk_index"`
	Count      int `json:"count"`
}

// DocumentIndexBuild 文档一次索引构建的历史。入队时创建，成功或放弃重试时写入结束时间；重试不另开记录。
type DocumentIndexBuild struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	TenantID    uint           `gorm:"index;not null" json:"tenant_id"`
	DocumentID  uint           `gorm:"index;not null" json:"document_id"`
	Status      DocumentStatus `gorm:"size:32;index;not null;default:pending" json:"status"`
	TriggeredAt time.Time      `gorm:"not null" json:"triggered_at"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	ErrorMsg    string         `gorm:"type:text" json:"error_msg,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (DocumentIndexBuild) TableName() string { return "document_index_builds" }

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
	// Vote / Score 来自 message_feedbacks，不入库、不进入模型上下文。
	Vote  string `gorm:"-" json:"vote,omitempty"`
	Score int    `gorm:"-" json:"score,omitempty"`
	// RelevantDocIDs 来自 retrieval_labels，仅用户问题有值，不入库。
	RelevantDocIDs []string `gorm:"-" json:"relevant_doc_ids,omitempty"`
}

func (Message) TableName() string { return "messages" }

const (
	FeedbackVoteUp   = "up"
	FeedbackVoteDown = "down"
)

// MessageFeedback 用户对一条助手回答的点赞、点踩和 1–5 分评分。
// Vote 为空表示未表态；Score 为 0 表示未评分。再次提交同一项可取消。
type MessageFeedback struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"index;not null" json:"tenant_id"`
	UserID    string    `gorm:"size:64;index;not null" json:"user_id"`
	SessionID string    `gorm:"size:64;index;not null" json:"session_id"`
	MessageID uint      `gorm:"uniqueIndex;not null" json:"message_id"`
	Vote      string    `gorm:"size:8;not null;default:''" json:"vote"`
	Score     int       `gorm:"not null;default:0" json:"score"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (MessageFeedback) TableName() string { return "message_feedbacks" }

// RetrievalHit 一次问答里按检索原始顺序记下的召回片段。
// Rank 是该轮 TopK 内的 1-based 名次，不使用融合后的相似度分数。
// Cited 表示最终入库的回答用合法 [n] 引用了该片段；编号来自当次 sources，不解析日志。
type RetrievalHit struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	TenantID           uint      `gorm:"index:idx_hit_doc,priority:1;not null" json:"tenant_id"`
	UserID             string    `gorm:"size:64;not null" json:"user_id"`
	SessionID          string    `gorm:"size:64;index;not null" json:"session_id"`
	UserMessageID      uint      `gorm:"index;not null" json:"user_message_id"`
	AssistantMessageID uint      `gorm:"index;not null" json:"assistant_message_id"`
	KnowledgeBaseID    uint      `gorm:"index:idx_hit_doc,priority:2;not null;default:0" json:"knowledge_base_id"`
	Round              int       `gorm:"not null;default:1" json:"round"` // 1-based；线性 RAG 恒为 1，Agent 为第几轮 knowledge_retrieve
	Rank               int       `gorm:"not null" json:"rank"`            // 该轮原始召回顺序，1-based
	DocID              string    `gorm:"size:64;index:idx_hit_doc,priority:3;not null" json:"doc_id"`
	ChunkIndex         int       `gorm:"not null;default:0" json:"chunk_index"`
	Cited              bool      `gorm:"not null;default:false" json:"cited"`
	CreatedAt          time.Time `json:"created_at"`
}

func (RetrievalHit) TableName() string { return "retrieval_hits" }

// RetrievalLabel 某条用户问题的相关文档标注，作为文档召回率的分母。
type RetrievalLabel struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TenantID        uint      `gorm:"index:idx_label_doc,priority:1;not null" json:"tenant_id"`
	UserID          string    `gorm:"size:64;not null" json:"user_id"`
	SessionID       string    `gorm:"size:64;index;not null" json:"session_id"`
	UserMessageID   uint      `gorm:"uniqueIndex:idx_label_msg_doc,priority:1;not null" json:"user_message_id"`
	KnowledgeBaseID uint      `gorm:"index:idx_label_doc,priority:2;not null" json:"knowledge_base_id"`
	DocID           string    `gorm:"size:64;uniqueIndex:idx_label_msg_doc,priority:2;index:idx_label_doc,priority:3;not null" json:"doc_id"`
	CreatedAt       time.Time `json:"created_at"`
}

func (RetrievalLabel) TableName() string { return "retrieval_labels" }

// Tenant 租户（注册/登录时填写的租户 ID 对应 Code）
type Tenant struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Code           string    `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name           string    `gorm:"size:128;not null" json:"name"`
	MaxFiles       int       `gorm:"not null;default:5" json:"max_files"`
	MaxFileSizeMB  int       `gorm:"not null;default:5" json:"max_file_size_mb"`
	MaxSessions    int       `gorm:"not null;default:5" json:"max_sessions"`
	MaxTurns       int       `gorm:"not null;default:5" json:"max_turns"`
	MaxVoiceInputs int       `gorm:"not null;default:5" json:"max_voice_inputs"`
	MaxTTS         int       `gorm:"not null;default:5" json:"max_tts"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
	// DefaultTenantMaxVoiceInputs 租户默认每天语音输入次数
	DefaultTenantMaxVoiceInputs = 5
	// DefaultTenantMaxTTS 租户默认每天文字转语音次数
	DefaultTenantMaxTTS = 5
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

// VoiceInputMax 有效的每日语音输入次数。未配置时用默认值。
func (t Tenant) VoiceInputMax() int {
	if t.MaxVoiceInputs <= 0 {
		return DefaultTenantMaxVoiceInputs
	}
	return t.MaxVoiceInputs
}

// TTSMax 有效的每日文字转语音次数。未配置时用默认值。
func (t Tenant) TTSMax() int {
	if t.MaxTTS <= 0 {
		return DefaultTenantMaxTTS
	}
	return t.MaxTTS
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

// TenantSpeechUsage 租户当天的语音输入与文字转语音次数。
type TenantSpeechUsage struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	TenantID    uint      `gorm:"not null;uniqueIndex:idx_tenant_speech_day,priority:1" json:"tenant_id"`
	Day         string    `gorm:"size:10;not null;uniqueIndex:idx_tenant_speech_day,priority:2" json:"day"`
	VoiceInputs int       `gorm:"not null;default:0" json:"voice_inputs"`
	TTSCount    int       `gorm:"not null;default:0" json:"tts_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (TenantSpeechUsage) TableName() string { return "tenant_speech_usages" }

// ShanghaiLocation 北京时间。加载失败时用固定东八区。
func ShanghaiLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

// StartOfTodayShanghai 北京时间当天 0 点。
func StartOfTodayShanghai() time.Time {
	now := time.Now().In(ShanghaiLocation())
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// TodayShanghai 北京时间日期，格式 2006-01-02。
func TodayShanghai() string {
	return time.Now().In(ShanghaiLocation()).Format("2006-01-02")
}
