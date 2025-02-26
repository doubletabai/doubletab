package tooling

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/openai/openai-go"
	"github.com/rs/zerolog/log"

	"github.com/doubletabai/doubletab/pkg/config"
	"github.com/doubletabai/doubletab/pkg/vector"
)

type Service struct {
	DB        *sqlx.DB
	KS        *vector.KnowledgeService
	Mem       *vector.MemoryService
	OpenAICli *openai.Client
	ChatModel string
	CodeModel string
	TmpDir    string
}

func New(cfg *config.Config, db *sqlx.DB, ks *vector.KnowledgeService, mem *vector.MemoryService, cli *openai.Client) (*Service, error) {
	tmpDir, err := os.MkdirTemp("", "doubletab-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}
	return &Service{
		DB:        db,
		KS:        ks,
		Mem:       mem,
		OpenAICli: cli,
		ChatModel: cfg.LLMChatModel,
		CodeModel: cfg.LLMCodeModel,
		TmpDir:    tmpDir,
	}, nil
}

func (s *Service) Clear() {
	os.RemoveAll(s.TmpDir)
}

func (s *Service) HandleToolCall(ctx context.Context, tool openai.ChatCompletionMessageToolCallFunction) string {
	switch tool.Name {
	case GenerateOpenAPISpecToolName:
		return s.GenerateOpenAPISpec(ctx, tool.Arguments)
	case ListTablesToolName:
		return s.ListTables(ctx)
	case GenerateSchemaToolName:
		return s.GenerateSchema(ctx, tool.Arguments)
	case StoreSchemaToolName:
		return s.StoreSchema(ctx, tool.Arguments)
	case GenerateHandlersCodeToolName:
		return s.GenerateHandlersCode(ctx)
	case GenerateServerCodeToolName:
		return s.GenerateServerCode(ctx, tool.Arguments)
	case SaveServerCodeToolName:
		return s.SaveServerCode(ctx, tool.Arguments)
	case BuildCodeToolName:
		return s.BuildCode(ctx)
	case QueryKnowledgeBaseToolName:
		return s.QueryKnowledgeBase(ctx, tool.Arguments)
	case QueryMemoryToolName:
		return s.QueryMemory(ctx, tool.Arguments)
	default:
		return fmt.Sprintf("I don't know how to handle this tool call: %s", tool.Name)
	}
}

type Agent struct {
	ts     *Service
	params openai.ChatCompletionNewParams
}

func (s *Service) Agent(prompt, userInput string) *Agent {
	return &Agent{
		ts: s,
		params: openai.ChatCompletionNewParams{
			Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(prompt),
				openai.UserMessage(userInput),
			}),
			Seed: openai.Int(1),
		},
	}
}

func (a *Agent) WithTools(tools ...openai.ChatCompletionToolParam) *Agent {
	a.params.Tools = openai.F(tools)
	return a
}

func (a *Agent) WithModel(model string) *Agent {
	a.params.Model = openai.String(model)
	return a
}

func (a *Agent) Run(ctx context.Context) string {
	if len(a.params.Tools.Value) == 0 {
		completion, err := a.ts.OpenAICli.Chat.Completions.New(ctx, a.params)
		if err != nil {
			return fmt.Sprintf("Failed to get completion: %v", err)
		}
		return completion.Choices[0].Message.Content
	}

	var finalMessage string
	for {
		completion, err := a.ts.OpenAICli.Chat.Completions.New(ctx, a.params)
		if err != nil {
			return fmt.Sprintf("Failed to get completion: %v", err)
		}
		toolCalls := completion.Choices[0].Message.ToolCalls
		if len(toolCalls) == 0 && completion.Choices[0].FinishReason == "stop" {
			finalMessage = completion.Choices[0].Message.Content
			break
		}

		a.params.Messages.Value = append(a.params.Messages.Value, completion.Choices[0].Message)
		for _, toolCall := range toolCalls {
			if ctx.Err() != nil {
				return "Context canceled"
			}
			resp := a.ts.HandleToolCall(ctx, toolCall.Function)
			log.Debug().Msgf("Adding message to context from tool %s, resp: %s", toolCall.ID, resp)
			a.params.Messages.Value = append(a.params.Messages.Value, openai.ToolMessage(toolCall.ID, resp))

			// Don't store memory tool responses as that would duplicate data in the memory.
			if toolCall.Function.Name != QueryMemoryToolName {
				if err := a.ts.Mem.Store(ctx, vector.RoleTool, resp); err != nil {
					log.Err(err).Msg("Failed to store tool message")
				}
			}
		}
	}

	return finalMessage
}

func TrimNonCode(text, typ string) string {
	parts := strings.Split(text, "```"+typ)
	if len(parts) == 1 {
		return text
	}
	return strings.Split(parts[1], "```")[0]
}
