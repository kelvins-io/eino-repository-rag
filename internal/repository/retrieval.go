package repository

import (
	"database/sql"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"gorm.io/gorm"
)

// RetrievalRepo 持久化每次问答的召回片段，以及问题级相关文档标注。
type RetrievalRepo struct {
	db *gorm.DB
}

func NewRetrievalRepo(db *gorm.DB) *RetrievalRepo {
	return &RetrievalRepo{db: db}
}

func (r *RetrievalRepo) CreateHits(hits []model.RetrievalHit) error {
	if r == nil || r.db == nil || len(hits) == 0 {
		return nil
	}
	return r.db.Create(&hits).Error
}

// ReplaceLabels 用一组文档覆盖某条用户问题的相关标注。docIDs 为空则清空。
func (r *RetrievalRepo) ReplaceLabels(tenantID, userMessageID uint, labels []model.RetrievalLabel) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND user_message_id = ?", tenantID, userMessageID).
			Delete(&model.RetrievalLabel{}).Error; err != nil {
			return err
		}
		if len(labels) == 0 {
			return nil
		}
		return tx.Create(&labels).Error
	})
}

type countRow struct {
	N int64 `gorm:"column:n"`
}

// DocumentRecall 统计某文档的召回率。
// 分母：同租户、同知识库下把该文档标为相关的用户问题数。
// 分子：这些问题里，检索记录 rank<=k 出现过该文档的问题数。
func (r *RetrievalRepo) DocumentRecall(tenantID, knowledgeBaseID uint, docID string, k int) (labeled int, hits int, err error) {
	if r == nil || r.db == nil {
		return 0, 0, nil
	}
	var labeledRow countRow
	err = r.db.Raw(`
SELECT COUNT(DISTINCT user_message_id) AS n
FROM retrieval_labels
WHERE tenant_id = ? AND knowledge_base_id = ? AND doc_id = ?
`, tenantID, knowledgeBaseID, docID).Scan(&labeledRow).Error
	if err != nil {
		return 0, 0, err
	}
	var hitRow countRow
	err = r.db.Raw(`
SELECT COUNT(DISTINCT l.user_message_id) AS n
FROM retrieval_labels l
JOIN retrieval_hits h
  ON h.user_message_id = l.user_message_id
 AND h.tenant_id = l.tenant_id
 AND h.knowledge_base_id = l.knowledge_base_id
 AND h.doc_id = l.doc_id
WHERE l.tenant_id = ?
  AND l.knowledge_base_id = ?
  AND l.doc_id = ?
  AND h.rank > 0
  AND h.rank <= ?
`, tenantID, knowledgeBaseID, docID, k).Scan(&hitRow).Error
	if err != nil {
		return 0, 0, err
	}
	return int(labeledRow.N), int(hitRow.N), nil
}

func (r *RetrievalRepo) ConversationBySession(sessionID string) (*model.Conversation, error) {
	var conv model.Conversation
	if err := r.db.Where("session_id = ?", sessionID).First(&conv).Error; err != nil {
		return nil, err
	}
	return &conv, nil
}

func (r *RetrievalRepo) ListLabelsByMessages(tenantID uint, messageIDs []uint) ([]model.RetrievalLabel, error) {
	if r == nil || r.db == nil || tenantID == 0 || len(messageIDs) == 0 {
		return nil, nil
	}
	var list []model.RetrievalLabel
	err := r.db.Where("tenant_id = ? AND user_message_id IN ?", tenantID, messageIDs).Find(&list).Error
	return list, err
}

// StoredRecall 落在文档上的召回率快照。Recall 仅在 Labeled>0 时有值。
type StoredRecall struct {
	K       int
	Labeled int
	Hits    int
	Recall  *float64
}

type docRecallStat struct {
	DocID   string `gorm:"column:doc_id"`
	Labeled int64  `gorm:"column:labeled"`
	Hits    int64  `gorm:"column:hits"`
}

