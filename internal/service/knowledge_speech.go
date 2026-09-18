package service

import (
	"fmt"

	"github.com/kelvins-io/eino-repository-rag/internal/model"
)

// EnsureVoiceInputAllowed 当天语音输入未达上限时才允许识别。
func (s *KnowledgeService) EnsureVoiceInputAllowed(tenantID uint) error {
	return s.ensureSpeechAllowed(tenantID, true)
}

// EnsureTTSAllowed 当天文字转语音未达上限时才允许合成。
func (s *KnowledgeService) EnsureTTSAllowed(tenantID uint) error {
	return s.ensureSpeechAllowed(tenantID, false)
}

// RecordVoiceInput 记一次成功的语音输入。
func (s *KnowledgeService) RecordVoiceInput(tenantID uint) error {
	if s == nil || s.speechUsage == nil {
		return nil
	}
	return s.speechUsage.IncrementVoice(tenantID, model.TodayShanghai())
}

// RecordTTS 记一次成功的文字转语音。
func (s *KnowledgeService) RecordTTS(tenantID uint) error {
	if s == nil || s.speechUsage == nil {
		return nil
	}
	return s.speechUsage.IncrementTTS(tenantID, model.TodayShanghai())
}

func (s *KnowledgeService) ensureSpeechAllowed(tenantID uint, voice bool) error {
	if s == nil || s.speechUsage == nil || s.tenantRepo == nil || tenantID == 0 {
		return nil
	}
	tenant, err := s.tenantRepo.GetByID(tenantID)
	if err != nil {
		return err
	}
	voiceN, ttsN, err := s.speechUsage.Counts(tenantID, model.TodayShanghai())
	if err != nil {
		return err
	}
	if voice {
		if voiceN >= tenant.VoiceInputMax() {
			return fmt.Errorf("已达到租户今日语音输入次数上限 %d", tenant.VoiceInputMax())
		}
		return nil
	}
	if ttsN >= tenant.TTSMax() {
		return fmt.Errorf("已达到租户今日文字转语音次数上限 %d", tenant.TTSMax())
	}
	return nil
}

// ChatQuota 当前租户今日剩余的新建会话、语音输入和文字转语音次数。
type ChatQuota struct {
	MaxSessions          int   `json:"max_sessions"`
	TodaySessionCount    int64 `json:"today_session_count"`
	RemainingSessions    int   `json:"remaining_sessions"`
	MaxVoiceInputs       int   `json:"max_voice_inputs"`
	TodayVoiceInputs     int   `json:"today_voice_inputs"`
	RemainingVoiceInputs int   `json:"remaining_voice_inputs"`
	MaxTTS               int   `json:"max_tts"`
	TodayTTSCount        int   `json:"today_tts_count"`
	RemainingTTS         int   `json:"remaining_tts"`
}

func (s *KnowledgeService) ChatQuota(tenantID uint) (ChatQuota, error) {
	if s == nil || s.tenantRepo == nil || tenantID == 0 {
		return ChatQuota{}, fmt.Errorf("tenant_id is required")
	}
	tenant, err := s.tenantRepo.GetByID(tenantID)
	if err != nil {
		return ChatQuota{}, err
	}
	var sessions int64
	if s.mem != nil {
		sessions, err = s.mem.CountNewSessionsSince(tenantID, model.StartOfTodayShanghai())
		if err != nil {
			return ChatQuota{}, err
		}
	}
	voice, tts := 0, 0
	if s.speechUsage != nil {
		voice, tts, err = s.speechUsage.Counts(tenantID, model.TodayShanghai())
		if err != nil {
			return ChatQuota{}, err
		}
	}
	return ChatQuota{
		MaxSessions:          tenant.SessionMax(),
		TodaySessionCount:    sessions,
		RemainingSessions:    remainingCount(tenant.SessionMax(), sessions),
		MaxVoiceInputs:       tenant.VoiceInputMax(),
		TodayVoiceInputs:     voice,
		RemainingVoiceInputs: remainingCount(tenant.VoiceInputMax(), int64(voice)),
		MaxTTS:               tenant.TTSMax(),
		TodayTTSCount:        tts,
		RemainingTTS:         remainingCount(tenant.TTSMax(), int64(tts)),
	}, nil
}

func remainingCount(max int, used int64) int {
	left := int64(max) - used
	if left < 0 {
		return 0
	}
	return int(left)
}
