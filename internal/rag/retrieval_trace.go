package rag

import (
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	dbmodel "github.com/kelvins-io/eino-repository-rag/internal/model"
	"github.com/kelvins-io/eino-repository-rag/internal/repository"
)

// retrievalHitWriter 只负责把已算好的召回记录写入库。分数不从日志读取。
type retrievalHitWriter interface {
	CreateHits(hits []dbmodel.RetrievalHit) error
}

// SetRetrievalRepo 注入检索记录仓储。未注入时问答仍可用，只是不落召回记录。
func (p *Pipeline) SetRetrievalRepo(repo *repository.RetrievalRepo) {
	if p == nil {
		return
	}
	p.retrieval = repo
}

// LinearRetrievalHits 线性 RAG：一轮、按 sources 原顺序排名。
// cited 是 validateCitations 的 1-based 合法引用，对应 sources 下标，而不是日志计数。
func LinearRetrievalHits(sources []SourceDocument, cited []int) []dbmodel.RetrievalHit {
	keep := indexSet(cited)
	hits := make([]dbmodel.RetrievalHit, 0, len(sources))
	for i, s := range sources {
		docID := canonicalDocID(s.DocID)
		if docID == "" {
			continue
		}
		rank := i + 1
		_, citedHit := keep[rank]
		hits = append(hits, dbmodel.RetrievalHit{
			Round:      1,
			Rank:       rank,
			DocID:      docID,
			ChunkIndex: s.ChunkIndex,
			Cited:      citedHit,
		})
	}
	return hits
}

// AgentRetrievalHits 按每一轮 knowledge_retrieve 的原始顺序记 rank。
// 引用编号是 collector 的全局编号，不能用该轮 rank 代替。
func AgentRetrievalHits(rounds [][]*schema.Document, globalIndex func(*schema.Document) int, cited []int) []dbmodel.RetrievalHit {
	keep := indexSet(cited)
	var hits []dbmodel.RetrievalHit
	for round, docs := range rounds {
		for i, d := range docs {
			if d == nil {
				continue
			}
			docID := canonicalDocID(metaString(d.MetaData, "doc_id"))
			if docID == "" {
				continue
			}
			n := 0
			if globalIndex != nil {
				n = globalIndex(d)
			}
			_, citedHit := keep[n]
			hits = append(hits, dbmodel.RetrievalHit{
				Round:      round + 1,
				Rank:       i + 1,
				DocID:      docID,
				ChunkIndex: metaInt(d.MetaData, "chunk_index"),
				Cited:      citedHit && n > 0,
			})
		}
	}
	return hits
}

func (p *Pipeline) persistRetrievalHits(req QueryRequest, userMsg, assistantMsg *dbmodel.Message, hits []dbmodel.RetrievalHit) {
	if p == nil || p.retrieval == nil || userMsg == nil || assistantMsg == nil {
		return
	}
	if userMsg.ID == 0 || assistantMsg.ID == 0 || len(hits) == 0 {
		return
	}
	for i := range hits {
		hits[i].TenantID = req.TenantID
		hits[i].UserID = req.UserID
		hits[i].SessionID = req.SessionID
		hits[i].UserMessageID = userMsg.ID
		hits[i].AssistantMessageID = assistantMsg.ID
		hits[i].KnowledgeBaseID = req.KnowledgeBaseID
	}
	if err := p.retrieval.CreateHits(hits); err != nil {
		logger.S().Errorf("[rag] save retrieval hits failed session=%s err=%v", req.SessionID, err)
		return
	}
	p.refreshStoredDocStats(req, hits)
}

type documentStatRefresher interface {
	RefreshDocumentRecalls(tenantID, knowledgeBaseID uint, docIDs []string, k int) (map[string]repository.StoredRecall, error)
	RefreshDocumentCitations(tenantID, knowledgeBaseID uint, docIDs []string) (map[string]int, error)
}

func (p *Pipeline) refreshStoredDocStats(req QueryRequest, hits []dbmodel.RetrievalHit) {
	refresher, ok := p.retrieval.(documentStatRefresher)
	if !ok || req.TenantID == 0 || req.KnowledgeBaseID == 0 {
		return
	}
	seen := make(map[string]struct{}, len(hits))
	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		if hit.DocID == "" {
			continue
		}
		if _, ok := seen[hit.DocID]; ok {
			continue
		}
		seen[hit.DocID] = struct{}{}
		ids = append(ids, hit.DocID)
	}
	if len(ids) == 0 {
		return
	}
	if p.cfg != nil && p.cfg.RAG.TopK > 0 {
		if _, err := refresher.RefreshDocumentRecalls(req.TenantID, req.KnowledgeBaseID, ids, p.cfg.RAG.TopK); err != nil {
			logger.S().Errorf("[rag] save document recall failed session=%s err=%v", req.SessionID, err)
		}
	}
	if _, err := refresher.RefreshDocumentCitations(req.TenantID, req.KnowledgeBaseID, ids); err != nil {
		logger.S().Errorf("[rag] save document cited count failed session=%s err=%v", req.SessionID, err)
	}
}

func indexSet(cited []int) map[int]struct{} {
	keep := make(map[int]struct{}, len(cited))
	for _, n := range cited {
		if n > 0 {
			keep[n] = struct{}{}
		}
	}
	return keep
}

func canonicalDocID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		return ""
	}
	return strconv.FormatUint(n, 10)
}
