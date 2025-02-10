package tooling

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
)

const (
	generateEndpointsPrompt = `You are an AI assistant that generates REST API endpoints for a given database table.
- Follow the structure of standard CRUD APIs.
- Always return structured JSON, do not generate full code.
- The response must strictly follow this format:

{
    "resource": "<resource_name>",
    "base_path": "/<resource>",
    "endpoints": [
        {"method": "GET", "path": "/<resource>", "description": "List all records"},
        {"method": "POST", "path": "/<resource>", "description": "Create a new record"},
        {"method": "GET", "path": "/<resource>/{id}", "description": "Get a specific record"},
        {"method": "PUT", "path": "/<resource>/{id}", "description": "Update a record"},
        {"method": "DELETE", "path": "/<resource>/{id}", "description": "Delete a record"}
    ]
}`
	generateHandlersCodePrompt = `You are an AI assistant that helps developers generate REST API handler functions in Go for
a given endpoints specification in JSON format:

{
    "resource": "<resource_name>",
    "base_path": "/<resource>",
    "endpoints": [
        {"method": "GET", "path": "/<resource>", "description": "List all records"},
        {"method": "POST", "path": "/<resource>", "description": "Create a new record"},
        {"method": "GET", "path": "/<resource>/{id}", "description": "Get a specific record"},
        {"method": "PUT", "path": "/<resource>/{id}", "description": "Update a record"},
        {"method": "DELETE", "path": "/<resource>/{id}", "description": "Delete a record"}
    ]
}

- ONLY generate contents of a Go file called <resource_name>.go.
- Package name is "api".
- Assume that you already have a router instance.
- Assume that you already have a "service" with the database connection.
- Handlers should be methods on that "service", so they should start with "func (s *service) ...".
- Use standard Go libraries for http, json, and error handling.
- No external dependencies like gorilla, gin, etc.
- If you need to use database, use the "s.db" connection.
- "s.db" is a service that has methods: "List", Get", "Create", "Update", "Delete". Don't write SQL queries, those are
already implemented.
`
)

const GenerateEndpointsToolName = "generate_endpoints"

func (s *Service) GenerateEndpointsTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(GenerateEndpointsToolName),
			Description: openai.String("Takes generated schema and generates API endpoints in JSON format."),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"schema": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"schema"},
			}),
		}),
	}
}

const GenerateAndStoreHandlersCodeToolName = "generate_and_store_handlers_code"

func (s *Service) GenerateAndStoreHandlersCodeTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(GenerateAndStoreHandlersCodeToolName),
			Description: openai.String("Takes generated endpoints definition and creates appropriate file with Go code implementing handlers."),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"endpoints": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"endpoints"},
			}),
		}),
	}
}

func (s *Service) GenerateEndpoints(ctx context.Context, arguments string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Generating endpoints...")
	defer spinner.Stop()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	schema := args["schema"].(string)

	log.Debug().Msgf("Creating endpoints for schema: %s", schema)

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(generateEndpointsPrompt),
			openai.UserMessage(schema),
		}),
		Model: openai.F(openai.ChatModelGPT4o),
	}

	completion, err := s.OpenAICli.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get completion")
	}

	log.Debug().Msgf("Endpoints: %s", completion.Choices[0].Message.Content)
	return completion.Choices[0].Message.Content
}

type Endpoints struct {
	Resource  string     `json:"resource"`
	BasePath  string     `json:"base_path"`
	Endpoints []Endpoint `json:"endpoints"`
}

type Endpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

func (s *Service) GenerateAndStoreHandlersCode(ctx context.Context, arguments string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Creating endpoints...")
	defer spinner.Stop()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	endpoints := args["endpoints"].(string)

	var endpointsObj Endpoints
	if err := json.Unmarshal([]byte(endpoints), &endpointsObj); err != nil {
		return fmt.Sprintf("Failed to unmarshal json endpoints: %v", err)
	}

	apiDir := path.Join(os.Getenv("PROJECT_ROOT"), "pkg", "api")
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		return fmt.Sprintf("Failed to create directory")
	}

	fh, err := os.Create(path.Join(apiDir, fmt.Sprintf("%s.go", endpointsObj.Resource)))
	if err != nil {
		return fmt.Sprintf("Failed to open file")
	}

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(generateHandlersCodePrompt),
			openai.UserMessage(endpoints),
		}),
		Model: openai.F(openai.ChatModelGPT4o),
	}

	completion, err := s.OpenAICli.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get completion")
	}

	_, err = fh.WriteString(strings.TrimSuffix(strings.TrimPrefix(completion.Choices[0].Message.Content, "```go\n"), "```"))
	if err != nil {
		return fmt.Sprintf("Failed to write to file")
	}

	return "Endpoints created successfully"
}
