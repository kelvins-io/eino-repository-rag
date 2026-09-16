package rag

import (
	"context"
	"fmt"
	"log"
	"strings"

	milvusindexer "github.com/cloudwego/eino-ext/components/indexer/milvus2"
	milvusretriever "github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2/search_mode"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus/client/v2/milvusclient"

	"github.com/kelvins-io/eino-repository-rag/internal/config"
)

type milvusVectorStore struct {
	cfg       *config.Config
	client    *milvusclient.Client
	indexer   *milvusindexer.Indexer
	retriever *milvusretriever.Retriever
}

func newMilvusLiteVectorStore(ctx context.Context, cfg *config.Config, emb embedding.Embedder) (*milvusVectorStore, error) {
	dim := cfg.Milvus.Dimension
	if dim <= 0 {
		dim = cfg.Embedding.Dimensions
	}
	if dim <= 0 {
		return nil, fmt.Errorf("milvus dimension is required (set milvus.dimension or embedding.dimensions)")
	}

	client, err := milvusclient.New(ctx, &milvusclient.ClientConfig{
		Address:  cfg.Milvus.Address,
		Username: cfg.Milvus.Username,
		Password: cfg.Milvus.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("connect milvus lite: %w", err)
	}

	idxMetric := parseIndexerMetric(cfg.Milvus.MetricType)
	retMetric := parseRetrieverMetric(cfg.Milvus.MetricType)

	idx, err := milvusindexer.NewIndexer(ctx, &milvusindexer.IndexerConfig{
		Client:     client,
		Collection: cfg.Milvus.Collection,
		Embedding:  emb,
		Vector: &milvusindexer.VectorConfig{
			Dimension:    int64(dim),
			MetricType:   idxMetric,
			IndexBuilder: milvusindexer.NewHNSWIndexBuilder().WithM(16).WithEfConstruction(200),
			VectorField:  "vector",
		},
		EnableDynamicSchema: true,
	})
	if err != nil {
		_ = client.Close(ctx)
		return nil, fmt.Errorf("create milvus indexer: %w", err)
	}

	ret, err := milvusretriever.NewRetriever(ctx, &milvusretriever.RetrieverConfig{
		Client:       client,
		Collection:   cfg.Milvus.Collection,
		TopK:         cfg.RAG.TopK,
		VectorField:  "vector",
		OutputFields: []string{"id", "content", "metadata"},
		SearchMode:   search_mode.NewApproximate(retMetric),
		Embedding:    emb,
	})
	if err != nil {
		_ = client.Close(ctx)
		return nil, fmt.Errorf("create milvus retriever: %w", err)
	}

	log.Printf("[rag] milvus_lite vector store ready address=%s collection=%s dim=%d metric=%s",
		cfg.Milvus.Address, cfg.Milvus.Collection, dim, cfg.Milvus.MetricType)

	return &milvusVectorStore{
		cfg:       cfg,
		client:    client,
		indexer:   idx,
		retriever: ret,
	}, nil
}

func (s *milvusVectorStore) Store(ctx context.Context, docs []*schema.Document) error {
	if _, err := s.indexer.Store(ctx, docs); err != nil {
		return fmt.Errorf("milvus store vectors: %w", err)
	}
	return nil
}

func (s *milvusVectorStore) Retrieve(ctx context.Context, query string, filter *RetrieveFilter) ([]*schema.Document, error) {
	expr := buildMilvusFilter(filter)
	var (
		docs []*schema.Document
		err  error
	)
	if expr != "" {
		docs, err = s.retriever.Retrieve(ctx, query, milvusretriever.WithFilter(expr))
	} else {
		docs, err = s.retriever.Retrieve(ctx, query)
	}
	if err != nil {
		return nil, fmt.Errorf("milvus retrieve: %w", err)
	}
	return docs, nil
}

func buildMilvusFilter(filter *RetrieveFilter) string {
	if filter == nil {
		return ""
	}
	var parts []string
	if filter.KnowledgeBaseID != "" {
		parts = append(parts, fmt.Sprintf(`metadata["kb_id"] == %q`, filter.KnowledgeBaseID))
	}
	if len(filter.DirectoryIDs) == 1 {
		parts = append(parts, fmt.Sprintf(`metadata["directory_id"] == %q`, filter.DirectoryIDs[0]))
	} else if len(filter.DirectoryIDs) > 1 {
		ors := make([]string, 0, len(filter.DirectoryIDs))
		for _, id := range filter.DirectoryIDs {
			ors = append(ors, fmt.Sprintf(`metadata["directory_id"] == %q`, id))
		}
		parts = append(parts, "("+strings.Join(ors, " or ")+")")
	}
	return strings.Join(parts, " and ")
}

func (s *milvusVectorStore) Close(ctx context.Context) error {
	if s.client == nil {
		return nil
	}
	return s.client.Close(ctx)
}

func parseIndexerMetric(v string) milvusindexer.MetricType {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "L2":
		return milvusindexer.L2
	case "IP":
		return milvusindexer.IP
	default:
		return milvusindexer.COSINE
	}
}

func parseRetrieverMetric(v string) milvusretriever.MetricType {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "L2":
		return milvusretriever.L2
	case "IP":
		return milvusretriever.IP
	default:
		return milvusretriever.COSINE
	}
}
