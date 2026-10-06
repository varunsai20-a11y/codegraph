# CodeGraph

> AI-powered repository intelligence and code understanding.

CodeGraph is an AI-assisted developer tool designed to help engineers understand unfamiliar repositories without manually reading thousands of lines of code.

Instead of treating a codebase as plain unparsed text and asking an LLM to guess how it works, CodeGraph parses source files into AST representations, extracts symbols, builds cross-file relationship graphs (calls, imports, references), performs hybrid RAG retrieval (lexical, vector, graph), evaluates evidence sufficiency, and synthesizes grounded explanations backed by verifiable inline citations.

---

## Overview

Understanding complex codebases is one of the most time-consuming tasks in software engineering. Traditional approaches require manually reading documentation, following imports, tracing function calls line by line, and guessing runtime flow.

CodeGraph automates this reverse engineering workflow by combining deterministic static analysis with evidence-grounded LLM reasoning:

```text
┌─────────────────────────────────────────────────────────────────────────┐
│                           Source Code Repository                        │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                    AST Analysis & Language Detection                    │
│      Symbol Extraction: Functions, Classes, Methods, Variables, Types   │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                            Code Graph Engine                            │
│         CALLS │ IMPORTS │ REFERENCES │ CONTAINS │ INHERITS               │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         Hybrid RAG Retrieval Engine                     │
│         Lexical Search    +    Vector Embeddings    +   Graph Hops      │
│                            Reciprocal Rank Fusion                       │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                      Query-Aware Sufficiency Engine                     │
│        Evaluates evidence completeness & prevents hallucinated flows    │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                   LLM Provider (Ollama / Gemini / Groq)                 │
│         Generates structured explanations with citation validation      │
└────────────────────────────────────┬────────────────────────────────────┘
                                     │
                                     ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                       Interactive Visual Workstation                    │
│          Graph View │ Flow Inspector │ File Explorer │ AI Chat          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## What CodeGraph Does

* **Answers Architectural Questions**: Explains repository entry points, module boundaries, system responsibilities, and overall project structure.
* **Traces Request & Feature Flows**: Reconstructs cross-file execution paths from HTTP entry points down to database and service handlers.
* **Detects Missing & Unsupported Features**: Evaluates query-aware evidence sufficiency. If a requested technology or feature is absent from the repository (e.g., asking about Redis sharding on a repo without Redis), CodeGraph rejects the request cleanly without generating hallucinated code flows.
* **Generates Structured Explanations**: Produces human-readable explanations divided into clear Markdown sections (*What it does*, *How the code works*, *In simpler terms*, *Important details*, *Evidence*).
* **Validates Citations**: Exposes interactive, clickable citations (`[E1]`, `[E2]`) that jump directly to exact line numbers in the file explorer and code graph.

---

## Key Features

* 🧠 **Grounded AI Explanations**: Answers are backed by AST symbols and cross-file relationships rather than open-ended model hallucinations.
* 🕸️ **Interactive Code Graph**: Visualizes symbol nodes and relationship edges using Dagre and `@xyflow/react`.
* 🔀 **Feature & Request Flow Tracing**: Multi-hop call path traversal for inspecting function callers, callees, and execution paths.
* 🛡️ **Anti-Hallucination & Sufficiency Check**: Automatically flags insufficient evidence and skips LLM generation when required code references are missing.
* 💻 **Local-First LLM Support**: Native integration with local **Ollama** (`qwen2.5:3b`) for private, offline code analysis.
* ☁️ **Cloud LLM Cascade**: Fallback support for **Google Gemini** (`gemini-3.8-flash`) and **Groq** (`openai/gpt-oss-120b`).
* 📁 **Multi-Language Ingestion**: Analyzes TypeScript, JavaScript, Python, Go, Java, Rust, C, C++, JSON, Markdown, YAML, TOML, SQL, HTML, and CSS.
* 🔒 **Local & Privacy-Preserving**: Runs entirely on your local machine using SQLite and local workspace indexing.

---

## How It Works

1. **Ingestion & AST Analysis**: When a repository is registered, CodeGraph scans all source files, detects programming languages, extracts symbol declarations, and indexes structural tokens into SQLite.
2. **Graph Construction**: Builds an in-memory graph representing `CALLS`, `IMPORTS`, `REFERENCES`, `INHERITS`, and `CONTAINS` relationships across files.
3. **Intent Classification & Hybrid Retrieval**: User queries are classified into intents (e.g., `TRACE`, `ARCHITECTURE`, `FILE_QUERY`, `SYMBOL_LOOKUP`). CodeGraph performs Lexical BM25-style search, Vector similarity search, and Graph traversal, combining candidates via Reciprocal Rank Fusion (RRF).
4. **Sufficiency Evaluation**: Evaluates whether retrieved evidence covers the query. If specific requested technical terms are missing from the codebase, it flags `SUFFICIENT_INSUFFICIENT` and returns a concise, ungrounded notification.
5. **Context Assembly & LLM Synthesis**: Builds a prompt package containing evidence snippets and graph relationship chains, sending it to the configured LLM provider (Ollama, Gemini, or Groq).
6. **Citation Validation & Rendering**: Validates that all generated `[E1]`, `[E2]` citation tags map to real retrieved evidence, rendering the response in the interactive Markdown Workstation UI.

---

## Architecture

```text
codegraph/
├── cmd/
│   └── codegraph/           # Main Go backend application entry point
├── internal/
│   ├── api/                 # Chi REST API routes, handlers, & CORS middleware
│   ├── config/              # Environment configuration loader (.env)
│   ├── evaluation/          # E2E acceptance, feature flow, & benchmark tests
│   ├── graph/               # In-memory graph engine, traversal, & neighborhood algorithms
│   ├── guide/               # 5-step automated reverse engineering orchestrator
│   ├── ingestion/           # File scanner, AST symbol extraction, & indexer
│   ├── language/            # Multi-language detector & extension mapping
│   ├── llm/                 # Ollama, Gemini, Groq providers & prompt builders
│   ├── models/              # Core domain models, nodes, edges, & evidence structures
│   ├── repository/          # Workspace file manager & directory isolation
│   ├── retrieval/           # Lexical, Vector, Graph, Hybrid RRF, & Sufficiency engine
│   ├── security/            # Path sanitization, secret filtering, & URL validator
│   ├── storage/             # Pure-Go SQLite storage engine & schema
│   └── vector/              # SQLite-backed vector store & embedding providers
├── frontend/                # Next.js 15 / React 19 / TypeScript UI
│   ├── components/
│   │   ├── ai/              # AI Workstation & Explanation Panel
│   │   ├── explorer/        # Code File Explorer & Line Highlight Viewer
│   │   ├── flow/            # Flow Tracer & Execution Inspector
│   │   ├── graph/           # Interactive Graph Visualizer (@xyflow/react)
│   │   ├── guide/           # Reverse Engineering 5-Step Guide UI
│   │   └── ui/              # MarkdownRenderer & UI components
│   ├── lib/                 # API client & TypeScript type definitions
│   └── store/               # Zustand state management
├── _workspaces/             # Indexed local repository workspace root
├── codegraph.db             # Local SQLite database
├── .env.example             # Example environment configuration file
└── go.mod                   # Go module definitions
```

---

## Repository Understanding

CodeGraph extracts structural declarations from source files without executing code:

* **Functions & Methods**: Parameters, return types, start/end line locations.
* **Classes, Structs & Interfaces**: Fields, methods, and inheritance links.
* **Imports & Dependencies**: Module import statements and package declarations.
* **Variables & Constants**: Top-level exports and definitions.

---

## Code Graph

CodeGraph constructs an in-memory graph representing structural code relationships:

| Edge Type | Description | Example |
| :--- | :--- | :--- |
| `CALLS` | Function or method invocation | `main()` → `startServer()` |
| `IMPORTS` | Cross-file or package import | `routing.py` → `dependencies/utils.py` |
| `REFERENCES` | Symbol usage or variable reference | `handler` → `DBConfig` |
| `CONTAINS` | Parent-child AST containment | `APIRoute` class → `get_request_handler()` method |
| `INHERITS` | Class extension or interface implementation | `CustomRouter` → `BaseRouter` |

---

## Retrieval & Evidence

Retrieval combines three complementary strategies using **Reciprocal Rank Fusion (RRF)**:

1. **Lexical Retrieval**: Full-text and symbol matching across indexed filenames and source code snippets.
2. **Vector Retrieval**: Semantic similarity matching using embeddings (`gemini-embedding-001` or local mock embedding fallback).
3. **Graph Traversal**: Multi-hop traversal starting from seed symbols to gather connected callers, callees, and imported modules.

---

## Flow & Feature Understanding

CodeGraph traces complete feature flows across multiple files:

* **Entry Point Discovery**: Identifies HTTP route handlers, CLI entry points, and main execution loops.
* **Call Chain Traversal**: Follows `CALLS` and `IMPORTS` edges to connect entry points with intermediate services and data stores.
* **Evidence Packaging**: Aggregates connected call paths into unified evidence packages.

---

## Grounding & Sufficiency

Before invoking an LLM, CodeGraph's **Query-Aware Sufficiency Engine** checks whether retrieved evidence is sufficient:

* **Technology Term Extraction**: Extracts specific technical terms from the query (e.g. `redis`, `kafka`, `graphql`, `webhooks`).
* **Absent Feature Detection**: If requested technical terms are completely absent from the codebase, CodeGraph marks the evidence `SUFFICIENT_INSUFFICIENT`.
* **Fallback Notification**: Returns `Insufficient evidence available in repository to generate a fully grounded answer.` without wasting LLM tokens or inventing non-existent features.

---

## AI Explanations

AI explanations are formatted into five distinct Markdown sections:

```markdown
### What it does
Summary of the feature or architectural module.

