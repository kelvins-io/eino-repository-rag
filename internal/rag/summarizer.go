package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/kelvins-io/eino-repository-rag/internal/memory"
)

// llmSummarizer 使用聊天模型生成滚动历史摘要。
type llmSummarizer struct {
	chat    einomodel.BaseChatModel
	timeout time.Duration
}

func (s *llmSummarizer) Summarize(ctx context.Context, previousSummary string, turns []memory.ChatTurn) (string, error) {
	if len(turns) == 0 {
		return previousSummary, nil
	}
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	var b strings.Builder
	if previousSummary != "" {
		b.WriteString("已有摘要:\n")
		b.WriteString(previousSummary)
		b.WriteString("\n\n")
	}
	b.WriteString("待合并的对话:\n")
	for _, t := range turns {
		fmt.Fprintf(&b, "%s: %s\n", t.Role, t.Content)
	}

	messages := []*schema.Message{
		{
			Role: schema.System,
			Content: `你是对话记忆压缩助手。请将「已有摘要」与「待合并的对话」合并为一份简洁中文摘要。
保留：用户意图、关键实体、已确认结论、未解决问题。
不要编造，不要逐句复述。输出纯摘要正文，不要标题或列表前缀。`,
		},
		{
			Role:    schema.User,
			Content: b.String(),
		},
	}

	resp, err := s.chat.Generate(ctx, messages)
	if err != nil {
		return "", err
	}
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return previousSummary, nil
	}
	return summary, nil
}
