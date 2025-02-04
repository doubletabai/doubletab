package main

import (
	"context"
	"fmt"

	"github.com/ollama/ollama/api"
	"github.com/rs/zerolog/log"
)

func addNumbers(a, b float64) float64 {
	return a + b
}

func main() {
	log.Info().Msg("Hello, World!")
	client, err := api.ClientFromEnvironment()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create client")
	}

	messages := []api.Message{
		{
			Role:    "user",
			Content: "add numbers 3 and 5",
		},
	}

	ctx := context.Background()

	addNumbersTool := api.ToolFunction{
		Name:        "add_numbers",
		Description: "Sum up given numbers",
		Parameters: struct {
			Type       string   `json:"type"`
			Required   []string `json:"required"`
			Properties map[string]struct {
				Type        string   `json:"type"`
				Description string   `json:"description"`
				Enum        []string `json:"enum,omitempty"`
			} `json:"properties"`
		}{
			Type:     "object",
			Required: []string{"a", "b"},
			Properties: make(map[string]struct {
				Type        string   `json:"type"`
				Description string   `json:"description"`
				Enum        []string `json:"enum,omitempty"`
			}),
		},
	}
	addNumbersTool.Parameters.Properties["a"] = struct {
		Type        string   `json:"type"`
		Description string   `json:"description"`
		Enum        []string `json:"enum,omitempty"`
	}{
		Type:        "integer",
		Description: "First number",
	}

	stream := true
	req := &api.ChatRequest{
		Model:    "llama3.1:8b",
		Messages: messages,
		Stream:   &stream,
		Tools: []api.Tool{
			{
				Type:     "function",
				Function: addNumbersTool,
			},
		},
	}

	respFunc := func(resp api.ChatResponse) error {
		log.Info().Msgf("%#v", resp)
		for _, tool := range resp.Message.ToolCalls {
			if tool.Function.Name == "add_numbers" {
				result := addNumbers(tool.Function.Arguments["a"].(float64), tool.Function.Arguments["b"].(float64))
				messages = append(messages, api.Message{
					Role:    "tool",
					Content: fmt.Sprintf("The result is %d", result),
				})
			}
		}
		return nil
	}

	err = client.Chat(ctx, req, respFunc)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to chat")
	}
}