### How the code works
Step-by-step description of execution flow, key functions, and modules.

### In simpler terms
Plain-English summary, including ASCII flow diagrams:
run.py ──> check_ollama_server() ──> CrewAI Orchestrator ──> Final Report

### Important details
Key parameters, caching strategies, error handling, or performance notes.

### Evidence
- [E1] `run.py` (L10-L45) — Main entry point
- [E2] `crew.py` (L20-L85) — Workflow orchestrator
```

---

## LLM Providers

CodeGraph supports three LLM providers:

| Provider | Type | Model | Verification Status | Configuration Key |
| :--- | :--- | :--- | :--- | :--- |
| **Ollama** *(Default)* | Local | `qwen2.5:3b` | **Verified Locally** | `CODEGRAPH_LLM_PROVIDER=ollama` |
| **Google Gemini** | Cloud | `gemini-3.8-flash` | Supported | `GEMINI_API_KEY=your_key` |
| **Groq** | Cloud | `openai/gpt-oss-120b` | Supported | `GROQ_API_KEY=your_key` |

---

## Supported Languages & File Types

* **TypeScript / TSX** (`.ts`, `.tsx`, `.mts`, `.cts`, `.astro`)
* **JavaScript / JSX** (`.js`, `.jsx`, `.mjs`, `.cjs`)
* **Python** (`.py`, `.pyw`)
* **Go** (`.go`)
* **Java** (`.java`)
* **C / C++** (`.c`, `.h`, `.cpp`, `.hpp`, `.cc`, `.cxx`)
* **Rust** (`.rs`)
* **HTML / CSS / SCSS** (`.html`, `.htm`, `.css`, `.scss`, `.less`)
* **Data & Config** (`.json`, `.jsonc`, `.md`, `.mdx`, `.yaml`, `.yml`, `.toml`, `.sql`, `.sh`, `.bash`, `.zsh`)

---

## System Requirements

* **Operating System**: Windows 10/11, macOS, or Linux
* **Go**: Version `1.23` or higher
* **Node.js**: Version `18.0.0` or higher (npm `9+`)
* **Ollama** *(Recommended for local LLM)*: Ollama v0.1.30+ with `qwen2.5:3b` model pulled

---

## Quick Start

### Windows (PowerShell)

```powershell
# 1. Clone the repository
git clone https://github.com/varunsai20-a11y/codegraph.git
cd codegraph

