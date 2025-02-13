package tooling

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/oapi-codegen/oapi-codegen/v2/pkg/codegen"
	"github.com/oapi-codegen/oapi-codegen/v2/pkg/util"
	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
)

const GenerateAndStoreHandlersCodeToolName = "generate_and_store_handlers_code"

func (s *Service) GenerateAndStoreHandlersCodeTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(GenerateAndStoreHandlersCodeToolName),
			Description: openai.String("Takes OpenAPI 3.0 spec and creates appropriate file with Go code implementing handlers."),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"openapi_spec": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"openapi_spec"},
			}),
		}),
	}
}

func (s *Service) GenerateAndStoreHandlersCode(ctx context.Context, arguments string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Creating endpoints...")
	defer spinner.Stop()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	endpoints := args["openapi_spec"].(string)

	specTmpFile, err := os.CreateTemp(s.TmpDir, "openapi-*.yaml")
	if err != nil {
		return fmt.Sprintf("Failed to create temporary file for OpenAPI spec: %v", err)
	}

	if _, err := specTmpFile.WriteString(endpoints); err != nil {
		return fmt.Sprintf("Failed to write OpenAPI spec to temporary file: %v", err)
	}

	opts := codegen.Configuration{
		PackageName: "api",
		Generate: codegen.GenerateOptions{
			StdHTTPServer: true,
			Models:        true,
		},
		Compatibility: codegen.CompatibilityOptions{},
		OutputOptions: codegen.OutputOptions{
			Overlay: codegen.OutputOptionsOverlay{
				Path: specTmpFile.Name(),
			},
		},
	}
	overlayOpts := util.LoadSwaggerWithOverlayOpts{
		Path:   opts.OutputOptions.Overlay.Path,
		Strict: true,
	}
	swagger, err := util.LoadSwaggerWithOverlay(specTmpFile.Name(), overlayOpts)
	if err != nil {
		return fmt.Sprintf("Failed to load OpenAPI spec: %v", err)
	}
	code, err := codegen.Generate(swagger, opts)
	if err != nil {
		return fmt.Sprintf("Failed to generate code: %v", err)
	}

	log.Warn().Msgf("Code: %s", code)

	apiDir := path.Join(os.Getenv("PROJECT_ROOT"), "pkg", "api")
	if err := os.MkdirAll(apiDir, 0755); err != nil {
		return fmt.Sprintf("Failed to create directory")
	}

	return "Endpoints created successfully"
}
