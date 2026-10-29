package rag

import (
	"context"
	"os"
	"strings"

	"github.com/kelvins-io/eino-repository-rag/internal/logger"
	dbmodel "github.com/kelvins-io/eino-repository-rag/internal/model"
)

// RemoteMaterializer 在索引开始前把在线文档下载到本地上传目录。
type RemoteMaterializer interface {
	MaterializeRemoteDocument(ctx context.Context, doc *dbmodel.Document) error
}

// SetRemoteMaterializer 注入在线文档下载器。需在启动索引队列之前调用，崩溃回灌才会重新拉取未完成的链接。
func (p *Pipeline) SetRemoteMaterializer(m RemoteMaterializer) {
	if p == nil {
		return
	}
	p.remote = m
}

// MaxUploadFileSizeBytes 系统配置的单文件大小上限。0 表示未配置。
func (p *Pipeline) MaxUploadFileSizeBytes() int64 {
	if p == nil || p.cfg == nil {
		return 0
	}
	return p.cfg.RAG.MaxUploadFileSizeBytes()
}

// ensureLocalDocument 在本地文件缺失且来源是 http(s) 时尝试下载。下载器自行判断扩展名。
func (p *Pipeline) ensureLocalDocument(ctx context.Context, doc *dbmodel.Document) (*dbmodel.Document, error) {
	if p == nil || doc == nil {
		return doc, nil
	}
	if localUploadReady(doc.FilePath) {
		return doc, nil
	}
	if p.remote == nil || !isHTTPDocumentSource(doc.Source) {
		return doc, nil
	}
	if err := p.remote.MaterializeRemoteDocument(ctx, doc); err != nil {
		logger.S().Errorf("[rag] fetch remote document id=%d err=%v", doc.ID, err)
		return nil, err
	}
	if p.docRepo == nil {
		return doc, nil
	}
	return p.docRepo.GetByID(doc.ID)
}

func localUploadReady(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func isHTTPDocumentSource(src string) bool {
	s := strings.ToLower(strings.TrimSpace(src))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
