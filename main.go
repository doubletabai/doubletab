package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/doubletabai/doubletab/pkg/tooling"
)

const (
	mainWorkflowPrompt = `You are an AI assistant that helps developers build backend applications step by step. Your
workflow always starts by defining a database schema before moving to API development. When a user describes an
application, first confirm their entities, then suggest a normalized PostgreSQL database schema before continuing.
Next, generate REST API endpoints for the schema, and finally, generate Go code for the API handlers.`
)

func main() {
	lvl, err := zerolog.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		os.Getenv("PG_HOST"), os.Getenv("PG_PORT"), os.Getenv("PG_DB"), os.Getenv("PG_USER"), os.Getenv("PG_PASS"), os.Getenv("PG_SSL"))

	db, err := sqlx.ConnectContext(ctx, "postgres", conn)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	openAICli := openai.NewClient()

	ts := tooling.New(db, openAICli)

	pterm.DefaultBasicText.Println("Welcome to the" + pterm.LightMagenta(" DoubleTab ") + "AI assistant for backend development! What would you like to build today?")
	question := os.Getenv("INITIAL_QUERY")
	if question != "" {
		fmt.Printf("> %s\n", question)
	} else {
		question, err = pterm.DefaultInteractiveTextInput.WithDefaultText(">").WithDelimiter(" ").Show()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to get user input")
		}
	}

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(mainWorkflowPrompt),
			openai.UserMessage(question),
		}),
		Tools: openai.F([]openai.ChatCompletionToolParam{
			ts.ListTablesTool(),
			ts.GenerateSchemaTool(),
			ts.StoreSchemaTool(),
			ts.GenerateEndpointsTool(),
			ts.GenerateAndStoreHandlersCodeTool(),
		}),
		Model: openai.F(openai.ChatModelGPT4o),
	}

	for {
		stream := openAICli.Chat.Completions.NewStreaming(ctx, params)
		acc := openai.ChatCompletionAccumulator{}

		begin := false
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)
			if !begin && chunk.Choices[0].Delta.Content != "" {
				begin = true
				pterm.DefaultBasicText.Print(pterm.LightMagenta("DoubleTab: "))
			}
			pterm.DefaultBasicText.Print(chunk.Choices[0].Delta.Content)
		}
		if stream.Err() != nil {
			log.Fatal().Err(stream.Err()).Msg("Failed to stream completion")
		}
		if begin {
			pterm.DefaultBasicText.Println()
		}

		toolCalls := acc.Choices[0].Message.ToolCalls
		log.Debug().Msgf("Tool calls: %v", toolCalls)
		log.Debug().Msgf("Finish reason: %s", acc.Choices[0].FinishReason)

		if len(toolCalls) == 0 && acc.Choices[0].FinishReason == "stop" {
			params.Messages.Value = append(params.Messages.Value, acc.Choices[0].Message)
			nextStep, err := pterm.DefaultInteractiveTextInput.WithDefaultText(">").WithDelimiter(" ").Show()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to get user input")
			}
			params.Messages.Value = append(params.Messages.Value, openai.UserMessage(nextStep))
			stream.Close()
			continue
		}

		log.Debug().Msgf("Adding message to context from %s with tools? %t", acc.Choices[0].Message.Role, len(acc.Choices[0].Message.ToolCalls) > 0)
		params.Messages.Value = append(params.Messages.Value, acc.Choices[0].Message)
		for _, toolCall := range toolCalls {
			switch toolCall.Function.Name {
			case tooling.ListTablesToolName:
				tables := ts.ListTables(ctx)
				log.Debug().Msgf("Adding tool message to context: %s", toolCall.ID)
				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, tables))
			case tooling.GenerateSchemaToolName:
				schemaJson := ts.GenerateSchema(ctx, toolCall.Function.Arguments)
				log.Debug().Msgf("Adding tool message to context: %s", toolCall.ID)
				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, schemaJson))
			case tooling.StoreSchemaToolName:
				resp := ts.StoreSchema(ctx, toolCall.Function.Arguments)
				log.Debug().Msgf("Adding tool message to context: %s", toolCall.ID)
				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, resp))
			case tooling.GenerateEndpointsToolName:
				endpointsJson := ts.GenerateEndpoints(ctx, toolCall.Function.Arguments)
				log.Debug().Msgf("Adding tool message to context: %s", toolCall.ID)
				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, endpointsJson))
			case tooling.GenerateAndStoreHandlersCodeToolName:
				result := ts.GenerateAndStoreHandlersCode(ctx, toolCall.Function.Arguments)
				log.Debug().Msgf("Adding tool message to context: %s", toolCall.ID)
				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, result))
			}
		}
		stream.Close()
	}
}

func setupDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "vectors.db")
	if err != nil {
		return nil, err
	}

	// Enable vector extension (SQLite version of PGVector)
	_, err = db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS embeddings USING vector(1536)") // 1536 dims for OpenAI embeddings
	if err != nil {
		return nil, err
	}

	return db, nil
}

func storeEmbedding(db *sql.DB, text string, embedding []float64) error {
	_, err := db.Exec("INSERT INTO embeddings (text, vector) VALUES (?, ?)", text, embedding)
	return err
}
