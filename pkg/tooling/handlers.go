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
)

const (
	generateServiceCodePrompt = `You are an AI assistant that generates Go code implementing service based on previously generated OpenAPI 3.0 spec.

Generate Go code implementing ServerInterface generated by oapi-codegen based on previously generated OpenAPI 3.0 spec.
Typically you'd have to implement 5 handlers for CRUD operations (ListResources, GetResource, CreateResource, UpdateResource, DeleteResource).
Follow the example below:

---
%s
---

The ServerInterface interface was generated as follows:

---
%s
---

Important notes:
- Implement the methods only, do not create any extra types. Assume that types defined in the OpenAPI spec are already
  available.
- Return only valid Go code in raw format (without go code block markdown syntax) and without any other comments (like
  "I did this and that").
`
)

const (
	serviceGo = `package api

import (
	"encoding/json"
	"net/http"

	"github.com/jmoiron/sqlx"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type Service struct {
	DB *sqlx.DB
}

func (s Service) ListResources(w http.ResponseWriter, r *http.Request) {
	resources := []Resource{}
	err := s.DB.SelectContext(r.Context(), &resources, "SELECT * FROM resources")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(resources); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// TODO: Implement the rest of the handlers: GetResource, CreateResource, UpdateResource, DeleteResource
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

const GenerateServiceCodeToolName = "generate_service_code"

func (s *Service) GenerateServiceCodeTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(GenerateServiceCodeToolName),
			Description: openai.String("Generates Go code implementing service based on previously generated OpenAPI 3.0 spec."),
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

func (s *Service) GenerateServiceCode(ctx context.Context, arguments string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Generating service code...")
	defer spinner.Stop()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	userInput := args["user_input"].(string)

	log.Info().Msgf("Creating service code with user input: %s", userInput)

	methods, err := extractServerInterfaceMethods(filepath.Join(os.Getenv("PROJECT_ROOT"), "pkg", "api", "handlers.gen.go"))
	if err != nil {
		return fmt.Sprintf("Failed to extract ServerInterface methods: %v", err)
	}
	iface := fmt.Sprintf("type ServerInterface interface {\n%s\n}\n", methods)

	prompt := fmt.Sprintf(generateServiceCodePrompt, serviceGo, iface)

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(prompt),
			openai.UserMessage(userInput),
		}),
		Model: openai.F(openai.ChatModelGPT4oMini),
		Seed:  openai.Int(1),
	}

	completion, err := s.OpenAICli.Chat.Completions.New(ctx, params)
	if err != nil {
		return fmt.Sprintf("Failed to get completion: %v", err)
	}

	apiDir := path.Join(os.Getenv("PROJECT_ROOT"), "pkg", "api")
	fh, err := os.Create(path.Join(apiDir, "service.go"))
	if err != nil {
		return fmt.Sprintf("Failed to create service.go file: %v", err)
	}
	defer fh.Close()

	var rawCode string
	parts := strings.Split(completion.Choices[0].Message.Content, "```go")
	if len(parts) == 2 {
		rawCode = parts[1]
		parts = strings.Split(rawCode, "```")
		rawCode = parts[0]
	} else {
		rawCode = completion.Choices[0].Message.Content
	}

	_, err = fh.WriteString(rawCode)
	if err != nil {
		return fmt.Sprintf("Failed to write service.go file: %v", err)
	}

	return rawCode
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
