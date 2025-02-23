package config

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Config struct {
	LogLevel               string `mapstructure:"log-level"`
	PGHost                 string `mapstructure:"pg-host"`
	PGPort                 int    `mapstructure:"pg-port"`
	PGDatabase             string `mapstructure:"pg-database"`
	PGUser                 string `mapstructure:"pg-user"`
	PGPassword             string `mapstructure:"pg-password"`
	PGSSLMode              string `mapstructure:"pg-sslmode"`
	DTPGHost               string `mapstructure:"dt-pg-host"`
	DTPGPort               int    `mapstructure:"dt-pg-port"`
	DTPGDatabase           string `mapstructure:"dt-pg-database"`
	DTPGUser               string `mapstructure:"dt-pg-user"`
	DTPGPassword           string `mapstructure:"dt-pg-password"`
	DTPGSSLMode            string `mapstructure:"dt-pg-sslmode"`
	OpenAIAPIKey           string `mapstructure:"openai-api-key"`
	LLMBaseURL             string `mapstructure:"llm-base-url"`
	LLMChatModel           string `mapstructure:"llm-chat-model"`
	LLMCodeModel           string `mapstructure:"llm-code-model"`
	LLMEmbeddingModel      string `mapstructure:"llm-embedding-model"`
	LLMEmbeddingDimensions int64  `mapstructure:"llm-embedding-dimensions"`
	InitialQuery           string `mapstructure:"initial-query"`
	ProjectRoot            string `mapstructure:"project-root"`
}

func Load() (*Config, error) {
	viper.SetEnvPrefix("")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	pflag.String("log-level", "warn", "Log level (debug, info, warn, error)")
	pflag.String("pg-host", "localhost", "PostgreSQL host")
	pflag.Int("pg-port", 5432, "PostgreSQL port")
	pflag.String("pg-database", "", "PostgreSQL database name")
	pflag.String("pg-user", "", "PostgreSQL username")
	pflag.String("pg-password", "", "PostgreSQL password")
	pflag.String("pg-sslmode", "disable", "PostgreSQL SSL mode")

	pflag.String("dt-pg-host", "localhost", "DoubleTab PostgreSQL host")
	pflag.Int("dt-pg-port", 5432, "DoubleTab PostgreSQL port")
	pflag.String("dt-pg-database", "doubletab", "DoubleTab PostgreSQL database name")
	pflag.String("dt-pg-user", "", "DoubleTab PostgreSQL username")
	pflag.String("dt-pg-password", "", "DoubleTab PostgreSQL password")
	pflag.String("dt-pg-sslmode", "disable", "DoubleTab PostgreSQL SSL mode")

	pflag.String("openai-api-key", "", "OpenAI API key")
	pflag.String("llm-base-url", "", "Base URL for LLM API")
	pflag.String("llm-chat-model", "gpt-4o", "Chat model for LLM")
	pflag.String("llm-code-model", "gpt-4o", "Code model for LLM")
	pflag.String("llm-embedding-model", "text-embedding-ada-002", "Embedding model for LLM")
	pflag.Int64("llm-embedding-dimensions", 1536, "Embedding dimensions for LLM")

	pflag.String("initial-query", "", "Initial query for processing")
	pflag.String("project-root", "", "Project root directory")
	pflag.Parse()

	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		return nil, fmt.Errorf("unable to bind pflags: %v", err)
	}

	cfg := Config{}
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to unmarshal config: %v", err)
	}

	return &cfg, nil
}
