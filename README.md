# CodeGraph

> **AI-Powered Codebase Reverse Engineering & Grounded Repository Intelligence**

Understand unfamiliar codebases through AST analysis, graph relationships, hybrid RAG retrieval, and grounded Gemini explanations.

CodeGraph is an AI-assisted developer tool designed to help engineers understand unfamiliar repositories without manually reading thousands of lines of code.

Instead of treating a repository as plain text and asking an LLM to guess how it works, CodeGraph builds a structured representation of the codebase using AST analysis, symbol extraction, cross-file relationships, graph traversal, hybrid retrieval, and evidence validation before generating an explanation.

The result is a system that can answer questions such as:
- How does this application start?
- Where does execution begin?
- How does this module interact with the rest of the system?
- What files depend on this component?
- What is the execution flow between these functions?
- What are the important architectural components of this repository?

---

## 🚀 Why CodeGraph?

Understanding an unfamiliar codebase is one of the first challenges faced by developers joining a new project.

Traditional approaches generally require:
```text
Open repository ──> Read README ──> Search files ──> Follow imports ──> Follow function calls ──> Build mental model
```

CodeGraph automates much of this process:

```text
┌─────────────────────┐
│     Repository      │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│   AST / Indexing    │
│ Symbols + Relations │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│     Code Graph      │
│  Calls / Imports /  │
│  Cross-file Edges   │
└──────────┬──────────┘
           │
 ┌─────────┴───────────┬──────────────────┐
 │                     │                  │
 ▼                     ▼                  ▼
Lexical Search    Vector Retrieval  Graph Retrieval
 └─────────┬───────────┴──────────────────┘
           │
           ▼
┌─────────────────────┐
│  RRF Hybrid Fusion  │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│Evidence Sufficiency │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Grounded Context   │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│  Gemini Generation  │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│   Evidence-backed   │
│     Explanation     │
└─────────────────────┘
```

The important distinction is that the LLM is not responsible for discovering the entire architecture by itself. CodeGraph first retrieves and validates repository evidence, then uses Gemini to synthesize that evidence into a human-readable explanation.

---

## ✨ Core Features

### 🧠 AI-Powered Repository Understanding
Ask natural-language questions about a repository and receive explanations grounded in actual source-code evidence.

**Example Query:**
> *"Explain how crew.py works, what files it depends on, and describe its execution flow."*

CodeGraph processes the request through:
`Intent Classification` → `Retrieval` → `Evidence Fusion` → `Sufficiency Evaluation` → `Context Assembly` → `Gemini` → `Citation Validation`

### 🔍 Automated Reverse Engineering
CodeGraph includes a dedicated 5-step Reverse Engineering Guide:

- **Step 1 — System Purpose & Architecture Scope**: Identifies repository size, source files, symbols, AST relationships, primary entrypoints, architectural modules, and external boundaries.
- **Step 2 — Major Modules & Responsibilities**: Groups the repository into meaningful architectural modules and determines how responsibilities are distributed.
- **Step 3 — Important Execution Components**: Uses structural signals (call counts, entrypoint proximity, module centrality) to identify key components (e.g. `crew.py:research_crew`).
- **Step 4 — Execution & Static Call Flow**: Reconstructs cross-file execution paths (e.g., `run.py:main` → `crew.py:research_crew` → `agents.py:research_agent`), explicitly scoping static analysis boundaries (*Statically Established*, *Inferred*, *Unavailable Runtime*).
- **Step 5 — Architectural Synthesis**: Combines previous discoveries into a cohesive architectural synthesis of the codebase.

### 🕸️ Code Graph
CodeGraph builds a structured in-memory graph of the repository rather than relying exclusively on text search. The graph captures:
- `Function` → **CALLS** → `Function`
- `File` → **IMPORTS** → `File`
- `Symbol` → **REFERENCES** → `Symbol`
- `Module` → **CONTAINS** → `Symbol`

### 🔎 Hybrid RAG Retrieval
Combines multiple retrieval techniques using **Reciprocal Rank Fusion (RRF)**:
- **Lexical Retrieval**: Finds exact terminology, symbols, filenames, and source code.
- **Vector Retrieval**: Uses semantic embeddings (`gemini-embedding-001`, 768 dimensions).
- **Graph Retrieval**: Multi-hop structural graph traversal.

