# DoubleTab

[DoubleTab](https://www.doubletab.ai) is an open-source AI-powered development tool that helps users create software projects from scratch to production. Designed for developers, but accessible to anyone, it automates essential backend tasks such as database schema generation, API endpoint creation, and code generation—streamlining the development workflow.

![screenshot.png](screenshot.png)

## Features

- [x] OpenAPI 3.0 Spec - Generate OpenAPI spec based on your input in natural language.
- [x] Schema Generation – Postgres schema generated and applied based on OpenAPI spec.
- [x] API Generation – Automatically generate structured API endpoints.
- [x] Building - Make sure that the generated code is buildable. If not, fix it automatically.
- [x] Ollama Integration – Integrate Ollama for local LLMs.

## Installation

To install and use DoubleTab, you need to have Go installed on your machine. You can download it from the [official website](https://golang.org/dl/).

Once you have Go installed, you can install DoubleTab by running the following command:

```bash
go install github.com/doubletabai/doubletab@latest
```

You should now be able to run the `doubletab` command from your terminal. Make sure to add the Go bin directory to your PATH if you haven't already.

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

## Usage

Change to an empty directory where you want to create your project. Before running `doubletab`, make sure that it has
configuration for the database and LLM connection. There are two ways to provide the configuration:

1. Flags - You can provide the configuration using flags. Run `doubletab -h` to see the available flags. Example minimal
   usage with OpenAI LLM:

   ```bash
   doubletab --pg-user <user> --pg-database <project_db> --pg-password <password> --dt-pg-user <user> --dt-pg-password <password> --openai-api-key <key>
   ```

   DoubleTab is using two databases: one for the project and one for the tool. The `--pg-user`, `--pg-database`, and `--pg-password` flags are used for the project database, and the `--dt-pg-user` and `--dt-pg-password` flags are used for the tool database (by default, the tool database is `doubletab`, but can be overwritten).

2. Environment variables - You can provide the configuration using environment variables. Each flag has a corresponding
    environment variable. Example:
 
   ```bash
    export PG_USER=<user>
    export PG_DATABASE=<project_db>
    export PG_PASSWORD=<password>
    export DT_PG_USER=<user>
    export DT_PG_PASSWORD=<password>
    export OPENAI_API_KEY=<key>
    doubletab
    ```

### Ollama Example

To use local LLMs, you need to have Ollama running. Then, configure `dobuletab` with the following additional flags:

```bash
doubletab <...pg flags...> --llm-base-url http://127.0.0.1:11434/v1/v1 --llm-embedding-model nomic-embed-text --llm-chat-model llama3.3 --llm-code-model llama3.3
```

## Roadmap

- [ ] Standardized Codebase – Ensures consistency by following predefined coding patterns.
- [ ] Code Execution – Run and validate generated Go code securely.
- [ ] Memory - Remember user inputs and tools outputs to avoid endless loops of incorrect solutions.
- [ ] Different DBs/languages – Support for different databases and programming languages.
- [ ] Tests Generation – Automatically generate and run tests for the generated code.
- [ ] Extensible Tools – Supports custom tools for additional automation.
- [ ] Custom Knowledge Base – Create and apply custom knowledge bases for specific domains.
