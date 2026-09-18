package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

// SetFeedbackRepo 注入问答反馈仓储。
func (s *KnowledgeService) SetFeedbackRepo(repo *repository.FeedbackRepo) {
	if s != nil {
		s.feedback = repo
	}
}

// ChatFeedbackInput 更新点赞/点踩或评分。未出现的字段保持原值。
// vote: up / down / 空字符串（取消）。score: 1–5，0 表示清除评分。
type ChatFeedbackInput struct {
	SessionID string  `json:"session_id"`
	MessageID uint    `json:"message_id"`
	Vote      *string `json:"vote"`
	Score     *int    `json:"score"`
}

// ChatFeedback 当前用户对一条回答的反馈。
type ChatFeedback struct {
	SessionID string `json:"session_id"`
	MessageID uint   `json:"message_id"`
	Vote      string `json:"vote"`
	Score     int    `json:"score"`
}

// SetChatFeedback 为一条助手回答写入点赞、点踩或评分。
func (s *KnowledgeService) SetChatFeedback(userID string, tenantID uint, in ChatFeedbackInput) (*ChatFeedback, error) {
	if s == nil || s.feedback == nil || s.msgRepo == nil {
		return nil, fmt.Errorf("问答反馈未启用")
	}
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	sessionID := strings.TrimSpace(in.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("缺少 session_id")
	}
	if in.MessageID == 0 {
		return nil, fmt.Errorf("无效的 message_id")
	}
	msg, err := s.msgRepo.GetByID(in.MessageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("消息不存在")
		}
		return nil, err
	}
	if msg.UserID != actor.UserID || (msg.TenantID != 0 && msg.TenantID != actor.TenantID) {
		return nil, ErrForbidden
	}
	if msg.SessionID != sessionID {
		return nil, fmt.Errorf("消息不属于该会话")
	}
	if msg.Role != model.RoleAssistant {
		return nil, fmt.Errorf("无效的消息：只能评价助手回答")
	}

	cur := model.MessageFeedback{
		TenantID:  actor.TenantID,
		UserID:    actor.UserID,
		SessionID: sessionID,
		MessageID: msg.ID,
	}
	if existing, err := s.feedback.Get(actor.UserID, msg.ID); err == nil {
		cur.Vote = existing.Vote
		cur.Score = existing.Score
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	merged, err := mergeFeedback(cur, in.Vote, in.Score)
	if err != nil {
		return nil, err
	}
	saved, err := s.feedback.Save(&merged)
	if err != nil {
		return nil, err
	}
	return &ChatFeedback{
		SessionID: saved.SessionID,
		MessageID: saved.MessageID,
		Vote:      saved.Vote,
		Score:     saved.Score,
	}, nil
}

func (s *KnowledgeService) attachFeedback(userID string, msgs []model.Message) error {
	if s == nil || s.feedback == nil || len(msgs) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == model.RoleAssistant && m.ID > 0 {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	list, err := s.feedback.ListByMessageIDs(userID, ids)
	if err != nil {
		return err
	}
	byID := make(map[uint]model.MessageFeedback, len(list))
	for _, fb := range list {
		byID[fb.MessageID] = fb
	}
	for i := range msgs {
		fb, ok := byID[msgs[i].ID]
		if !ok {
			continue
		}
		msgs[i].Vote = fb.Vote
		msgs[i].Score = fb.Score
	}
	return nil
}

func mergeFeedback(cur model.MessageFeedback, vote *string, score *int) (model.MessageFeedback, error) {
	if vote == nil && score == nil {
		return cur, fmt.Errorf("无效的反馈：缺少 vote 或 score")
	}
	if vote != nil {
		v, err := normalizeVote(*vote)
		if err != nil {
			return cur, err
		}
		cur.Vote = v
	}
	if score != nil {
		n, err := normalizeScore(*score)
		if err != nil {
			return cur, err
		}
		cur.Score = n
	}
	return cur, nil
}

func normalizeVote(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case model.FeedbackVoteUp:
		return model.FeedbackVoteUp, nil
	case model.FeedbackVoteDown:
		return model.FeedbackVoteDown, nil
	default:
		return "", fmt.Errorf("无效的 vote")
	}
}

func normalizeScore(n int) (int, error) {
	if n < 0 || n > 5 {
		return 0, fmt.Errorf("无效的 score")
	}
	return n, nil
}