# 2. Download Go dependencies & install frontend packages
go mod download
Set-Location frontend; npm install; Set-Location ..

# 3. Create configuration file
Copy-Item .env.example .env

# 4. Start Go backend (Terminal 1)
go run ./cmd/codegraph

# 5. Start Next.js frontend (Terminal 2)
Set-Location frontend
npm run dev
```

### macOS / Linux (Bash / Zsh)

```bash
# 1. Clone the repository
git clone https://github.com/varunsai20-a11y/codegraph.git
cd codegraph

# 2. Download Go dependencies & install frontend packages
go mod download
(cd frontend && npm install)

# 3. Create configuration file
cp .env.example .env

# 4. Start Go backend (Terminal 1)
go run ./cmd/codegraph

# 5. Start Next.js frontend (Terminal 2)
cd frontend
npm run dev
```

Open [http://localhost:3000](http://localhost:3000) in your browser.

---

## Full Setup

Follow these step-by-step instructions to set up CodeGraph from scratch.

### 1. Clone the repository

```bash
git clone https://github.com/varunsai20-a11y/codegraph.git
cd codegraph
```

### 2. Install Go dependencies

Verify Go installation (v1.23+ required):

```bash
go version
go mod download
```

### 3. Install frontend dependencies

```bash
cd frontend
npm install
cd ..
```

### 4. Configure environment

Create `.env` from `.env.example`:

**Windows PowerShell:**
```powershell
Copy-Item .env.example .env
```

**macOS / Linux:**
```bash
cp .env.example .env
```

Edit `.env` to configure your preferred settings:

```env
PORT=8080
MAX_FILE_SIZE=5242880
WORKSPACE_ROOT=./_workspaces
DATABASE_PATH=codegraph.db

