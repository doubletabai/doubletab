package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/doubletabai/doubletab/pkg/knowledgebase"
	"github.com/doubletabai/doubletab/pkg/tooling"
	"github.com/doubletabai/doubletab/pkg/vector"
)

const (
	mainWorkflowPrompt = `You are an AI assistant that helps developers build backend applications step by step. Your
workflow is as follow:

1. Agree with user on the entities and fields.
2. Generate an OpenAPI 3.0 yaml specification.
3. Generate PostgreSQL schema for the OpenAPI spec.
4. Store generated schema in the database.
5. Generate Go code implementing handlers.
6. Generate Go code implementing service.

When the code is generated, try building it. If it fails, re-generate the service code providing the error context to
the tool. Make sure to provide exact error from build step to the service code generation tool.

When user asks for something that doesn't fit the workflow, consult the knowledge base or ask clarifying questions.
`
)

const defaultKnowledgeBaseTable = "knowledge_base"

func main() {
	lvl, err := zerolog.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		os.Getenv("PGHOST"), os.Getenv("PGPORT"), os.Getenv("PGDATABASE"), os.Getenv("PGUSER"), os.Getenv("PGPASSWORD"), os.Getenv("PGSSLMODE"))

	db, err := sqlx.ConnectContext(ctx, "postgres", conn)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	openAICli := openai.NewClient()

	knowDB, err := vector.New(ctx, defaultKnowledgeBaseTable, openAICli)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize knowledge base")
	}
	defer knowDB.Close()

	if err := knowDB.Truncate(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to truncate knowledge base")
	}
	if err := knowledgebase.Populate(ctx, knowDB); err != nil {
		log.Fatal().Err(err).Msg("Failed to populate knowledge base")
	}

	ts, err := tooling.New(db, knowDB, openAICli)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize tooling service")
	}
	defer ts.Clear()

	pterm.DefaultBasicText.Println("Welcome to the" + pterm.LightMagenta(" DoubleTab ") + "AI assistant for backend development! What would you like to build today?")
	question := os.Getenv("INITIAL_QUERY")
	if question != "" {
		question, err = pterm.DefaultInteractiveTextInput.WithDefaultText(">").WithDelimiter(" ").WithDefaultValue(question).Show()
	} else {
		question, err = pterm.DefaultInteractiveTextInput.WithDefaultText(">").WithDelimiter(" ").Show()
	}
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get user input")
	}

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(mainWorkflowPrompt),
			openai.UserMessage(question),
		}),
		Tools: openai.F([]openai.ChatCompletionToolParam{
			ts.ListTablesTool(),
			ts.GenerateOpenAPISpecTool(),
			ts.GenerateSchemaTool(),
			ts.StoreSchemaTool(),
			ts.GenerateHandlersCodeTool(),
			ts.GenerateServiceCodeTool(),
			ts.BuildCodeTool(),
			ts.QueryKnowledgeBaseTool(),
		}),
		Model: openai.F(openai.ChatModelGPT4oMini),
		Seed:  openai.Int(1),
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
			var resp string
			switch toolCall.Function.Name {
			case tooling.GenerateOpenAPISpecToolName:
				resp = ts.GenerateOpenAPISpec(ctx, toolCall.Function.Arguments)
			case tooling.ListTablesToolName:
				resp = ts.ListTables(ctx)
			case tooling.GenerateSchemaToolName:
				resp = ts.GenerateSchema(ctx, toolCall.Function.Arguments)
			case tooling.StoreSchemaToolName:
				resp = ts.StoreSchema(ctx, toolCall.Function.Arguments)
			case tooling.GenerateHandlersCodeToolName:
				resp = ts.GenerateHandlersCode(ctx)
			case tooling.GenerateServiceCodeToolName:
				resp = ts.GenerateServiceCode(ctx, toolCall.Function.Arguments)
			case tooling.BuildCodeToolName:
				resp = ts.BuildCode(ctx)
			case tooling.QueryKnowledgeBaseToolName:
				resp = ts.QueryKnowledgeBase(ctx, toolCall.Function.Arguments)
			}
			params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, resp))
		}
		stream.Close()
	}
}