// RefreshDocumentRecalls 按已落库的标注和检索记录重算这些文档的召回率，并写回 documents。
// 不在结果里的文档视为没有标注，召回率写成 null。不改 documents.updated_at。
func (r *RetrievalRepo) RefreshDocumentRecalls(tenantID, knowledgeBaseID uint, docIDs []string, k int) (map[string]StoredRecall, error) {
	out := map[string]StoredRecall{}
	if r == nil || r.db == nil || tenantID == 0 || knowledgeBaseID == 0 || k <= 0 {
		return out, nil
	}
	ids := uniqueNumericDocIDs(docIDs)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []docRecallStat
	err := r.db.Raw(`
SELECT l.doc_id AS doc_id,
       COUNT(DISTINCT l.user_message_id) AS labeled,
       COUNT(DISTINCT CASE WHEN h.rank > 0 AND h.rank <= ? THEN h.user_message_id END) AS hits
FROM retrieval_labels l
LEFT JOIN retrieval_hits h
  ON h.user_message_id = l.user_message_id
 AND h.tenant_id = l.tenant_id
 AND h.knowledge_base_id = l.knowledge_base_id
 AND h.doc_id = l.doc_id
WHERE l.tenant_id = ?
  AND l.knowledge_base_id = ?
  AND l.doc_id IN ?
GROUP BY l.doc_id
`, k, tenantID, knowledgeBaseID, ids).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byID := make(map[string]docRecallStat, len(rows))
	for _, row := range rows {
		byID[row.DocID] = row
	}
	for _, id := range ids {
		row := byID[id]
		snap := StoredRecall{K: k, Labeled: int(row.Labeled), Hits: int(row.Hits)}
		recallVal := sql.NullFloat64{}
		if ratio, ok := RecallRatio(snap.Labeled, snap.Hits); ok {
			snap.Recall = &ratio
			recallVal = sql.NullFloat64{Float64: ratio, Valid: true}
		}
		n, err := strconv.ParseUint(id, 10, 64)
		if err != nil || n == 0 {
			continue
		}
		err = r.db.Exec(`
UPDATE documents
SET recall = ?, recall_k = ?, labeled_queries = ?, hit_queries = ?
WHERE id = ? AND tenant_id = ? AND knowledge_base_id = ?
`, recallVal, snap.K, snap.Labeled, snap.Hits, uint(n), tenantID, knowledgeBaseID).Error
		if err != nil {
			return out, err
		}
		out[id] = snap
	}
	return out, nil
}

type docCitedStat struct {
	DocID string `gorm:"column:doc_id"`
	N     int64  `gorm:"column:n"`
}

// RefreshDocumentCitations 按 retrieval_hits.cited 重算文档被引用次数并写回 documents。
// 次数是引用过该文档的回答数，同一条回答的多个分块只计 1 次。没有引用时写 0。不改 updated_at。
func (r *RetrievalRepo) RefreshDocumentCitations(tenantID, knowledgeBaseID uint, docIDs []string) (map[string]int, error) {
	out := map[string]int{}
	if r == nil || r.db == nil || tenantID == 0 || knowledgeBaseID == 0 {
		return out, nil
	}
	ids := uniqueNumericDocIDs(docIDs)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []docCitedStat
	err := r.db.Raw(`
SELECT doc_id AS doc_id, COUNT(DISTINCT assistant_message_id) AS n
FROM retrieval_hits
WHERE tenant_id = ?
  AND knowledge_base_id = ?
  AND doc_id IN ?
  AND cited = true
  AND assistant_message_id > 0
GROUP BY doc_id
`, tenantID, knowledgeBaseID, ids).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byID := make(map[string]int, len(rows))
	for _, row := range rows {
		if row.N < 0 {
			continue
		}
		byID[row.DocID] = int(row.N)
	}
	for _, id := range ids {
		n, err := strconv.ParseUint(id, 10, 64)
		if err != nil || n == 0 {
			continue
		}
		count := byID[id]
		err = r.db.Exec(`
UPDATE documents
SET cited_count = ?
WHERE id = ? AND tenant_id = ? AND knowledge_base_id = ?
`, count, uint(n), tenantID, knowledgeBaseID).Error
		if err != nil {
			return out, err
		}
		out[id] = count
	}
	return out, nil
}

