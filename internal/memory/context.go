package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Summarizer 将较早的多轮对话压缩为摘要（由 RAG 层注入 LLM 实现）。
type Summarizer interface {
	Summarize(ctx context.Context, previousSummary string, turns []ChatTurn) (string, error)
}

// ContextPack 供生成阶段使用的历史上下文（摘要 + 最近原文）。
type ContextPack struct {
	Summary string
	Turns   []ChatTurn
}

type rollingSummary struct {
	Content string `json:"content"`
}

func summaryKey(sessionID string) string {
	return fmt.Sprintf("memory:summary:%s", sessionID)
}

// SetSummarizer 注入摘要实现；未注入时仅做 token 预算裁剪。
func (m *Manager) SetSummarizer(s Summarizer) {
	m.summarizer = s
}

func (m *Manager) loadSummary(ctx context.Context, sessionID string) (string, error) {
	raw, err := m.rdb.Get(ctx, summaryKey(sessionID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil
		}
		return "", err
	}
	var s rollingSummary
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return strings.TrimSpace(raw), nil
	}
	return s.Content, nil
}

func (m *Manager) saveSummary(ctx context.Context, sessionID string, content string) error {
	raw, err := json.Marshal(rollingSummary{Content: content})
	if err != nil {
		return err
	}
	return m.rdb.Set(ctx, summaryKey(sessionID), raw, m.ttl).Err()
}

func (m *Manager) trimShortTerm(ctx context.Context, sessionID string, keep int) error {
	if keep <= 0 {
		return nil
	}
	return m.rdb.LTrim(ctx, shortKey(sessionID), int64(-keep), -1).Err()
}

// BuildContextForPrompt 构建带摘要与 token 预算控制的历史上下文。
//
// 策略：
//  1. 拉取窗口内历史 + 已有滚动摘要
//  2. 超出条数/预算时，将「较旧原文」交给 LLM 合并进摘要，并裁短短期记忆
//  3. 最终按 token 预算丢弃最旧原文或截断摘要
func (m *Manager) BuildContextForPrompt(ctx context.Context, sessionID string) (*ContextPack, error) {
	turns, err := m.BuildContextMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	summaryContent, err := m.loadSummary(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load summary: %w", err)
	}

	keepRecent := m.keepRecent
	if keepRecent <= 0 {
		keepRecent = 6
	}
	trigger := m.summaryTrigger
	if trigger <= 0 {
		trigger = 8
	}

	// 有摘要时限制参与折叠的活跃原文规模，避免长期记忆回退一次塞入过多轮次
	maxActive := keepRecent + trigger
	if summaryContent != "" && len(turns) > maxActive {
		turns = turns[len(turns)-maxActive:]
	}

	shouldSummarize := m.summaryEnabled && m.summarizer != nil && len(turns) > keepRecent
	if shouldSummarize {
		overCount := len(turns) >= trigger
		overBudget := m.tokenBudget > 0 && estimateHistoryTokens(summaryContent, turns) > m.tokenBudget
		shouldSummarize = overCount || overBudget
	}

	if shouldSummarize {
		foldEnd := len(turns) - keepRecent
		toFold := turns[:foldEnd]
		recent := turns[foldEnd:]

		newSummary, sumErr := m.summarizer.Summarize(ctx, summaryContent, toFold)
		if sumErr != nil {
			log.Printf("[memory] summarize failed session=%s err=%v; fallback to trim", sessionID, sumErr)
			turns = recent
		} else {
			summaryContent = newSummary
			turns = recent
			if saveErr := m.saveSummary(ctx, sessionID, summaryContent); saveErr != nil {
				log.Printf("[memory] save summary failed session=%s err=%v", sessionID, saveErr)
			}
			if trimErr := m.trimShortTerm(ctx, sessionID, keepRecent); trimErr != nil {
				log.Printf("[memory] trim short-term failed session=%s err=%v", sessionID, trimErr)
			}
		}
	}

	summaryContent, turns = FitToTokenBudget(summaryContent, turns, m.tokenBudget)
	return &ContextPack{Summary: summaryContent, Turns: turns}, nil
}
