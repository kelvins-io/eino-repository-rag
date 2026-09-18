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
