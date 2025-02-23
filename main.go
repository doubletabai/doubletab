package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/doubletabai/doubletab/pkg/config"
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
6. Generate Go code implementing server.

When user asks for something that doesn't fit the workflow, consult the knowledge base or ask clarifying questions.
`
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	lvl, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := fmt.Sprintf("host='%s' port='%d' dbname='%s' user='%s' password='%s' sslmode='%s'",
		cfg.PGHost, cfg.PGPort, cfg.PGDatabase, cfg.PGUser, cfg.PGPassword, cfg.PGSSLMode)

	db, err := sqlx.ConnectContext(ctx, "postgres", conn)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to project database")
	}
	defer db.Close()

	var opts []option.RequestOption
	if cfg.LLMBaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.LLMBaseURL))
	}
	llmCli := openai.NewClient(opts...)
	vs, err := vector.New(ctx, cfg, llmCli)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize vector service")
	}
	defer vs.Close()

	ks, err := vector.NewKnowledge(ctx, vs)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize knowledge service")
	}
	if err := knowledgebase.Populate(ctx, ks); err != nil {
		log.Fatal().Err(err).Msg("Failed to populate knowledge base")
	}

	sid := uuid.NewString()

	mem, err := vector.NewMemory(ctx, vs, sid)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize memory service")
	}

	ts, err := tooling.New(cfg, db, ks, mem, llmCli)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize tooling service")
	}
	defer ts.Clear()

	pterm.DefaultBasicText.Println("Welcome to the" + pterm.LightMagenta(" DoubleTab ") + "AI assistant for backend development! What would you like to build today?")
	pterm.DefaultBasicText.Printfln("Session ID: %s", sid)
	question := os.Getenv("INITIAL_QUERY")
	if question != "" {
		question, err = pterm.DefaultInteractiveTextInput.
			WithDefaultText(">").
			WithDelimiter(" ").
			WithOnInterruptFunc(exitFunc(sid)).
			WithDefaultValue(question).
			Show()
	} else {
		question, err = pterm.DefaultInteractiveTextInput.
			WithDefaultText(">").
			WithDelimiter(" ").
			WithOnInterruptFunc(exitFunc(sid)).
			Show()
	}
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get user input")
	}

	go runMainWorkflow(ctx, cfg, sid, question, ts, llmCli)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	<-sigs

	pterm.DefaultBasicText.Printf("Closing session %s\n", sid)
}

func exitFunc(sid string) func() {
	return func() {
		pterm.DefaultBasicText.Printf("Closing session %s\n", sid)
		os.Exit(1)
	}
}

func runMainWorkflow(ctx context.Context, cfg *config.Config, sid, question string, ts *tooling.Service, openAICli *openai.Client) {
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
			ts.GenerateServerCodeTool(),
			ts.QueryKnowledgeBaseTool(),
		}),
		Model: openai.String(cfg.LLMChatModel),
		Seed:  openai.Int(1),
	}

	if err := ts.Mem.Store(ctx, vector.RoleSystem, mainWorkflowPrompt); err != nil {
		log.Fatal().Err(err).Msg("Failed to store system message")
	}
	if err := ts.Mem.Store(ctx, vector.RoleUser, question); err != nil {
		log.Fatal().Err(err).Msg("Failed to store user message")
	}

	for {
		if ctx.Err() != nil {
			return
		}
		stream := openAICli.Chat.Completions.NewStreaming(ctx, params)
		acc := openai.ChatCompletionAccumulator{}

		begin := false
		for stream.Next() {
			if ctx.Err() != nil {
				stream.Close()
				return
			}
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
		if len(toolCalls) == 0 && acc.Choices[0].FinishReason == "stop" {
			if err := ts.Mem.Store(ctx, vector.RoleAssistant, acc.Choices[0].Message.Content); err != nil {
				log.Err(err).Msg("Failed to store assistant message")
			}
			params.Messages.Value = append(params.Messages.Value, acc.Choices[0].Message)
			nextStep, err := pterm.DefaultInteractiveTextInput.
				WithDefaultText(">").
				WithDelimiter(" ").
				WithOnInterruptFunc(exitFunc(sid)).
				Show()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to get user input")
			}
			if err := ts.Mem.Store(ctx, vector.RoleUser, nextStep); err != nil {
				log.Err(err).Msg("Failed to store user message")
			}
			params.Messages.Value = append(params.Messages.Value, openai.UserMessage(nextStep))
			stream.Close()
			continue
		}

		params.Messages.Value = append(params.Messages.Value, acc.Choices[0].Message)
		for _, toolCall := range toolCalls {
			if ctx.Err() != nil {
				stream.Close()
				return
			}
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
			case tooling.GenerateServerCodeToolName:
				resp = ts.GenerateServerCode(ctx, toolCall.Function.Arguments)
			case tooling.QueryKnowledgeBaseToolName:
				resp = ts.QueryKnowledgeBase(ctx, toolCall.Function.Arguments)
			}
			log.Debug().Msgf("Adding message to context from tool %s, resp: %s", toolCall.ID, resp)
			if err := ts.Mem.Store(ctx, vector.RoleTool, resp); err != nil {
				log.Err(err).Msg("Failed to store tool message")
			}
			params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, resp))
		}
		stream.Close()
	}
}
