package vector

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
	"github.com/rs/zerolog/log"

	"github.com/doubletabai/doubletab/pkg/config"
)

type Service struct {
	DB         *sqlx.DB
	OpenAICli  *openai.Client
	Model      string
	Dimensions int64
}

func New(ctx context.Context, cfg *config.Config, cli *openai.Client) (*Service, error) {
	conn := fmt.Sprintf("host='%s' port='%d' dbname='%s' user='%s' password='%s' sslmode='%s'",
		cfg.DTPGHost, cfg.DTPGPort, cfg.DTPGDatabase, cfg.DTPGUser, cfg.DTPGPassword, cfg.DTPGSSLMode)

	db, err := sqlx.ConnectContext(ctx, "postgres", conn)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to doubletab database")
	}

	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings table: %w", err)
	}

	return &Service{
		DB:         db,
		OpenAICli:  cli,
		Model:      cfg.LLMEmbeddingModel,
		Dimensions: cfg.LLMEmbeddingDimensions,
	}, nil
}

func (s *Service) Close() {
	s.DB.Close()
}

func (s *Service) GenerateEmbeddings(ctx context.Context, text string) ([]float32, error) {
	resp, err := s.OpenAICli.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input:          openai.F[openai.EmbeddingNewParamsInputUnion](shared.UnionString(text)),
		Model:          openai.String(s.Model),
		EncodingFormat: openai.F(openai.EmbeddingNewParamsEncodingFormatFloat),
	})
	if err != nil {
		return nil, err
	}
	embedding := make([]float32, len(resp.Data[0].Embedding))
	for i, v := range resp.Data[0].Embedding {
		embedding[i] = float32(v)
	}
	return embedding, nil
}
