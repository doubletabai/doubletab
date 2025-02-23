package tooling

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"

	"github.com/doubletabai/doubletab/pkg/vector"
)

const (
	generateServerCodePrompt = `You are an AI assistant that generates Go code implementing server based on previously generated OpenAPI 3.0 spec.

Implement ServerInterface generated by oapi-codegen. Your workflow is as follows:

1. Check the knowledge base for best practices and sample code.
2. Implement the ServerInterface methods strictly following sample code from the knowledge base. The interface was
   generated as follows:

%s

3. Save the code to the server.go file in the api package.
4. Build the server code. If it fails, address the build errors and re-generate the server code.

Important notes:
- Don't create any new types for resources, use the ones provided by the OpenAPI spec. Stick to the sample code provided
  by the knowledge base.
- Don't ask the user for any additional information, use the OpenAPI spec as the single source of truth.
`
)

const GenerateHandlersCodeToolName = "generate_handlers_code"

func (s *Service) GenerateHandlersCodeTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(GenerateHandlersCodeToolName),
			Description: openai.String("Generates Go code implementing handlers based on previously generated OpenAPI 3.0 spec."),
		}),
	}
}

const GenerateServerCodeToolName = "generate_server_code"

func (s *Service) GenerateServerCodeTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(GenerateServerCodeToolName),
			Description: openai.String("Generates Go code implementing server based on previously generated OpenAPI 3.0 spec."),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"openapi_spec": map[string]string{
						"type": "string",
					},
					"build_errors": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"openapi_spec"},
			}),
		}),
	}
}

const SaveServerCodeToolName = "save_server_code"

func (s *Service) SaveServerCodeTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(SaveServerCodeToolName),
			Description: openai.String("Save generated server Go code to the server.go file in the api package."),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"server_go_code": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"server_go_code"},
			}),
		}),
	}
}

func (s *Service) GenerateHandlersCode(ctx context.Context) string {
	spinner, _ := pterm.DefaultSpinner.Start("Generating handlers...")
	defer spinner.Stop()

	absRoot, err := filepath.Abs(os.Getenv("PROJECT_ROOT"))
	if err != nil {
		return fmt.Sprintf("Failed to get absolute path of project root: %v", err)
	}
	cmd := exec.CommandContext(ctx, "go", "generate", "./...")
	cmd.Dir = absRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("go generate failed: %v\n%s", err, output)
	}

	return "Handlers code generated successfully"
}

func (s *Service) GenerateServerCode(ctx context.Context, arguments string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Generating server code...")
	defer spinner.Stop()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	var buildErrors string
	var ok bool
	openApiSpec := args["openapi_spec"].(string)
	buildErrors, ok = args["build_errors"].(string)
	if !ok {
		buildErrors = ""
	}

	log.Debug().Msgf("Creating server code for OpenAPI spec: %s", openApiSpec)

	methods, err := extractServerInterfaceMethods(filepath.Join(os.Getenv("PROJECT_ROOT"), "pkg", "api", "handlers.gen.go"))
	if err != nil {
		return fmt.Sprintf("Failed to extract ServerInterface methods: %v", err)
	}
	iface := fmt.Sprintf("type ServerInterface interface {\n%s\n}\n", methods)

	prompt := fmt.Sprintf(generateServerCodePrompt, iface)

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(prompt),
			openai.UserMessage(openApiSpec),
			openai.UserMessage(buildErrors),
		}),
		Tools: openai.F([]openai.ChatCompletionToolParam{
			s.QueryKnowledgeBaseTool(),
			s.SaveServerCodeTool(),
			s.BuildCodeTool(),
		}),
		Model: openai.String(s.CodeModel),
		Seed:  openai.Int(1),
	}

	var finalMessage string
	for {
		completion, err := s.OpenAICli.Chat.Completions.New(ctx, params)
		if err != nil {
			return fmt.Sprintf("Failed to get completion: %v", err)
		}
		log.Debug().Msgf("Finish reason: %v", completion.Choices[0].FinishReason)
		log.Debug().Msgf("Tool calls: %v", completion.Choices[0].Message.ToolCalls)
		toolCalls := completion.Choices[0].Message.ToolCalls
		if len(toolCalls) == 0 && completion.Choices[0].FinishReason == "stop" {
			finalMessage = completion.Choices[0].Message.Content
			break
		}

		params.Messages.Value = append(params.Messages.Value, completion.Choices[0].Message)
		for _, toolCall := range toolCalls {
			if ctx.Err() != nil {
				return "Context canceled"
			}
			var resp string
			switch toolCall.Function.Name {
			case QueryKnowledgeBaseToolName:
				resp = s.QueryKnowledgeBase(ctx, toolCall.Function.Arguments)
			case SaveServerCodeToolName:
				resp = s.SaveServerCode(ctx, toolCall.Function.Arguments)
			case BuildCodeToolName:
				resp = s.BuildCode(ctx)
			}
			log.Debug().Msgf("Adding message to context from tool %s, resp: %s", toolCall.ID, resp)
			if err := s.Mem.Store(ctx, vector.RoleTool, resp); err != nil {
				log.Err(err).Msg("Failed to store tool message")
			}
			params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, resp))
		}
	}

	return finalMessage
}

func (s *Service) SaveServerCode(_ context.Context, arguments string) string {
	apiDir := path.Join(os.Getenv("PROJECT_ROOT"), "pkg", "api")
	fh, err := os.Create(path.Join(apiDir, "server.go"))
	if err != nil {
		return fmt.Sprintf("Failed to create server.go file: %v", err)
	}
	defer fh.Close()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	code := args["server_go_code"].(string)

	var rawCode string
	parts := strings.Split(code, "```go")
	if len(parts) == 2 {
		rawCode = parts[1]
		parts = strings.Split(rawCode, "```")
		rawCode = parts[0]
	} else {
		rawCode = code
	}

	_, err = fh.WriteString(rawCode)
	if err != nil {
		return fmt.Sprintf("Failed to write server.go file: %v", err)
	}

	return "Server code saved successfully"
}

// Scan a Go file and extract methods from `ServerInterface`
func extractServerInterfaceMethods(filename string) (string, error) {
	src, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, src, parser.AllErrors)
	if err != nil {
		return "", fmt.Errorf("failed to parse file: %w", err)
	}

	var methods []string
	ast.Inspect(node, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "ServerInterface" {
			return true
		}

		interfaceType, ok := typeSpec.Type.(*ast.InterfaceType)
		if !ok {
			return false
		}

		// Extract method signatures
		for _, method := range interfaceType.Methods.List {
			if len(method.Names) == 0 { // Ignore embedded interfaces
				continue
			}
			methodName := method.Names[0].Name

			// Extract parameters and types properly
			var params []string
			if funcType, ok := method.Type.(*ast.FuncType); ok {
				for _, param := range funcType.Params.List {
					paramType := types.ExprString(param.Type) // Get a clean type name
					for _, paramName := range param.Names {
						params = append(params, fmt.Sprintf("%s %s", paramName.Name, paramType))
					}
					if len(param.Names) == 0 { // Handle unnamed parameters
						params = append(params, paramType)
					}
				}
			}

			methods = append(methods, fmt.Sprintf("    %s(%s)", methodName, strings.Join(params, ", ")))
		}

		return false
	})

	if len(methods) == 0 {
		return "", fmt.Errorf("ServerInterface not found or has no methods")
	}

	// Format the extracted methods as a structured string
	return strings.Join(methods, "\n"), nil
}
