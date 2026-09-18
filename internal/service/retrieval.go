package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

// SetRetrievalRepo 注入检索记录与相关标注仓储。
func (s *KnowledgeService) SetRetrievalRepo(repo *repository.RetrievalRepo) {
	if s != nil {
		s.retrieval = repo
	}
}

// SetRecallK 文档列表落库召回率时使用的 K，与 rag.top_k 一致。
func (s *KnowledgeService) SetRecallK(k int) {
	if s != nil && k > 0 {
		s.recallK = k
	}
}

// RetrievalRelevance 某条用户问题的相关文档标注。
type RetrievalRelevance struct {
	SessionID       string   `json:"session_id"`
	MessageID       uint     `json:"message_id"`
	KnowledgeBaseID uint     `json:"knowledge_base_id"`
	DocIDs          []string `json:"doc_ids"`
}

// DocumentRecallResult 单个文档在已标注问题上的召回率。
// Recall 仅在 LabeledQueries>0 时有值；没有标注时为 null，而不是 0。
type DocumentRecallResult struct {
	DocID           string   `json:"doc_id"`
	KnowledgeBaseID uint     `json:"knowledge_base_id"`
	K               int      `json:"k"`
	LabeledQueries  int      `json:"labeled_queries"`
	HitQueries      int      `json:"hit_queries"`
	Recall          *float64 `json:"recall"`
}

// SetRetrievalRelevance 覆盖一条用户问题的相关文档。空 doc_ids 表示清除标注。
// 仅当前租户用户名为 admin 的账号可调用。
func (s *KnowledgeService) SetRetrievalRelevance(userID, username string, tenantID uint, in RetrievalRelevance) (*RetrievalRelevance, error) {
	if s == nil || s.retrieval == nil || s.msgRepo == nil {
		return nil, fmt.Errorf("检索记录未启用")
	}
	if err := requireTenantAdmin(username); err != nil {
		return nil, err
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
	if msg.Role != model.RoleUser {
		return nil, fmt.Errorf("无效的消息：只能标注用户提问")
	}
	conv, err := s.retrieval.ConversationBySession(sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("会话不存在")
		}
		return nil, err
	}
	if conv.UserID != actor.UserID || (conv.TenantID != 0 && conv.TenantID != actor.TenantID) {
		return nil, ErrForbidden
	}
	if conv.KnowledgeBaseID == 0 {
		return nil, fmt.Errorf("无效的会话：未绑定知识库")
	}
	if _, err := s.requireKBAccess(conv.KnowledgeBaseID, actor); err != nil {
		return nil, err
	}

	docIDs, err := normalizeLabelDocIDs(in.DocIDs)
	if err != nil {
		return nil, err
	}
	prev, err := s.retrieval.ListLabelsByMessages(actor.TenantID, []uint{msg.ID})
	if err != nil {
		return nil, err
	}
	for _, id := range docIDs {
		n, _ := strconv.ParseUint(id, 10, 64)
		doc, err := s.requireDocAccess(uint(n), actor)
		if err != nil {
			return nil, err
		}
		if doc.KnowledgeBaseID != conv.KnowledgeBaseID {
			return nil, fmt.Errorf("文档不属于当前会话的知识库")
		}
	}

	labels := make([]model.RetrievalLabel, 0, len(docIDs))
	for _, id := range docIDs {
		labels = append(labels, model.RetrievalLabel{
			TenantID:        actor.TenantID,
			UserID:          actor.UserID,
			SessionID:       sessionID,
			UserMessageID:   msg.ID,
			KnowledgeBaseID: conv.KnowledgeBaseID,
			DocID:           id,
		})
	}
	if err := s.retrieval.ReplaceLabels(actor.TenantID, msg.ID, labels); err != nil {
		return nil, err
	}
	affected := make([]string, 0, len(prev)+len(docIDs))
	for _, lb := range prev {
		if lb.KnowledgeBaseID == conv.KnowledgeBaseID {
			affected = append(affected, lb.DocID)
		}
	}
	affected = append(affected, docIDs...)
	s.persistDocumentRecalls(actor.TenantID, conv.KnowledgeBaseID, affected)
	if docIDs == nil {
		docIDs = []string{}
	}
	return &RetrievalRelevance{
		SessionID:       sessionID,
		MessageID:       msg.ID,
		KnowledgeBaseID: conv.KnowledgeBaseID,
		DocIDs:          docIDs,
	}, nil
}

