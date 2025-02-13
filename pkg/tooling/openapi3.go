package tooling

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
)

const (
	generateOpenAPISpecPrompt = `You are an AI that generates OpenAPI 3.0 specifications for REST APIs.

Generate an OpenAPI 3.0 YAML spec for an application described by user. The spec should follow a typical CRUD API
structure:

- GET /resources: List all resources.
- POST /resources: Create a new resource.
- GET /resources/{id}: Get a resource by ID.
- PUT /resources/{id}: Update a resource.
- DELETE /resources/{id}: Delete a resource.

The API should:
- Use plural resource names.
- All IDs should be UUIDs.
- Use JSON request/response bodies.
- Follow OpenAPI 3.0 syntax.
- Include proper request/response models.

Return only valid OpenAPI YAML.
`
)

const GenerateOpenAPISpecToolName = "generate_openapi_spec"

func (s *Service) GenerateOpenAPISpecTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(GenerateOpenAPISpecToolName),
			Description: openai.String("Generates an OpenAPI 3.0.0 spec in YAML format based on user input about entities and fields."),
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

func (s *Service) GenerateOpenAPISpec(ctx context.Context, arguments string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Generating OpenAPI spec...")
	defer spinner.Stop()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	userInput := args["user_input"].(string)

	log.Debug().Msgf("Creating spec for question: %s", userInput)
	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(generateOpenAPISpecPrompt),
			openai.UserMessage(userInput),
		}),
		Model: openai.F(openai.ChatModelGPT4o),
	}

	completion, err := s.OpenAICli.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get completion")
	}

	return completion.Choices[0].Message.Content
}
