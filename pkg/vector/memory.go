package vector

import (
	"context"
	"fmt"
	"time"

	"github.com/pgvector/pgvector-go"
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

type MemoryService struct {
	V         *Service
	SessionID string
}

func NewMemory(ctx context.Context, v *Service, sid string) (*MemoryService, error) {
	_, err := v.DB.ExecContext(ctx, memorySchemaSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to create memory schema: %w", err)
	}
	return &MemoryService{
		V:         v,
		SessionID: sid,
	}, nil
}

func (s *MemoryService) Store(ctx context.Context, role, content string) error {
	embedding, err := s.V.GenerateEmbeddings(ctx, content)
	if err != nil {
		return err
	}
	return s.StoreEmbedding(ctx, role, content, embedding)
}

func (s *MemoryService) StoreEmbedding(ctx context.Context, role, content string, embedding []float64) error {
	embs32 := make([]float32, len(embedding))
	for i, v := range embedding {
		embs32[i] = float32(v)
	}

	args := map[string]interface{}{
		"session_id": s.SessionID,
		"role":       role,
		"content":    content,
		"created_at": time.Now().UTC(),
		"embedding":  pgvector.NewVector(embs32),
	}
	_, err := s.V.DB.NamedExecContext(ctx, storeMemorySQL, args)
	return err
}
