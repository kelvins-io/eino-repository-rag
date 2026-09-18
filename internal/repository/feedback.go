package repository

import (
	"errors"

	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"gorm.io/gorm"
)

// FeedbackRepo 保存用户对助手回答的点赞、点踩和评分。
type FeedbackRepo struct {
	db *gorm.DB
}

func NewFeedbackRepo(db *gorm.DB) *FeedbackRepo {
	return &FeedbackRepo{db: db}
}

func (r *FeedbackRepo) Get(userID string, messageID uint) (*model.MessageFeedback, error) {
	var fb model.MessageFeedback
	err := r.db.Where("user_id = ? AND message_id = ?", userID, messageID).First(&fb).Error
	if err != nil {
		return nil, err
	}
	return &fb, nil
}

func (r *FeedbackRepo) ListByMessageIDs(userID string, messageIDs []uint) ([]model.MessageFeedback, error) {
	if r == nil || r.db == nil || userID == "" || len(messageIDs) == 0 {
		return nil, nil
	}
	var list []model.MessageFeedback
	err := r.db.Where("user_id = ? AND message_id IN ?", userID, messageIDs).Find(&list).Error
	return list, err
}

// Save 写入当前反馈。赞和分都空时删除记录。
func (r *FeedbackRepo) Save(fb *model.MessageFeedback) (*model.MessageFeedback, error) {
	if r == nil || r.db == nil || fb == nil {
		return nil, errors.New("feedback repo is nil")
	}
	existing, err := r.Get(fb.UserID, fb.MessageID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if fb.Vote == "" && fb.Score == 0 {
			return &model.MessageFeedback{
				UserID:    fb.UserID,
				SessionID: fb.SessionID,
				MessageID: fb.MessageID,
			}, nil
		}
		if err := r.db.Create(fb).Error; err != nil {
			return nil, err
		}
		return fb, nil
	}
	if err != nil {
		return nil, err
	}
	if fb.Vote == "" && fb.Score == 0 {
		if err := r.db.Delete(existing).Error; err != nil {
			return nil, err
		}
		existing.Vote = ""
		existing.Score = 0
		return existing, nil
	}
	existing.TenantID = fb.TenantID
	existing.SessionID = fb.SessionID
	existing.Vote = fb.Vote
	existing.Score = fb.Score
	if err := r.db.Save(existing).Error; err != nil {
		return nil, err
	}
	return existing, nil
}
