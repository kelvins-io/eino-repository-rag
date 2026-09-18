package repository

import (
	"errors"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SpeechUsageRepo struct {
	db *gorm.DB
}

func NewSpeechUsageRepo(db *gorm.DB) *SpeechUsageRepo {
	return &SpeechUsageRepo{db: db}
}

func (r *SpeechUsageRepo) Counts(tenantID uint, day string) (voiceInputs, ttsCount int, err error) {
	if r == nil || r.db == nil || tenantID == 0 || day == "" {
		return 0, 0, nil
	}
	var row model.TenantSpeechUsage
	err = r.db.Where("tenant_id = ? AND day = ?", tenantID, day).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return row.VoiceInputs, row.TTSCount, nil
}

func (r *SpeechUsageRepo) IncrementVoice(tenantID uint, day string) error {
	return r.increment(tenantID, day, true)
}

func (r *SpeechUsageRepo) IncrementTTS(tenantID uint, day string) error {
	return r.increment(tenantID, day, false)
}

func (r *SpeechUsageRepo) increment(tenantID uint, day string, voice bool) error {
	if r == nil || r.db == nil || tenantID == 0 || day == "" {
		return nil
	}
	row := model.TenantSpeechUsage{TenantID: tenantID, Day: day}
	column := "tts_count"
	if voice {
		row.VoiceInputs = 1
		column = "voice_inputs"
	} else {
		row.TTSCount = 1
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "day"}},
		DoUpdates: clause.Assignments(map[string]any{
			column:       gorm.Expr(column + " + 1"),
			"updated_at": time.Now(),
		}),
	}).Select("TenantID", "Day", "VoiceInputs", "TTSCount").Create(&row).Error
}
