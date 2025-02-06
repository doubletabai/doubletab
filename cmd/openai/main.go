package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/openai/openai-go"
	"github.com/rs/zerolog/log"
)

const (
	mainWorkflowPrompt = `You are an AI assistant that helps developers build backend applications step by step. Your
workflow always starts by defining a database schema before moving to API development. When a user describes an
application, first confirm their entities, then suggest a normalized PostgreSQL database schema before continuing.`
	generateSchemaPrompt = `You are an AI assistant that helps generate PostgreSQL schemas. 
When the user describes an application, extract entities and fields, then return a structured JSON representation of the schema. 
The response must strictly follow this format:

{
    "table_name": "<table_name>",
    "columns": [
        {"name": "<column_name>", "type": "<SQL_data_type>", "constraints": "<constraints_if_any>"},
        ...
    ]
}

- Use appropriate SQL data types (e.g., VARCHAR, INT, TIMESTAMP).
- Ensure every table has a PRIMARY KEY.
- Set NOT NULL for required fields.
- Use UNIQUE constraints when necessary.
- Do NOT include CREATE TABLE statements, only structured JSON output.`
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
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
		os.Getenv("PG_HOST"), os.Getenv("PG_PORT"), os.Getenv("PG_DB"), os.Getenv("PG_USER"), os.Getenv("PG_PASS"), os.Getenv("PG_SSL"))

	db, err := sqlx.ConnectContext(ctx, "postgres", conn)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	openAICli := openai.NewClient()

	ts := NewToolService(db, openAICli)

	question := "I want to develop a Contacts app. I will need object Contact with following fields: first_name, last_name, company_name, phone_number, email. For now single phone_number and email is enough."

	log.Info().Msgf("Asking: %s", question)

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(mainWorkflowPrompt),
			openai.UserMessage(question),
		}),
		Tools: openai.F([]openai.ChatCompletionToolParam{
			{
				Type: openai.F(openai.ChatCompletionToolTypeFunction),
				Function: openai.F(openai.FunctionDefinitionParam{
					Name:        openai.String("list_tables"),
					Description: openai.String("List existing DB tables."),
				}),
			},
			{
				Type: openai.F(openai.ChatCompletionToolTypeFunction),
				Function: openai.F(openai.FunctionDefinitionParam{
					Name:        openai.String("generate_schema"),
					Description: openai.String("Generates a PostgreSQL schema in JSON format based on user input about the domain."),
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
			},
			{
				Type: openai.F(openai.ChatCompletionToolTypeFunction),
				Function: openai.F(openai.FunctionDefinitionParam{
					Name:        openai.String("apply_schema"),
					Description: openai.String("Takes generated schema in JSON format and creates a PostgreSQL table."),
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
			},
			{
				Type: openai.F(openai.ChatCompletionToolTypeFunction),
				Function: openai.F(openai.FunctionDefinitionParam{
					Name:        openai.String("generate_endpoints"),
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
			},
		}),
		Model: openai.F(openai.ChatModelGPT4o),
	}

	for {
		// Make initial chat completion request
		completion, err := openAICli.Chat.Completions.New(ctx, params)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to get completion")
		}

		if completion.Choices[0].Message.Content != "" {
			fmt.Printf("LLM: %s\n", completion.Choices[0].Message.Content)
		}

		toolCalls := completion.Choices[0].Message.ToolCalls

		if len(toolCalls) == 0 && completion.Choices[0].FinishReason == "stop" {
			nextStep, err := getUserInput()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to get user input")
			}
			params.Messages.Value = append(params.Messages.Value, openai.UserMessage(nextStep))
			continue
		}

		// If there was a function call, continue the conversation
		params.Messages.Value = append(params.Messages.Value, completion.Choices[0].Message)
		for _, toolCall := range toolCalls {
			log.Debug().Msgf("Tool call: %v", toolCall.Function.Name)
			switch toolCall.Function.Name {
			case "list_tables":
				tables := ts.listTables(ctx)

				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, tables))
			case "generate_schema":
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					log.Fatal().Err(err).Msg("Failed to unmarshal function arguments")
				}
				userInput := args["user_input"].(string)

				schemaJson := ts.generateSchema(ctx, userInput)

				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, schemaJson))
			case "apply_schema":
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					log.Fatal().Err(err).Msg("Failed to unmarshal function arguments")
				}
				schema := args["json_schema"].(string)

				resp := ts.applySchema(ctx, schema)

				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, resp))
			case "generate_endpoints":
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					log.Fatal().Err(err).Msg("Failed to unmarshal function arguments")
				}
				schema := args["schema"].(string)

				endpointsJson := ts.generateEndpoints(ctx, schema)

				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, endpointsJson))
			}
		}
	}
}

func getUserInput() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("> ")

	// Read user input until newline
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	// Trim newline and print result
	return strings.TrimSpace(input), nil
}

type ToolService struct {
	DB        *sqlx.DB
	OpenAICli *openai.Client
}

func NewToolService(db *sqlx.DB, cli *openai.Client) *ToolService {
	return &ToolService{
		DB:        db,
		OpenAICli: cli,
	}
}

func (s *ToolService) listTables(ctx context.Context) string {
	tables := make([]string, 0)
	if err := s.DB.SelectContext(ctx, &tables, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'"); err != nil {
		log.Fatal().Err(err).Msg("Failed to query database")
	}

	return strings.Join(tables, ", ")
}

func (s *ToolService) generateSchema(ctx context.Context, question string) string {
	log.Debug().Msgf("Creating schema for question: %s", question)
	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(generateSchemaPrompt),
			openai.UserMessage(question),
		}),
		Model: openai.F(openai.ChatModelGPT4o),
	}

	completion, err := s.OpenAICli.Chat.Completions.New(ctx, params)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get completion")
	}

	return completion.Choices[0].Message.Content
}

func (s *ToolService) generateEndpoints(ctx context.Context, schema string) string {
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

type Schema struct {
	TableName string   `json:"table_name"`
	Columns   []Column `json:"columns"`
}

type Column struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Constraints string `json:"constraints"`
}

func (s *ToolService) applySchema(ctx context.Context, schema string) string {
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

func setupDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "vectors.db")
	if err != nil {
		return nil, err
	}

	// Enable vector extension (SQLite version of PGVector)
	_, err = db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS embeddings USING vector(1536)") // 1536 dims for OpenAI embeddings
	if err != nil {
		return nil, err
	}

	return db, nil
}

func storeEmbedding(db *sql.DB, text string, embedding []float64) error {
	_, err := db.Exec("INSERT INTO embeddings (text, vector) VALUES (?, ?)", text, embedding)
	return err
}
