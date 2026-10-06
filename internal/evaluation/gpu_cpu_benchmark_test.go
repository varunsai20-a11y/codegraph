package evaluation_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codegraph/internal/config"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

type BenchmarkMetric struct {
	QueryIndex       int
	Question         string
	Mode             string // "CPU" or "GPU"
	Latency          time.Duration
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	TokensPerSec     float64
	Processor        string // from ollama ps
	VRAMUsage        string // from nvidia-smi
	CitationStatus   string
	ValidCitations   int
}

func TestBenchmarkCPUvsGPU(t *testing.T) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil || resp.StatusCode != 200 {
		t.Skip("Local Ollama endpoint http://localhost:11434 is not running")
		return
	}
	resp.Body.Close()

	worldMonitorPath := `c:\Users\varun\codegraph\_workspaces\112684e1-3c95-43cd-aed2-163a4df0de1b`
	if _, err := os.Stat(worldMonitorPath); err != nil {
		t.Fatalf("WorldMonitor workspace path missing: %s", worldMonitorPath)
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "gpu_bench.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init SQLite storage: %v", err)
	}
	defer store.Close()

	wsMgr, err := repository.NewWorkspaceManager(filepath.Join(tmpDir, "workspaces"))
	if err != nil {
		t.Fatalf("failed to init workspace manager: %v", err)
	}

	cfg := &config.Config{
		DefaultExclusions: []string{".git", "node_modules", "dist", "build", "coverage", ".cache", ".tmp", "vendor"},
		MaxFileSize:       10 * 1024 * 1024,
	}

	idx := ingestion.NewIndexer(cfg, store, wsMgr)
	ctx := context.Background()

	wmRepo := &models.Repository{
		ID:         "worldmonitor",
		Name:       "WorldMonitor",
		SourceType: models.SourceTypeLocal,
		LocalPath:  worldMonitorPath,
		Status:     models.RepoStatusRegistered,
	}
	if err := store.CreateRepository(ctx, wmRepo); err != nil {
		t.Fatalf("failed to register WorldMonitor: %v", err)
	}
	wmJob := &models.IndexJob{ID: "job-wm-bench", RepositoryID: wmRepo.ID, Status: models.JobStatusPending}
	if err := store.CreateIndexJob(ctx, wmJob); err != nil {
		t.Fatalf("failed to create job: %v", err)
	}
	if err := idx.RunIndex(ctx, wmJob, wmRepo); err != nil {
		t.Fatalf("RunIndex WorldMonitor failed: %v", err)
	}

	lexRetriever := retrieval.NewLexicalRetriever(store, wsMgr)
	graphRetriever := retrieval.NewGraphRetriever(store)
	hybridEngine := retrieval.NewHybridRetrieverEngine(nil, lexRetriever, nil, graphRetriever, retrieval.DefaultHybridRetrievalConfig())
	hybridAdapter := retrieval.NewHybridRetrieverAdapter(hybridEngine)
	composer := retrieval.NewDefaultEvidenceComposer(hybridAdapter, nil, store)

	promptBuilder := llm.NewGroundedPromptBuilder()
	validator := llm.NewCitationValidator()
	scope, _ := models.NewRepositoryScope(wmRepo.ID)

	targetQueries := []string{
		"What does this project do? Explain it to me like a beginner.",
		"Explain the architecture of this project.",
		"Trace the request flow.",
	}

	// Setup GPU Provider (default payload, auto GPU offloading)
	gpuProv, err := llm.NewHTTPLLMProvider(llm.LLMConfig{
		Provider: "ollama",
		Model:    "qwen2.5:3b",
		Endpoint: "http://localhost:11434/api/chat",
		Timeout:  120 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("failed to init GPU provider: %v", err)
	}
	gpuSvc := llm.NewGroundedExplanationService(gpuProv, validator, composer, promptBuilder)

	fmt.Printf("\n==================================================\n")
	fmt.Printf("=== RUNNING BENCHMARK: GPU MODE (NVIDIA RTX 3050) ===\n")
	fmt.Printf("==================================================\n")

	var gpuMetrics []BenchmarkMetric

	for idx, q := range targetQueries {
		start := time.Now()
		expReq, _ := models.NewExplanationRequest(scope, q)
		resp, err := gpuSvc.ExplainRequest(ctx, expReq)
		dur := time.Since(start)

		if err != nil {
			t.Errorf("GPU Query %d '%s' failed: %v", idx+1, q, err)
			continue
		}

		psOut := getOllamaPS()
		vram := getNvidiaSMIMemory()
		tps := 0.0
		if dur.Seconds() > 0 && resp.CompletionTokens > 0 {
			tps = float64(resp.CompletionTokens) / dur.Seconds()
		}

		bm := BenchmarkMetric{
			QueryIndex:       idx + 1,
			Question:         q,
			Mode:             "GPU",
			Latency:          dur,
			PromptTokens:     resp.PromptTokens,
			CompletionTokens: resp.CompletionTokens,
			TotalTokens:      resp.TotalTokens,
			TokensPerSec:     tps,
			Processor:        psOut,
			VRAMUsage:        vram,
			CitationStatus:   string(resp.Grounding.Status),
			ValidCitations:   resp.Grounding.CitedEvidenceCount,
		}
		gpuMetrics = append(gpuMetrics, bm)

		fmt.Printf("\n[GPU] Q%d: \"%s\"\n", idx+1, q)
		fmt.Printf("Latency: %v | Prompt Tokens: %d | Completion Tokens: %d | Speed: %.2f tok/s\n",
			dur, resp.PromptTokens, resp.CompletionTokens, tps)
		fmt.Printf("Ollama Processor: %s | NVIDIA VRAM: %s\n", psOut, vram)
		fmt.Printf("Citations Valid: %d | Status: %s\n", resp.Grounding.CitedEvidenceCount, resp.Grounding.Status)
		fmt.Printf("--- ANSWER PREVIEW ---\n%s\n--- END PREVIEW ---\n", truncatePreview(resp.Answer, 300))
	}

	fmt.Printf("\n==================================================\n")
	fmt.Printf("=== GPU BENCHMARK COMPLETE ===\n")
	fmt.Printf("==================================================\n")
}

func getOllamaPS() string {
	cmd := exec.Command("ollama", "ps")
	out, err := cmd.Output()
	if err != nil {
		return "N/A"
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) > 1 && strings.TrimSpace(lines[1]) != "" {
		return strings.TrimSpace(lines[1])
	}
	return "Active"
}

func getNvidiaSMIMemory() string {
	cmd := exec.Command("nvidia-smi", "--query-gpu=memory.used,memory.free,memory.total", "--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err != nil {
		return "N/A"
	}
	return strings.TrimSpace(string(out)) + " MiB (used, free, total)"
}

func truncatePreview(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
