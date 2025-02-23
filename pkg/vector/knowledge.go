package vector

import (
	"context"
	"fmt"

	"github.com/pgvector/pgvector-go"
)

type KnowledgeService struct {
	V *Service
}

func NewKnowledge(ctx context.Context, v *Service) (*KnowledgeService, error) {
	_, err := v.DB.ExecContext(ctx, fmt.Sprintf(knowledgeSchemaSQL, v.Dimensions))
	if err != nil {
		return nil, fmt.Errorf("failed to create knowledge schema: %w", err)
	}
	s := &KnowledgeService{V: v}
	if err := s.Truncate(ctx); err != nil {
		return nil, fmt.Errorf("failed to truncate knowledge: %w", err)
	}
	return s, nil
}

func (s *KnowledgeService) Store(ctx context.Context, content string) error {
	embedding, err := s.V.GenerateEmbeddings(ctx, content)
	if err != nil {
		return err
	}
	return s.StoreEmbedding(ctx, content, embedding)
}

func (s *KnowledgeService) StoreEmbedding(ctx context.Context, content string, embedding []float64) error {
	embs32 := make([]float32, len(embedding))
	for i, v := range embedding {
		embs32[i] = float32(v)
	}
	_, err := s.V.DB.ExecContext(ctx, storeKnowledgeSQL, content, pgvector.NewVector(embs32))
	return err
}

func (s *KnowledgeService) Query(ctx context.Context, query string) ([]string, error) {
	embedding, err := s.V.GenerateEmbeddings(ctx, query)
	if err != nil {
		return nil, err
	}
	embs32 := make([]float32, len(embedding))
	for i, v := range embedding {
		embs32[i] = float32(v)
	}

	var rows []string
	err = s.V.DB.SelectContext(ctx, &rows, queryKnowledgeSQL, pgvector.NewVector(embs32))
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *KnowledgeService) Truncate(ctx context.Context) error {
	_, err := s.V.DB.ExecContext(ctx, truncateKnowledgeSQL)
	return err
}
