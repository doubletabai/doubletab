package tooling

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
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
