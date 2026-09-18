package repository

import (
	"errors"
	"time"

	"github.com/kelvins-io/eino-repository-rag/internal/model"
	"gorm.io/gorm"
)

// MigrateIndexBuilds 保证同一文档同时最多一条未结束构建，并清掉入队与开工竞态造成的重复记录。
func MigrateIndexBuilds(db *gorm.DB) error {
	if err := db.Exec(`
DELETE FROM document_index_builds a
USING document_index_builds b
WHERE a.document_id = b.document_id
  AND a.id > b.id
  AND a.status = b.status
  AND a.triggered_at >= b.triggered_at
  AND a.triggered_at < b.triggered_at + interval '2 seconds'
  AND a.finished_at IS NOT DISTINCT FROM b.finished_at
`).Error; err != nil {
		return err
	}
	return db.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_document_index_build_open
ON document_index_builds (document_id)
WHERE finished_at IS NULL
`).Error
}

// EnsureIndexBuild 为一次入队创建未结束的构建记录；已有未结束记录则不重复创建。
func (r *DocumentRepo) EnsureIndexBuild(doc *model.Document) error {
	if r == nil || doc == nil || doc.ID == 0 {
		return nil
	}
	err := r.db.Create(&model.DocumentIndexBuild{
		TenantID:    doc.TenantID,
		DocumentID:  doc.ID,
		Status:      model.DocumentStatusPending,
		TriggeredAt: time.Now(),
	}).Error
	if isDuplicateKey(err) {
		return nil
	}
	return err
}

// BeginIndexBuild 将未结束记录标为索引中；没有未结束记录时补建一条（回收任务或入队记录失败时）。
func (r *DocumentRepo) BeginIndexBuild(doc *model.Document) error {
	if r == nil || doc == nil || doc.ID == 0 {
		return nil
	}
	updated, err := r.markOpenIndexBuild(doc.ID, model.DocumentStatusIndexing)
	if err != nil || updated {
		return err
	}
	err = r.db.Create(&model.DocumentIndexBuild{
		TenantID:    doc.TenantID,
		DocumentID:  doc.ID,
		Status:      model.DocumentStatusIndexing,
		TriggeredAt: time.Now(),
	}).Error
	if isDuplicateKey(err) {
		_, err = r.markOpenIndexBuild(doc.ID, model.DocumentStatusIndexing)
	}
	return err
}

func (r *DocumentRepo) markOpenIndexBuild(docID uint, status model.DocumentStatus) (bool, error) {
	res := r.db.Model(&model.DocumentIndexBuild{}).
		Where("document_id = ? AND finished_at IS NULL", docID).
		Update("status", status)
	return res.RowsAffected > 0, res.Error
}

func isDuplicateKey(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}

// NoteIndexBuildError 重试前记下最近一次错误，记录保持未结束。
func (r *DocumentRepo) NoteIndexBuildError(docID uint, errMsg string) error {
	if r == nil || docID == 0 {
		return nil
	}
	return r.db.Model(&model.DocumentIndexBuild{}).
		Where("document_id = ? AND finished_at IS NULL", docID).
		Updates(map[string]any{
			"status":    model.DocumentStatusPending,
			"error_msg": errMsg,
		}).Error
}

// FinishIndexBuild 结束最近一条未完成的构建记录。
func (r *DocumentRepo) FinishIndexBuild(docID uint, status model.DocumentStatus, errMsg string) error {
	if r == nil || docID == 0 {
		return nil
	}
	now := time.Now()
	return r.db.Model(&model.DocumentIndexBuild{}).
		Where("document_id = ? AND finished_at IS NULL", docID).
		Updates(map[string]any{
			"status":      status,
			"error_msg":   errMsg,
			"finished_at": now,
		}).Error
}

// ListIndexBuilds 按触发时间倒序返回文档的索引构建历史。
func (r *DocumentRepo) ListIndexBuilds(docID uint, limit int) ([]model.DocumentIndexBuild, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []model.DocumentIndexBuild
	err := r.db.Where("document_id = ?", docID).
		Order("triggered_at desc, id desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}
