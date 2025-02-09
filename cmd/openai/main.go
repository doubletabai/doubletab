package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/openai/openai-go"
	"github.com/pterm/pterm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	mainWorkflowPrompt = `You are an AI assistant that helps developers build backend applications step by step. Your
workflow always starts by defining a database schema before moving to API development. When a user describes an
application, first confirm their entities, then suggest a normalized PostgreSQL database schema before continuing.
Next, generate REST API endpoints for the schema, and finally, generate Go code for the API handlers.`
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
	applyEndpointsPrompt = `You are an AI assistant that helps developers generate REST API handler functions in Go for
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

func main() {
	lvl, err := zerolog.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to parse log level")
	}
	zerolog.SetGlobalLevel(lvl)

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

	pterm.DefaultBasicText.Println("Welcome to the" + pterm.LightMagenta(" DoubleTab ") + "AI assistant for backend development! What would you like to build today?")
	question := os.Getenv("INITIAL_QUERY")
	if question != "" {
		fmt.Printf("> %s\n", question)
	} else {
		question, err = pterm.DefaultInteractiveTextInput.WithDefaultText(">").WithDelimiter(" ").Show()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to get user input")
		}
	}

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
			{
				Type: openai.F(openai.ChatCompletionToolTypeFunction),
				Function: openai.F(openai.FunctionDefinitionParam{
					Name:        openai.String("apply_endpoints"),
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
			},
		}),
		Model: openai.F(openai.ChatModelGPT4o),
	}

	for {
		stream := openAICli.Chat.Completions.NewStreaming(ctx, params)
		acc := openai.ChatCompletionAccumulator{}

		begin := false
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)
			if !begin && chunk.Choices[0].Delta.Content != "" {
				begin = true
				pterm.DefaultBasicText.Print(pterm.LightMagenta("DoubleTab: "))
			}
			pterm.DefaultBasicText.Print(chunk.Choices[0].Delta.Content)
		}
		if stream.Err() != nil {
			log.Fatal().Err(stream.Err()).Msg("Failed to stream completion")
		}
		if begin {
			pterm.DefaultBasicText.Println()
		}

		toolCalls := acc.Choices[0].Message.ToolCalls
		log.Debug().Msgf("Tool calls: %v", toolCalls)
		log.Debug().Msgf("Finish reason: %s", acc.Choices[0].FinishReason)

		if len(toolCalls) == 0 && acc.Choices[0].FinishReason == "stop" {
			params.Messages.Value = append(params.Messages.Value, acc.Choices[0].Message)
			nextStep, err := pterm.DefaultInteractiveTextInput.WithDefaultText(">").WithDelimiter(" ").Show()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to get user input")
			}
			params.Messages.Value = append(params.Messages.Value, openai.UserMessage(nextStep))
			stream.Close()
			continue
		}

		params.Messages.Value = append(params.Messages.Value, acc.Choices[0].Message)
		for _, toolCall := range toolCalls {
			//pterm.DefaultBasicText.Printf("Using tool: %s\n", toolCall.Function.Name)
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
			case "apply_endpoints":
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					log.Fatal().Err(err).Msg("Failed to unmarshal function arguments")
				}
				endpointsJson := args["endpoints"].(string)

				result := ts.applyEndpoints(ctx, endpointsJson)

				params.Messages.Value = append(params.Messages.Value, openai.ToolMessage(toolCall.ID, result))
			}
		}
		stream.Close()
	}
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
	spinner, _ := pterm.DefaultSpinner.Start("Listing tables...")
	defer spinner.Stop()

	tables := make([]string, 0)
	if err := s.DB.SelectContext(ctx, &tables, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'"); err != nil {
		log.Fatal().Err(err).Msg("Failed to query database")
	}

	return strings.Join(tables, ", ")
}

func (s *ToolService) generateSchema(ctx context.Context, question string) string {
	spinner, _ := pterm.DefaultSpinner.Start("Generating schema...")
	defer spinner.Stop()

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
	spinner, _ := pterm.DefaultSpinner.Start("Creating schema...")
	defer spinner.Stop()

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

func (s *ToolService) generateEndpoints(ctx context.Context, schema string) string {
	log.Debug().Msgf("Creating endpoints for schema: %s", schema)
	spinner, _ := pterm.DefaultSpinner.Start("Generating endpoints...")
	defer spinner.Stop()

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

func (s *ToolService) applyEndpoints(ctx context.Context, endpoints string) string {
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

	spinner, _ := pterm.DefaultSpinner.Start("Creating endpoints...")
	defer spinner.Stop()

	params := openai.ChatCompletionNewParams{
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(applyEndpointsPrompt),
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