# Choose LLM Provider: 'ollama', 'gemini', or 'groq'
CODEGRAPH_LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://localhost:11434
OLLAMA_MODEL=qwen2.5:3b
CODEGRAPH_LLM_TIMEOUT_SECONDS=180

# Cloud LLM Credentials (Optional)
GEMINI_API_KEY=
GROQ_API_KEY=
GROQ_MODEL=openai/gpt-oss-120b
LLM_PROVIDER_ORDER=gemini,groq
```

> **Security Note**: Never commit your `.env` file or raw API keys to Git repository history. `.env` is included in `.gitignore`.

### 5. Configure an LLM provider

Choose **one** of the following options:

* **Option A: Local Ollama (Recommended)** — Install Ollama and pull `qwen2.5:3b` (see [Ollama Setup](#ollama-setup-local-llm)).
* **Option B: Google Gemini** — Set `GEMINI_API_KEY` in `.env` (see [Gemini Setup](#gemini-setup-cloud-llm)).
* **Option C: Groq** — Set `GROQ_API_KEY` in `.env` (see [Groq Setup](#groq-setup-cloud-llm)).

### 6. Start the backend

Run from the repository root:

```bash
go run ./cmd/codegraph
```

Output:

```text
Starting CodeGraph server configuration loaded. Port: 8080
[LLM] Initialized Local Ollama Provider (model: qwen2.5:3b, endpoint: http://localhost:11434/api/chat, timeout: 3m0s)
CodeGraph backend listening on http://localhost:8080
```

### 7. Start the frontend

In a new terminal window:

```bash
cd frontend
npm run dev
```

Output:

```text
▲ Next.js 15.5.25
- Local: http://localhost:3000
✓ Ready in 2s
```

### 8. Open CodeGraph

Navigate to [http://localhost:3000](http://localhost:3000) in your web browser.

### 9. Index a repository

1. Click **Sync Workspace** / **Add Repository** in the sidebar.
2. Select **LOCAL** and enter the absolute file path to a local project directory (e.g., `C:\Users\varun\projects\my-app` or `/Users/varun/projects/my-app`), or select **GIT** and paste a public GitHub URL.
3. Click **Register & Index**.
4. CodeGraph will analyze the repository, extract symbols, build relationships, and populate the database.

### 10. Ask your first question

Switch to the **AI Workstation** tab and ask your first question:

> *"Explain the architecture of this repository."*

Verify that the response status shows `ProviderMode: LOCAL_LLM` (when using Ollama) and renders structured Markdown sections backed by clickable citations.

---

## Ollama Setup (Local LLM)

For free, private, offline execution without cloud API keys:

1. **Install Ollama**: Download from [ollama.com](https://ollama.com/download) and install for your OS.
2. **Start Ollama Service**:
   ```bash
   ollama serve
   ```
3. **Pull `qwen2.5:3b` Model**:
   ```bash
   ollama pull qwen2.5:3b
   ```
4. **Verify Ollama Status**:
   ```bash
   curl http://localhost:11434/api/tags
   ```
5. **Configure `.env`**:
   ```env
   CODEGRAPH_LLM_PROVIDER=ollama
   OLLAMA_BASE_URL=http://localhost:11434
   OLLAMA_MODEL=qwen2.5:3b
   CODEGRAPH_LLM_TIMEOUT_SECONDS=180
   ```
6. **Hardware Performance Note**: Ollama automatically utilizes GPU acceleration (CUDA, Metal, ROCm) when available. CPU execution is fully supported; inference speed will depend on your local CPU core count and system RAM.

---

## Gemini Setup (Cloud LLM)

To use Google Gemini for cloud inference:

1. Obtain an API key from [Google AI Studio](https://aistudio.google.com/).
2. Edit `.env`:
   ```env
   CODEGRAPH_LLM_PROVIDER=gemini
   GEMINI_API_KEY=your_gemini_api_key_here
   ```

---

## Groq Setup (Cloud LLM)

To use Groq for fast cloud inference:

1. Obtain an API key from [console.groq.com](https://console.groq.com/).
2. Edit `.env`:
   ```env
   CODEGRAPH_LLM_PROVIDER=groq
   GROQ_API_KEY=your_groq_api_key_here
   GROQ_MODEL=openai/gpt-oss-120b
   ```

---

## Configuration Reference

| Variable | Description | Default Value |
| :--- | :--- | :--- |
| `PORT` | Go backend HTTP port | `8080` |
| `MAX_FILE_SIZE` | Max indexable file size in bytes | `5242880` (5 MB) |
| `WORKSPACE_ROOT` | Directory storing cloned/copied workspace files | `./_workspaces` |
| `DATABASE_PATH` | Path to local SQLite database | `codegraph.db` |
| `ALLOWED_ORIGINS` | Comma-separated list of allowed CORS origins | `http://localhost:3000,http://127.0.0.1:3000` |
| `CODEGRAPH_LLM_PROVIDER` | Active LLM provider (`ollama`, `gemini`, `groq`) | `ollama` |
| `OLLAMA_BASE_URL` | Base URL for local Ollama instance | `http://localhost:11434` |
| `OLLAMA_MODEL` | Model tag for Ollama | `qwen2.5:3b` |
| `CODEGRAPH_LLM_TIMEOUT_SECONDS` | Request timeout for local LLM inference | `180` |
| `GEMINI_API_KEY` | Google AI Studio Gemini API key | *(empty)* |
| `GROQ_API_KEY` | Groq Console API key | *(empty)* |
| `GROQ_MODEL` | Model identifier for Groq | `openai/gpt-oss-120b` |
| `LLM_PROVIDER_ORDER` | Fallback provider cascade order | `gemini,groq` |

---

## Development & Testing

### Running Backend Tests

```bash
# Run backend retrieval tests
go test ./internal/retrieval

# Run backend LLM service tests
go test ./internal/llm

# Run feature-level code understanding E2E test suite
go test ./internal/evaluation -run TestFeatureLevelCodeUnderstanding

# Run all backend unit and integration tests
go test ./...
```

### Running Frontend Checks

```bash
# Verify TypeScript types and build production bundle
cd frontend
npm run build
```

---

## Troubleshooting

### 1. Ollama Connection Refused / Timeout
* **Symptom**: `Failed to initialize Ollama provider` or timeout error in backend logs.
* **Fix**: Ensure Ollama service is running (`ollama serve`) and the model is pulled (`ollama pull qwen2.5:3b`). Test response using `curl http://localhost:11434/api/tags`.

### 2. Model Not Installed in Ollama
* **Symptom**: `model "qwen2.5:3b" not found`.
* **Fix**: Run `ollama pull qwen2.5:3b` in terminal to download model weights.

### 3. Backend Startup Failure / Port Conflict
* **Symptom**: `bind: address already in use` or port 8080 conflict.
* **Fix**: Change `PORT=8081` in `.env` or terminate the process occupying port 8080.

### 4. Frontend Dependency / Build Error
* **Symptom**: `Module not found` or TypeScript error during `npm run dev`.
* **Fix**: Run `cd frontend && npm ci` or `npm install` to ensure node_modules are cleanly installed.

### 5. Repository Indexing Failure
* **Symptom**: Index job status turns to `FAILED`.
* **Fix**: Check directory permissions for `WORKSPACE_ROOT` (`./_workspaces`). Ensure repository files are not locked by another process.

### 6. Insufficient Evidence Notification
* **Symptom**: Response displays `Insufficient evidence available in repository to generate a fully grounded answer.`
* **Fix**: This is an intended security/sufficiency check. It indicates the query asked about a specific technology, symbol, or feature absent from the indexed repository.

---

## Limitations

* **Static Analysis Scope**: Static analysis cannot trace dynamic runtime execution (e.g., dynamic `eval()`, complex reflection, or unindexed external API contracts).
* **Local CPU Execution Speed**: Local LLM inference speed depends on system CPU/GPU hardware. Running Ollama on older CPUs without GPU acceleration may require longer timeout settings (`CODEGRAPH_LLM_TIMEOUT_SECONDS=300`).
* **Database Scaling**: Extremely large repositories with millions of lines may require setting a larger `MAX_FILE_SIZE` and increasing SQLite timeout limits.

---

## Roadmap

- [x] Multi-language AST symbol & relationship extraction
- [x] Cross-file code relationship graph engine (Calls, Imports, References)
- [x] Hybrid RAG retrieval with Reciprocal Rank Fusion (RRF)
- [x] Query-aware evidence sufficiency evaluation
- [x] Multi-repository feature flow understanding & verification
- [x] Local Ollama `qwen2.5:3b` integration designed to reduce unsupported generation
- [x] Interactive Markdown rendering with custom citation badges & ASCII diagram support
- [ ] Language Server Protocol (LSP) integration for real-time type definitions
- [ ] Tree-sitter native C-bindings for deep syntax parsing across 20+ additional languages

---

## Author

**Varun Sai**  
GitHub: [https://github.com/varunsai20-a11y](https://github.com/varunsai20-a11y)

---

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository on GitHub.
2. Create a feature branch (`git checkout -b feature/my-feature`).
3. Ensure all tests pass (`go test ./...` and `cd frontend && npm run build`).
4. Commit your changes with conventional commit messages (`git commit -m "feat: add my feature"`).
5. Push to your branch and open a Pull Request.

---

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
