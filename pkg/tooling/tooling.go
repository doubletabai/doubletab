package tooling

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/openai/openai-go"

	"github.com/doubletabai/doubletab/pkg/vector"
)

type Service struct {
	DB        *sqlx.DB
	KS        *vector.KnowledgeService
	Mem       *vector.MemoryService
	OpenAICli *openai.Client
	TmpDir    string
}

func New(db *sqlx.DB, ks *vector.KnowledgeService, mem *vector.MemoryService, cli *openai.Client) (*Service, error) {
	tmpDir, err := os.MkdirTemp("", "doubletab-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	return &Service{
		DB:        db,
		KS:        ks,
		Mem:       mem,
		OpenAICli: cli,
		TmpDir:    tmpDir,
	}, nil
}

func (s *Service) Clear() {
	os.RemoveAll(s.TmpDir)
}