func (s *KnowledgeService) attachRelevance(tenantID uint, msgs []model.Message) error {
	if s == nil || s.retrieval == nil || tenantID == 0 || len(msgs) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == model.RoleUser && m.ID > 0 {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	list, err := s.retrieval.ListLabelsByMessages(tenantID, ids)
	if err != nil {
		return err
	}
	byMsg := make(map[uint][]string, len(ids))
	for _, lb := range list {
		if lb.DocID == "" {
			continue
		}
		byMsg[lb.UserMessageID] = append(byMsg[lb.UserMessageID], lb.DocID)
	}
	for i := range msgs {
		ids, ok := byMsg[msgs[i].ID]
		if !ok {
			continue
		}
		msgs[i].RelevantDocIDs = ids
	}
	return nil
}

// persistDocumentRecalls 按当前标注和检索记录重算并写回文档召回率。失败只记日志，不阻断标注或列表。
func (s *KnowledgeService) persistDocumentRecalls(tenantID, knowledgeBaseID uint, docIDs []string) map[string]repository.StoredRecall {
	if s == nil || s.retrieval == nil || s.recallK <= 0 || tenantID == 0 || knowledgeBaseID == 0 || len(docIDs) == 0 {
		return nil
	}
	snaps, err := s.retrieval.RefreshDocumentRecalls(tenantID, knowledgeBaseID, docIDs, s.recallK)
	if err != nil {
		logger.S().Errorf("[recall] save document recall tenant=%d kb=%d err=%v", tenantID, knowledgeBaseID, err)
	}
	return snaps
}

// attachStoredRecalls 列表返回前重算当前页文档的召回率并写回，保证列上的值与标注、检索记录一致。
func (s *KnowledgeService) attachStoredRecalls(list []model.Document) {
	if len(list) == 0 {
		return
	}
	type groupKey struct {
		tenant uint
		kb     uint
	}
	groups := map[groupKey][]string{}
	for _, doc := range list {
		if doc.ID == 0 || doc.TenantID == 0 || doc.KnowledgeBaseID == 0 {
			continue
		}
		key := groupKey{doc.TenantID, doc.KnowledgeBaseID}
		groups[key] = append(groups[key], strconv.FormatUint(uint64(doc.ID), 10))
	}
	for key, ids := range groups {
		snaps := s.persistDocumentRecalls(key.tenant, key.kb, ids)
		if len(snaps) == 0 {
			continue
		}
		for i := range list {
			if list[i].TenantID != key.tenant || list[i].KnowledgeBaseID != key.kb {
				continue
			}
			snap, ok := snaps[strconv.FormatUint(uint64(list[i].ID), 10)]
			if !ok {
				continue
			}
			list[i].Recall = snap.Recall
			list[i].RecallK = snap.K
			list[i].LabeledQueries = snap.Labeled
			list[i].HitQueries = snap.Hits
		}
	}
}

func requireTenantAdmin(username string) error {
	if strings.TrimSpace(username) != tenantAdminUsername {
		return ErrTenantAdminOnly
	}
	return nil
}

// DocumentRecall 按已落库的检索记录和相关标注计算文档召回率。
func (s *KnowledgeService) DocumentRecall(userID string, tenantID, docID uint, k int) (*DocumentRecallResult, error) {
	if s == nil || s.retrieval == nil {
		return nil, fmt.Errorf("检索记录未启用")
	}
	actor, err := requireActor(userID, tenantID)
	if err != nil {
		return nil, err
	}
	if k <= 0 {
		return nil, fmt.Errorf("无效的 k")
	}
	doc, err := s.requireDocAccess(docID, actor)
	if err != nil {
		return nil, err
	}
	docKey := strconv.FormatUint(uint64(doc.ID), 10)
	labeled, hits, err := s.retrieval.DocumentRecall(actor.TenantID, doc.KnowledgeBaseID, docKey, k)
	if err != nil {
		return nil, err
	}
	out := &DocumentRecallResult{
		DocID:           docKey,
		KnowledgeBaseID: doc.KnowledgeBaseID,
		K:               k,
		LabeledQueries:  labeled,
		HitQueries:      hits,
	}
	if ratio, ok := repository.RecallRatio(labeled, hits); ok {
		out.Recall = &ratio
	}
	return out, nil
}

func normalizeLabelDocIDs(ids []string) ([]string, error) {
	if len(ids) > 50 {
		return nil, fmt.Errorf("无效的 doc_ids：一次最多 50 个")
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("无效的 doc_id: %s", raw)
		}
		id := strconv.FormatUint(n, 10)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}
