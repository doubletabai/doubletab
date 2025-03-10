package tooling

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
)

const (
	generateOpenAPISpecPrompt = `You are an AI assistant that generates OpenAPI 3.0 specifications for REST APIs.

First, query memory for any relevant information. Then, based on the memory and user input, generate an OpenAPI 3.0 spec
in YAML format. The spec should follow a typical CRUD API structure:

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
- Avoid duplicating models just for Create/Update requests (eg. when some field like ID is not needed).
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

func (s *Service) GenerateOpenAPISpec(ctx context.Context, multi *pterm.MultiPrinter, arguments string) string {
	spinner := NewSpinner(multi, "Generating OpenAPI spec...")
	defer spinner.Success("OpenAPI spec generated")

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	userInput := args["user_input"].(string)

	log.Debug().Msgf("Creating spec for question: %s", userInput)
	agent := s.Agent(generateOpenAPISpecPrompt, userInput).
		WithTools(s.QueryMemoryTool()).
		WithModel(s.ChatModel)

	spec := agent.Run(ctx)

	if err := createBoilerPlate(); err != nil {
		return fmt.Sprintf("Failed to create boilerplate: %v", err)
	}

	docDir := path.Join(os.Getenv("PROJECT_ROOT"), "pkg", "api", "doc")
	fh, err := os.Create(path.Join(docDir, "openapi.yaml"))
	if err != nil {
		return fmt.Sprintf("Failed to create openapi spec file: %v", err)
	}
	defer fh.Close()

	spec = TrimNonCode(spec, "yaml")

	_, err = fh.WriteString(spec)
	if err != nil {
		return fmt.Sprintf("Failed to write openapi spec file: %v", err)
	}

	return spec
}
