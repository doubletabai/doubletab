# DoubleTab

[DoubleTab](https://www.doubletab.ai) is an open-source AI-powered development tool that helps users create software projects from scratch to production. Designed for developers, but accessible to anyone, it automates essential backend tasks such as database schema generation, API endpoint creation, and code generation—streamlining the development workflow.

![screenshot.png](screenshot.png)

## Features

- [x] OpenAPI 3.0 Spec - Generate OpenAPI spec based on your input in natural language.
- [x] Schema Generation – Postgres schema generated and applied based on OpenAPI spec.
- [x] API Generation – Automatically generate structured API endpoints.
- [x] Building - Make sure that the generated code is buildable. If not, fix it automatically.

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

Change to an empty directory where you want to create your project and run the following command:

```bash
PGHOST=<host> PGPORT=<port> PGDATABASE=<db_name> PGUSER=<user> PGPASSWORD=<secret> PGSSLMODE=disable OPENAI_API_KEY=<secret> doubletab
```

Make sure that you have a PostgreSQL database running and the necessary environment variables set. The `OPENAI_API_KEY` environment variable is required to use the OpenAI API for natural language processing. DoubleTab will guide you through the process of creating your project from the database schema to the API endpoints. Just describe what kind of project/application you want to create, and DoubleTab will try to come up with right solutions.

## Roadmap

- [ ] Standardized Codebase – Ensures consistency by following predefined coding patterns.
- [ ] Code Execution – Run and validate generated Go code securely.
- [ ] Memory - Remember user inputs and tools outputs to avoid endless loops of incorrect solutions.
- [ ] Ollama Integration – Integrate Ollama for local LLMs.
- [ ] Different DBs/languages – Support for different databases and programming languages.
- [ ] Tests Generation – Automatically generate and run tests for the generated code.
- [ ] Extensible Tools – Supports custom tools for additional automation.
- [ ] Custom Knowledge Base – Create and apply custom knowledge bases for specific domains.
