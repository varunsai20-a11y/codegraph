# CodeGraph

> **Grounded Architecture Intelligence & Code Understanding Engine**

CodeGraph is a high-performance code intelligence platform designed for deep codebase indexing, static call graph analysis, grounded AI explanations, and guided reverse-engineering tours. It combines static AST analysis across multiple languages with an embedded graph engine, hybrid retrieval fusion, and citation-validated LLM explanations.

---

## Key Features

- **Multi-Language Static Analysis**: AST-based symbol and relationship extraction for **Go**, **TypeScript / JavaScript**, and **Java**.
- **In-Memory Graph Engine**: Sub-millisecond graph traversals including:
  - Caller & Callee resolution (`GetCallers`, `GetCallees`)
  - Module dependency analysis (`GetModuleDependencies`)
  - Transitive impact analysis (`GetTransitiveDependents`)
  - Inheritance hierarchy & bounded neighborhood queries
- **Static Flow Tracing**: Static execution flow simulation (`TraceStaticFlow`) across call graphs without requiring runtime execution.
- **Grounded AI Explanations**: LLM-powered codebase explanations with deterministic citation validation (`[E1]`, `[E2]`) to eliminate hallucinated claims.
- **Guided Reverse-Engineering**: Step-by-step architecture investigation tours leading developers from overview to detailed component breakdown.
- **Hybrid Evidence Retrieval**: Reciprocal Rank Fusion (RRF) combining lexical, semantic vector, and graph retrieval into token-budgeted evidence packages.
- **Resilient Persistence**: Embedded SQLite storage configured with Write-Ahead Logging (WAL) mode and 5000ms busy timeout for safe concurrent reads during graph re-indexing.
- **Multi-Repository Isolation**: Complete tenant and security isolation ensuring symbols, graphs, and evidence remain strictly scoped per repository.

---

## Architecture Overview

```text
 ┌────────────────┐     ┌───────────────────┐     ┌───────────────────┐
 │ Source Files   ├────>│ Ingestion Scanner ├────>│ Language Pipeline │
 └────────────────┘     └───────────────────┘     └─────────┬─────────┘
                                                            │ AST Analysis
                                                            ▼
 ┌────────────────┐     ┌───────────────────┐     ┌───────────────────┐
 │  REST API /    │<────┤ Graph Engine      │<────┤ SQLite WAL        │
 │  Chi Router    │     │ (In-Memory Maps)  │     │ Storage           │
 └───────┬────────┘     └─────────▲─────────┘     └─────────┬─────────┘
         │                        │                         │
         ▼                        │                         ▼
 ┌────────────────┐               │               ┌───────────────────┐
 │ Grounded LLM   ├───────────────┴───────────────┤ Hybrid Retrieval  │
 │ Service        │  Evidence & Citation Check    │ & RRF Fusion      │
 └────────────────┘                               └───────────────────┘
```

---

## API Reference

CodeGraph exposes a comprehensive RESTful API via Chi router:

### Repositories & Ingestion
- `POST /api/repositories`: Register a new local or Git repository.
- `GET /api/repositories`: List all registered repositories.
- `GET /api/repositories/{id}`: Get repository details and status.
- `POST /api/repositories/{id}/index`: Start an asynchronous indexing job.
- `GET /api/repositories/{id}/files`: Retrieve file manifest list.
- `GET /api/repositories/{id}/file?path={relPath}`: Fetch source file content with line counts and language detection (with path traversal & secret file protection).

### Code Intelligence & Graph Queries
- `GET /api/repositories/{id}/symbols`: List extracted code symbols.
- `GET /api/repositories/{id}/relationships`: List extracted code relationships.
- `GET /api/repositories/{id}/graph`: Query graph overview or bounded neighborhood.
- `GET /api/repositories/{id}/graph/callers?symbol_id={id}`: Get caller call-sites for a target symbol.
- `GET /api/repositories/{id}/graph/callees?symbol_id={id}`: Get callee call-sites for a target symbol.
- `GET /api/repositories/{id}/graph/dependencies?file_id={id}`: Get module dependencies for a file.
- `GET /api/repositories/{id}/graph/impact?file_id={id}`: Get transitive impact analysis (files affected by changes).
- `GET /api/repositories/{id}/flow?root={symbol_id}`: Trace static execution flow.

### Grounded AI & Guided Reverse-Engineering
- `POST /api/repositories/{id}/explain`: Generate grounded codebase explanation with citation validation.
- `POST /api/repositories/{id}/guide`: Progress through guided architecture investigation tour (`START` / `NEXT` / `RESET`).

---

## Getting Started

### Prerequisites

- **Go**: Version `1.23.0` or higher
- **CGO**: Not required (`CGO_ENABLED=0` fully supported via pure Go `modernc.org/sqlite`)

### Installation & Build

Clone the repository and build the binary:

```bash
git clone https://github.com/varunsai20-a11y/codegraph.git
cd codegraph

# Build the executable
go build ./cmd/codegraph
```

### Running the Server

```bash
./codegraph
```

The server starts on port `8080` by default (configurable via environment variables or configuration file).

---

## Testing & Quality Assurance

### Run Unit & Integration Tests

```bash
# Run full regression suite across all packages (uncached)
go test -count=1 ./...

# Run static analysis
go vet ./...
```

### Run Micro-Benchmarks & Performance Stress Tests

```bash
# Run native Go micro-benchmarks (latency & memory allocations)
go test -bench="Benchmark" -benchmem ./internal/...

# Run scenario stress tests
go test -v ./tests -run "TestLargeGraphStress|TestExternalRepoBenchmarkChi|TestE2E_HTTPConcurrencyAndReindexingResilience"
```

---

## License

This project is licensed under the [MIT License](LICENSE).
