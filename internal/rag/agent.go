package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	ub "github.com/cloudwego/eino/utils/callbacks"
	"github.com/google/uuid"

	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	"github.com/kelvins-io/eino-repository-rag/internal/memory"
	dbmodel "github.com/kelvins-io/eino-repository-rag/internal/model"
)

const agentPersona = `你是企业知识库 Agent。回答用户问题前，必须先调用 knowledge_retrieve 工具检索知识库。
可多次检索以补全不同角度的信息。请仅依据工具返回的片段作答；片段编号为 [n]，引用时只能使用这些编号。
若检索结果不足以回答，请明确说明「根据现有知识库无法确定」，不要编造。
回答使用简洁中文。`

// AgentQueryStream ReAct 多步检索问答；推送 step/tool_* 事件，最终 delta+done。
func (p *Pipeline) AgentQueryStream(ctx context.Context, req QueryRequest, onEvent StreamHandler) error {
	if onEvent == nil {
		return fmt.Errorf("stream handler is required")
	}
	if p.toolChat == nil {
		return fmt.Errorf("agent requires ToolCallingChatModel (deepseek)")
	}
	if req.SessionID == "" {
		req.SessionID = uuid.NewString()
	}
	if req.UserID == "" {
		req.UserID = "anonymous"
	}
	tenantMeta := uintToMeta(req.TenantID)
	if req.Filter == nil {
		req.Filter = &RetrieveFilter{TenantID: tenantMeta}
	} else if req.Filter.TenantID == "" {
		req.Filter.TenantID = tenantMeta
	}

	if err := onEvent(StreamEvent{
		Type:            StreamEventMeta,
		SessionID:       req.SessionID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		DirectoryID:     req.DirectoryID,
		Message:         "agent",
	}); err != nil {
		return err
	}
	_ = onEvent(StreamEvent{Type: StreamEventStep, Step: 0, Message: "启动 ReAct Agent"})

	history, err := p.mem.BuildContextForPrompt(ctx, req.SessionID)
	if err != nil {
		return fmt.Errorf("load memory: %w", err)
	}

	collector := newAgentDocCollector()
	ctx = withAgentRetrieveContext(ctx, req.Filter, collector)

	topK := p.cfg.Agent.ToolTopK
	if topK <= 0 {
		topK = p.cfg.RAG.TopK
	}
	retrieveTool, err := p.newKnowledgeRetrieveTool(topK)
	if err != nil {
		return fmt.Errorf("create retrieve tool: %w", err)
	}

	maxRounds := p.cfg.Agent.MaxSteps
	if maxRounds <= 0 {
		maxRounds = 4
	}
	maxRunSteps := maxRounds * 3
	if maxRunSteps < 6 {
		maxRunSteps = 6
	}

	var stepCounter int32
	toolCb := &ub.ToolCallbackHandler{
		OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
			n := int(atomic.AddInt32(&stepCounter, 1))
			name := knowledgeRetrieveToolName
			if info != nil && info.Name != "" {
				name = info.Name
			}
			q := ""
			if input != nil {
				q = parseToolQueryFromArgs(input.ArgumentsInJSON)
			}
			_ = onEvent(StreamEvent{
				Type:      StreamEventToolStart,
				Step:      n,
				Tool:      name,
				ToolQuery: q,
				Message:   fmt.Sprintf("调用工具 %s", name),
			})
			return ctx
		},
		OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
			n := int(atomic.LoadInt32(&stepCounter))
			name := knowledgeRetrieveToolName
			if info != nil && info.Name != "" {
				name = info.Name
			}
			count := 0
			if output != nil && output.Response != "" {
				var parsed knowledgeRetrieveOutput
				if err := json.Unmarshal([]byte(strings.TrimSpace(output.Response)), &parsed); err == nil {
					count = parsed.Count
				}
			}
			_ = onEvent(StreamEvent{
				Type:      StreamEventToolResult,
				Step:      n,
				Tool:      name,
				ToolCount: count,
				Message:   fmt.Sprintf("工具返回 %d 条", count),
			})
			return ctx
		},
	}
	cb := ub.NewHandlerHelper().Tool(toolCb).Handler()

	ra, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: p.toolChat,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []tool.BaseTool{retrieveTool},
		},
		MaxStep: maxRunSteps,
		MessageModifier: func(_ context.Context, input []*schema.Message) []*schema.Message {
			out := make([]*schema.Message, 0, len(input)+1)
			out = append(out, schema.SystemMessage(agentPersona))
			out = append(out, input...)
			return out
		},
	})
	if err != nil {
		return fmt.Errorf("create react agent: %w", err)
	}

	inputMsgs := buildAgentInputMessages(req.Query, history)
	msg, err := ra.Generate(ctx, inputMsgs, agent.WithComposeOptions(compose.WithCallbacks(cb)))
	if err != nil {
		return fmt.Errorf("agent generate: %w", err)
	}

	answer := ""
	if msg != nil {
		answer = strings.TrimSpace(msg.Content)
	}
	docs := collector.Docs()
	sources := make([]SourceDocument, 0, len(docs))
	for _, d := range docs {
		sources = append(sources, buildSourceDocument(d))
	}

	finalSources := sources
	check := validateCitations(answer, len(sources))
	if p.cfg.RAG.CitationValidateEnabled && check.Changed {
		logger.S().Infof("[rag] agent citation validate removed=%v kept=%v", check.Removed, check.ValidCited)
		answer = check.Answer
		if p.cfg.RAG.CitationFilterSources {
			finalSources = filterSourcesByCited(sources, check.ValidCited)
		}
	}

	if answer != "" {
		if err := onEvent(StreamEvent{Type: StreamEventDelta, Content: answer}); err != nil {
			return err
		}
	}

	userMsg, err := p.mem.Append(ctx, req.TenantID, req.UserID, req.SessionID, dbmodel.RoleUser, req.Query, req.KnowledgeBaseID, req.DirectoryID)
	if err != nil {
		logger.S().Errorf("[rag] agent append user memory failed: %v", err)
	}
	assistantMsg, err := p.mem.Append(ctx, req.TenantID, req.UserID, req.SessionID, dbmodel.RoleAssistant, answer, req.KnowledgeBaseID, req.DirectoryID)
	if err != nil {
		logger.S().Errorf("[rag] agent append assistant memory failed: %v", err)
	}
	p.persistRetrievalHits(req, userMsg, assistantMsg, AgentRetrievalHits(collector.Rounds(), collector.GlobalIndex, check.ValidCited))

	return onEvent(StreamEvent{
		Type:            StreamEventDone,
		Answer:          answer,
		SessionID:       req.SessionID,
		KnowledgeBaseID: req.KnowledgeBaseID,
		DirectoryID:     req.DirectoryID,
		Sources:         finalSources,
		MessageID:       messageRowID(assistantMsg),
		UserMessageID:   messageRowID(userMsg),
	})
}

func buildAgentInputMessages(query string, history *memory.ContextPack) []*schema.Message {
	messages := make([]*schema.Message, 0, 8)
	if history != nil && history.Summary != "" {
		messages = append(messages, &schema.Message{
			Role:    schema.System,
			Content: "历史对话摘要:\n" + history.Summary,
		})
	}
	var turns []memory.ChatTurn
	if history != nil {
		turns = history.Turns
	}
	for _, turn := range turns {
		role := schema.User
		switch turn.Role {
		case dbmodel.RoleAssistant:
			role = schema.Assistant
		case dbmodel.RoleSystem:
			role = schema.System
		}
		messages = append(messages, &schema.Message{Role: role, Content: turn.Content})
	}
	messages = append(messages, &schema.Message{Role: schema.User, Content: query})
	return messages
}
