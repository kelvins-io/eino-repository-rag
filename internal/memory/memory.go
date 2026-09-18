package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// ChatTurn 单轮对话（短期记忆条目）
type ChatTurn struct {
	Role      model.MessageRole `json:"role"`
	Content   string            `json:"content"`
	Timestamp int64             `json:"timestamp"`
}

// Manager 统一管理短期（Redis）与长期（PostgreSQL）记忆
type Manager struct {
	rdb      *redis.Client
	msgRepo  *repository.MessageRepo
	convRepo *repository.ConversationRepo
	tenants  *repository.TenantRepo
	ttl      time.Duration
	shortMax int
	longMax  int

	tokenBudget    int
	keepRecent     int
	summaryEnabled bool
	summaryTrigger int
	summarizer     Summarizer
}

func NewManager(
	rdb *redis.Client,
	msgRepo *repository.MessageRepo,
	convRepo *repository.ConversationRepo,
	cfg config.MemoryConfig,
) *Manager {
	return &Manager{
		rdb:            rdb,
		msgRepo:        msgRepo,
		convRepo:       convRepo,
		ttl:            time.Duration(cfg.ShortTermTTLMinutes) * time.Minute,
		shortMax:       cfg.ShortTermMaxMessages,
		longMax:        cfg.LongTermMaxMessages,
		tokenBudget:    cfg.ContextTokenBudget,
		keepRecent:     cfg.KeepRecentMessages,
		summaryEnabled: cfg.SummaryEnabled,
		summaryTrigger: cfg.SummaryTriggerMessages,
	}
}

// SetTenantRepo 注入租户仓储，用于限制新建会话总数。
func (m *Manager) SetTenantRepo(tenants *repository.TenantRepo) {
	if m != nil {
		m.tenants = tenants
	}
}

func shortKey(sessionID string) string {
	return fmt.Sprintf("memory:short:%s", sessionID)
}

// Append 同时写入短期与长期记忆
func (m *Manager) Append(
	ctx context.Context,
	tenantID uint,
	userID, sessionID string,
	role model.MessageRole,
	content string,
	knowledgeBaseID uint,
	directoryID *uint,
) error {
	title := content
	if len([]rune(title)) > 40 {
		title = string([]rune(title)[:40]) + "..."
	}
	if err := m.ensureNewSessionAllowed(tenantID, sessionID); err != nil {
		return err
	}
	conv, err := m.convRepo.GetOrCreate(tenantID, userID, sessionID, title, knowledgeBaseID, directoryID)
	if err != nil {
		return fmt.Errorf("get or create conversation: %w", err)
	}

	msg := &model.Message{
		TenantID:       tenantID,
		ConversationID: conv.ID,
		UserID:         userID,
		SessionID:      sessionID,
		Role:           role,
		Content:        content,
	}
	if err := m.msgRepo.Create(msg); err != nil {
		return fmt.Errorf("save long-term memory: %w", err)
	}

	turn := ChatTurn{
		Role:      role,
		Content:   content,
		Timestamp: time.Now().Unix(),
	}
	raw, err := json.Marshal(turn)
	if err != nil {
		return err
	}

	key := shortKey(sessionID)
	pipe := m.rdb.Pipeline()
	pipe.RPush(ctx, key, raw)
	pipe.LTrim(ctx, key, int64(-m.shortMax), -1)
	pipe.Expire(ctx, key, m.ttl)
	pipe.Expire(ctx, summaryKey(sessionID), m.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("save short-term memory: %w", err)
	}
	return nil
}

// EnsureNewSessionAllowed 已有会话直接通过；新建时不能超过租户会话总数上限。
func (m *Manager) EnsureNewSessionAllowed(tenantID uint, sessionID string) error {
	return m.ensureNewSessionAllowed(tenantID, sessionID)
}

func (m *Manager) ensureNewSessionAllowed(tenantID uint, sessionID string) error {
	if m == nil || m.tenants == nil || m.convRepo == nil || tenantID == 0 || sessionID == "" {
		return nil
	}
	if _, err := m.convRepo.GetBySessionID(sessionID); err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	tenant, err := m.tenants.GetByID(tenantID)
	if err != nil {
		return err
	}
	n, err := m.convRepo.CountByTenant(tenantID)
	if err != nil {
		return err
	}
	if n >= int64(tenant.SessionMax()) {
		return fmt.Errorf("已达到租户会话总数上限 %d", tenant.SessionMax())
	}
	return nil
}

// ListSessions 按租户/用户/知识库/目录列出历史会话
func (m *Manager) ListSessions(tenantID uint, userID string, knowledgeBaseID uint, directoryID *uint) ([]model.Conversation, error) {
	filter := repository.ConversationListFilter{
		TenantID:           tenantID,
		UserID:             userID,
		KnowledgeBaseID:    knowledgeBaseID,
		DirectoryID:        directoryID,
		MatchNullDirectory: directoryID == nil,
	}
	return m.convRepo.List(filter, 100)
}

// GetShortTerm 读取 Redis 短期记忆
func (m *Manager) GetShortTerm(ctx context.Context, sessionID string) ([]ChatTurn, error) {
	vals, err := m.rdb.LRange(ctx, shortKey(sessionID), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	turns := make([]ChatTurn, 0, len(vals))
	for _, v := range vals {
		var t ChatTurn
		if err := json.Unmarshal([]byte(v), &t); err != nil {
			continue
		}
		turns = append(turns, t)
	}
	return turns, nil
}

// GetLongTerm 读取 PostgreSQL 长期记忆
func (m *Manager) GetLongTerm(ctx context.Context, sessionID string) ([]model.Message, error) {
	return m.msgRepo.ListBySession(sessionID, m.longMax)
}

// BuildContextMessages 优先用短期记忆，缺失时回退长期记忆
func (m *Manager) BuildContextMessages(ctx context.Context, sessionID string) ([]ChatTurn, error) {
	short, err := m.GetShortTerm(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(short) > 0 {
		return short, nil
	}

	long, err := m.GetLongTerm(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	turns := make([]ChatTurn, 0, len(long))
	for _, msg := range long {
		turns = append(turns, ChatTurn{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.CreatedAt.Unix(),
		})
	}
	return turns, nil
}
