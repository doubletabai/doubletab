package vector

import (
	"context"
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
	"github.com/rs/zerolog/log"
)

type Service struct {
	DB        *sqlx.DB
	OpenAICli *openai.Client
}

type Config struct {
	Host     string
	Port     string
	Database string
	User     string
	Password string
	SSLMode  string
}

func EnvConfig() *Config {
	cfg := &Config{
		Host:     os.Getenv("DT_PGHOST"),
		Port:     os.Getenv("DT_PGPORT"),
		Database: os.Getenv("DT_PGDATABASE"),
		User:     os.Getenv("DT_PGUSER"),
		Password: os.Getenv("DT_PGPASSWORD"),
		SSLMode:  os.Getenv("DT_PGSSLMODE"),
	}
	if cfg.Host == "" {
		cfg.Host = os.Getenv("PGHOST")
	}
	if cfg.Port == "" {
		cfg.Port = os.Getenv("PGPORT")
	}
	if cfg.Database == "" {
		cfg.Database = os.Getenv("PGDATABASE")
	}
	if cfg.User == "" {
		cfg.User = os.Getenv("PGUSER")
	}
	if cfg.Password == "" {
		cfg.Password = os.Getenv("PGPASSWORD")
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = os.Getenv("PGSSLMODE")
	}
	return cfg
}

func New(ctx context.Context, cli *openai.Client) (*Service, error) {
	cfg := EnvConfig()
	conn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Database, cfg.User, cfg.Password, cfg.SSLMode)

	db, err := sqlx.ConnectContext(ctx, "postgres", conn)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}

	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		return nil, fmt.Errorf("failed to create embeddings table: %w", err)
	}

	return &Service{
		DB:        db,
		OpenAICli: cli,
	}, nil
}

func (s *Service) Close() {
	s.DB.Close()
}

func (s *Service) GenerateEmbeddings(ctx context.Context, text string) ([]float64, error) {
	resp, err := s.OpenAICli.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input:          openai.F[openai.EmbeddingNewParamsInputUnion](shared.UnionString(text)),
		Model:          openai.F(openai.EmbeddingModelTextEmbeddingAda002),
		EncodingFormat: openai.F(openai.EmbeddingNewParamsEncodingFormatFloat),
	})
	if err != nil {
		return nil, err
	}
	return resp.Data[0].Embedding, nil
}
