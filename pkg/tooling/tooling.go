package tooling

import (
	"github.com/jmoiron/sqlx"
	"github.com/openai/openai-go"
)

type Service struct {
	DB        *sqlx.DB
	OpenAICli *openai.Client
}

func New(db *sqlx.DB, cli *openai.Client) *Service {
	return &Service{
		DB:        db,
		OpenAICli: cli,
	}
}
