package tooling

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog/log"
)

const (
	generateSchemaPrompt = `You are an AI assistant that helps generate PostgreSQL schemas.

Based on given OpenAPI 3.0 spec, generate a PostgreSQL schema in a structured JSON format. The response must strictly
follow this format:

{
    "table_name": "<table_name>",
    "columns": [
        {"name": "<column_name>", "type": "<SQL_data_type>", "constraints": "<constraints_if_any>"},
        ...
    ]
}

- Ensure every table has a PRIMARY KEY.
- For IDs which are UUIDs, use TEXT data type without auto generation.
- Use appropriate SQL data types (e.g., TEXT, INT, TIMESTAMP).
- Prefer TEXT over VARCHAR.
- Set NOT NULL for required fields.
- Use UNIQUE constraints when necessary.
- Do NOT include CREATE TABLE statements, only structured JSON output.
- Do NOT add any additional fields that are not present in the OpenAPI spec (e.g., created_at, updated_at).
`
)

const ListTablesToolName = "list_tables"

func (s *Service) ListTablesTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(ListTablesToolName),
			Description: openai.String("List existing DB tables."),
		}),
	}
}

const GenerateSchemaToolName = "generate_schema"

func (s *Service) GenerateSchemaTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String(GenerateSchemaToolName),
			Description: openai.String("Generates a PostgreSQL schema in JSON format based on OpenAPI 3.0 specification."),
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

const StoreSchemaToolName = "store_schema"

func (s *Service) StoreSchemaTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Type: openai.F(openai.ChatCompletionToolTypeFunction),
		Function: openai.F(openai.FunctionDefinitionParam{
			Name:        openai.String("store_schema"),
			Description: openai.String("Takes generated schema in JSON format and creates a new PostgreSQL table."),
			Parameters: openai.F(openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"json_schema": map[string]string{
						"type": "string",
					},
				},
				"required": []string{"json_schema"},
			}),
		}),
	}
}

func (s *Service) ListTables(ctx context.Context) string {
	spinner, _ := pterm.DefaultSpinner.Start("Listing tables...")
	defer spinner.Stop()

	tables := make([]string, 0)
	if err := s.DB.SelectContext(ctx, &tables, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'"); err != nil {
		log.Fatal().Err(err).Msg("Failed to query database")
	}

	return strings.Join(tables, ", ")
}

func (s *Service) GenerateSchema(ctx context.Context, arguments string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Generating schema...")
	defer spinner.Stop()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	openAPISpec := args["openapi_spec"].(string)

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(generateSchemaPrompt),
			openai.UserMessage(openAPISpec),
		}),
		Model: openai.F(openai.ChatModelGPT4o),
		Seed:  openai.F(int64(0)),
	}

	completion, err := s.OpenAICli.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get completion")
	}

	return completion.Choices[0].Message.Content
}

type Schema struct {
	TableName string   `json:"table_name"`
	Columns   []Column `json:"columns"`
}

type Column struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Constraints string `json:"constraints"`
}

func (s *Service) StoreSchema(ctx context.Context, arguments string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Creating schema...")
	defer spinner.Stop()

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return fmt.Sprintf("Failed to unmarshal function arguments: %v", err)
	}
	schema := args["json_schema"].(string)

	var schemaObj Schema
	if err := json.Unmarshal([]byte(schema), &schemaObj); err != nil {
		return fmt.Sprintf("Failed to unmarshal json schema: %v", err)
	}

	query := fmt.Sprintf("CREATE TABLE %s (", schemaObj.TableName)
	for i, col := range schemaObj.Columns {
		query += fmt.Sprintf("%s %s %s", col.Name, col.Type, col.Constraints)
		if i < len(schemaObj.Columns)-1 {
			query += ", "
		}
	}
	query += ")"

	if _, err := s.DB.ExecContext(ctx, query); err != nil {
		return fmt.Sprintf("Failed to create table: %v", err)
	}

	return "Table created successfully"
}
