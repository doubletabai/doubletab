package vector

import (
	"context"
	"fmt"
	"slices"
	"strings"
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
	_, err := v.DB.ExecContext(ctx, fmt.Sprintf(memorySchemaSQL, v.Dimensions))
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

func (s *MemoryService) StoreEmbedding(ctx context.Context, role, content string, embedding []float32) error {
	args := map[string]interface{}{
		"session_id": s.SessionID,
		"role":       role,
		"content":    content,
		"created_at": time.Now().UTC(),
		"embedding":  pgvector.NewVector(embedding),
	}
	_, err := s.V.DB.NamedExecContext(ctx, storeMemorySQL, args)
	return err
}

type Memory struct {
	Role    string `db:"role"`
	Content string `db:"content"`
}

func (s *MemoryService) Query(ctx context.Context, query string) (string, error) {
	embedding, err := s.V.GenerateEmbeddings(ctx, query)
	if err != nil {
		return "", err
	}

	var mem []Memory
	err = s.V.DB.SelectContext(ctx, &mem, queryMemorySQL, s.SessionID, pgvector.NewVector(embedding))
	if err != nil {
		return "", err
	}

	// We want to feed an agent with the information in chronological order.
	slices.Reverse(mem)

	memories := make([]string, len(mem))
	for _, m := range mem {
		memories = append(memories, fmt.Sprintf("%s: %s", m.Role, m.Content))
	}
	return strings.Join(memories, "\n"), nil
}
