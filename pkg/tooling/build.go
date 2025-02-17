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

const BuildCodeToolName = "build_code"

func (s *Service) BuildCodeTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(BuildCodeToolName),
			Description: openai.String("Builds Go code generated based on previously generated OpenAPI 3.0 spec."),
		}),
	}
}

func (s *Service) BuildCode(ctx context.Context) string {
	spinner, _ := pterm.DefaultSpinner.Start("Building code...")
	defer spinner.Stop()

	absRoot, err := filepath.Abs(os.Getenv("PROJECT_ROOT"))
	if err != nil {
		return fmt.Sprintf("Failed to get absolute path of project root: %v", err)
	}
	cmd := exec.CommandContext(ctx, "go", "build")
	cmd.Dir = absRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("go build failed: %v\n%s", err, output)
	}

	return "Code built successfully"
}