type docChunkCitedStat struct {
	DocID      string `gorm:"column:doc_id"`
	ChunkIndex int    `gorm:"column:chunk_index"`
	N          int64  `gorm:"column:n"`
}

// RefreshDocumentChunkRanks 按合法引用重算每个片段的引用次数，只保留前 3 名并写回 documents.cited_chunks。
// 不改 updated_at。没有被引用的片段不进入排行。
func (r *RetrievalRepo) RefreshDocumentChunkRanks(tenantID, knowledgeBaseID uint, docIDs []string) (map[string][]model.CitedChunk, error) {
	out := map[string][]model.CitedChunk{}
	if r == nil || r.db == nil || tenantID == 0 || knowledgeBaseID == 0 {
		return out, nil
	}
	ids := uniqueNumericDocIDs(docIDs)
	if len(ids) == 0 {
		return out, nil
	}
	var rows []docChunkCitedStat
	err := r.db.Raw(`
SELECT doc_id AS doc_id, chunk_index AS chunk_index, COUNT(DISTINCT assistant_message_id) AS n
FROM retrieval_hits
WHERE tenant_id = ?
  AND knowledge_base_id = ?
  AND doc_id IN ?
  AND cited = true
  AND assistant_message_id > 0
GROUP BY doc_id, chunk_index
`, tenantID, knowledgeBaseID, ids).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	byDoc := make(map[string]map[int]int, len(ids))
	for _, id := range ids {
		byDoc[id] = map[int]int{}
	}
	for _, row := range rows {
		if _, ok := byDoc[row.DocID]; !ok || row.N <= 0 {
			continue
		}
		byDoc[row.DocID][row.ChunkIndex] += int(row.N)
	}
	for _, id := range ids {
		n, err := strconv.ParseUint(id, 10, 64)
		if err != nil || n == 0 {
			continue
		}
		ranks := RankCitedChunks(byDoc[id])
		payload, err := json.Marshal(ranks)
		if err != nil {
			return out, err
		}
		err = r.db.Exec(`
UPDATE documents
SET cited_chunks = ?::jsonb
WHERE id = ? AND tenant_id = ? AND knowledge_base_id = ?
`, string(payload), uint(n), tenantID, knowledgeBaseID).Error
		if err != nil {
			return out, err
		}
		out[id] = ranks
	}
	return out, nil
}

// RankCitedChunks 把片段引用次数排成名次，只保留前 3 名。
// 次数高的在前；相同次数按片段标号升序。标号从 0 起，0 是有效片段。
func RankCitedChunks(counts map[int]int) []model.CitedChunk {
	const topN = 3
	type pair struct {
		idx int
		n   int
	}
	pairs := make([]pair, 0, len(counts))
	for idx, n := range counts {
		if idx < 0 || n <= 0 {
			continue
		}
		pairs = append(pairs, pair{idx: idx, n: n})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].idx < pairs[j].idx
	})
	if len(pairs) > topN {
		pairs = pairs[:topN]
	}
	out := make([]model.CitedChunk, len(pairs))
	for i, p := range pairs {
		out[i] = model.CitedChunk{Rank: i + 1, ChunkIndex: p.idx, Count: p.n}
	}
	return out
}

func uniqueNumericDocIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		n, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
		if err != nil || n == 0 {
			continue
		}
		id := strconv.FormatUint(n, 10)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// RecallRatio 在有标注时返回命中数/标注数，上限为 1。无标注时 ok=false。
func RecallRatio(labeled, hits int) (float64, bool) {
	if labeled <= 0 {
		return 0, false
	}
	if hits < 0 {
		hits = 0
	}
	if hits > labeled {
		hits = labeled
	}
	return float64(hits) / float64(labeled), true
}