```text
Lexical  │
Vector   ├───> RRF Fusion ───> Evidence Set
Graph    │
```

### 🛡️ Evidence-Grounded AI
Before Gemini generates an explanation, CodeGraph evaluates whether enough repository evidence was retrieved. Successful responses expose clickable citations (e.g., `[E1] crew.py:research_crew`, `[E2] agents.py:research_agent`) that jump directly to source code in the IDE.

### 🤖 Real Gemini Integration
CodeGraph uses a real Gemini HTTP REST provider:
- **Generation Model**: `gemini-3.8-flash`
- **Embedding Model**: `gemini-embedding-001`
- **Honest Status Reporting**: `GEMINI GROUNDED`, `DETERMINISTIC FALLBACK`, `PROVIDER ERROR`, `RETRIEVAL INSUFFICIENT`.

---

## 🏗️ Architecture

```text
┌─────────────────────────────────────────────────────────┐
│                    Next.js Frontend                     │
│                                                         │
│  Explorer │ Graph │ Flow │ Reverse Engineering │ AI UI  │
└───────────────────────────┬─────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────┐
│                    Go Backend / API                     │
│                                                         │
│  Repository API │ Guide │ Retrieval │ LLM │ Storage     │
└──────────┬──────────────┬───────────────┬──────────────┘
           │              │               │
           ▼              ▼               ▼
     AST Analysis    Code Graph    SQLite Storage
           │              │               │
           └───────┬──────┘               ▼
                   ▼               Hybrid Retrieval
            RRF Fusion            ┌───────┼────────┐
                   │              ▼       ▼        ▼
                   ▼           Lexical Vector Graph
         Evidence Sufficiency     └───────┼────────┘
                   │                      │
                   ▼                      ▼
           Context Assembly ──────> Gemini API
```

---

## 🛠️ Technology Stack

| Area | Technology |
| :--- | :--- |
| **Backend** | Go (Chi Router, Pure-Go SQLite) |
| **Frontend** | Next.js / React / TypeScript / TailwindCSS |
| **Code Parsing** | Tree-sitter / AST Analysis |
| **Graph Analysis** | Custom In-Memory Code Relationship Graph |
| **Database** | SQLite (WAL Mode) |
| **Vector Retrieval** | Gemini Embeddings (`gemini-embedding-001`, 768-dim) |
| **LLM Provider** | Gemini (`gemini-3.8-flash`) |
| **Retrieval Fusion** | Reciprocal Rank Fusion (RRF) |
| **Testing** | Go Test Suite / Frontend Production Build |

---

## 🔐 Static Analysis Honesty

CodeGraph intentionally distinguishes between what can and cannot be established statically:
- **Statically Established**: Direct AST and graph relationships (e.g. `run.py:main` → `crew.py:research_crew`).
- **Inferred**: Architectural interpretations derived from static evidence.
- **Unavailable Runtime**: Behavior requiring runtime execution (dynamic reflection, dynamic imports, LLM outputs).

---

## 🚀 Getting Started

### Prerequisites
- **Go**: Version `1.23` or higher
- **Node.js**: Version `18` or higher
- **Gemini API Key**: From Google AI Studio

### Installation & Setup

1. **Clone the Repository**:
   ```bash
   git clone https://github.com/varunsai20-a11y/codegraph.git
   cd codegraph
   ```

2. **Configure Environment**:
   Create a `.env` file in the project root:
   ```env
   PORT=8080
   GEMINI_API_KEY=your_gemini_api_key_here
   ```

3. **Run Backend**:
   ```bash
   go run ./cmd/codegraph
   ```

4. **Run Frontend**:
   ```bash
   cd frontend
   npm install
   npm run dev
   ```
   Open `http://localhost:3000` in your browser.

---

## 🧪 Testing & Validation

```bash
# Run backend test suite across all packages
go test ./...

# Run frontend production build
cd frontend
npm run build
```

---

## 👨‍💻 Author

**Varun Sai**  
*Artificial Intelligence & Data Engineering*  
[GitHub Profile](https://github.com/varunsai20-a11y) • [CodeGraph Repository](https://github.com/varunsai20-a11y/codegraph)

⭐ **If you find CodeGraph interesting, give the repository a star!**
