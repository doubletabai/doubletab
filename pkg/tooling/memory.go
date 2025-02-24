package tooling

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
)

const QueryMemoryToolName = "query_memory"

func (s *Service) QueryMemoryTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(QueryMemoryToolName),
			Description: openai.String("Query recent memory for a relevant information."),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"query"},
			}),
		}),
	}
}

func (s *Service) QueryMemory(ctx context.Context, arguments string) string {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	query := args["query"].(string)

	mem, err := s.Mem.Query(ctx, query)
	if err != nil {
		return fmt.Sprintf("Failed to query memory: %v", err)
	}

	return mem
}
