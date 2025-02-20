package tooling

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
)

const QueryKnowledgeBaseToolName = "query_knowledge_base"

func (s *Service) QueryKnowledgeBaseTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(QueryKnowledgeBaseToolName),
			Description: openai.String("Consult the knowledge base for any user issues not fitting into the standard workflow."),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"user_input": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"user_input"},
			}),
		}),
	}
}

func (s *Service) QueryKnowledgeBase(ctx context.Context, arguments string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Querying knowledge base...")
	defer spinner.Stop()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	userInput := args["user_input"].(string)

	resp, err := s.KS.Query(ctx, userInput)
	if err != nil {
		log.Warn().Str("user_input", userInput).Err(err).Msg("Failed to query knowledge base")
		return fmt.Sprintf("Failed to query knowledge base: %v", err)
	}

	return strings.Join(resp, "\n")
}
