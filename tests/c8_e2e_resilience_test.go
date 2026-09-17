package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"codegraph/internal/api"
	"codegraph/internal/config"
	"codegraph/internal/guide"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/storage"
)

func TestE2E_HTTPConcurrencyAndReindexingResilience(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "resilience.db")
	wsRoot := filepath.Join(tmpDir, "workspaces")

	cfg := &config.Config{
		Port:               8080,
		MaxFileSize:        2 * 1024 * 1024,
		WorkspaceRoot:      wsRoot,
		DatabasePath:       dbPath,
		DefaultExclusions:  []string{".git"},
		SupportedLanguages: []string{"Go", "TypeScript", "Java"},
	}

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	wsMgr, err := repository.NewWorkspaceManager(wsRoot)
	if err != nil {
		t.Fatalf("failed to init workspace manager: %v", err)
	}

	indexer := ingestion.NewIndexer(cfg, store, wsMgr)
	server := api.NewServer(cfg, store, wsMgr, indexer)

	fixturePath, err := filepath.Abs(filepath.Join("fixtures", "sample-repository"))
	if err != nil {
		t.Fatalf("failed to resolve fixture path: %v", err)
	}

	// Register & Index Repository
	regBody, _ := json.Marshal(api.RegisterRepoRequest{
		Name:       "Resilience E2E Repo",
		SourceType: models.SourceTypeLocal,
		LocalPath:  fixturePath,
	})
	req := httptest.NewRequest("POST", "/api/repositories", bytes.NewReader(regBody))
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d; want 201", rec.Code)
	}

	var repo models.Repository
	_ = json.Unmarshal(rec.Body.Bytes(), &repo)

	ctx := context.Background()
	job := &models.IndexJob{ID: "job-resilience-1", RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
	_ = store.CreateIndexJob(ctx, job)
	if err := indexer.RunIndex(ctx, job, &repo); err != nil {
		t.Fatalf("indexing failed: %v", err)
	}

	mockProv := llm.NewMockLLMProvider("Main application entry point [E1].", nil)
	lexRetriever := retrieval.NewLexicalRetriever(store)
	composer := retrieval.NewDefaultEvidenceComposer(lexRetriever, nil, store)
	expSvc := llm.NewGroundedExplanationService(mockProv, nil, composer, nil)
	server.SetExplanationService(expSvc)

	engine, _ := indexer.GetGraphEngine(repo.ID)
	sumGen := guide.NewDefaultArchitectureSummaryGenerator(store, engine)
	orchestrator := guide.NewDefaultGuideOrchestrator(store, sumGen, composer, expSvc)
	server.SetGuideOrchestrator(orchestrator)

	symbols, _ := store.GetSymbolsForRepository(ctx, repo.ID)
	var rootSym string
	if len(symbols) > 0 {
		rootSym = symbols[0].ID
	}

	var wg sync.WaitGroup
	stopChan := make(chan struct{})
	errChan := make(chan string, 1000)
	durationsChan := make(chan time.Duration, 10000)

	// Atomic counters per endpoint
	var graphReqCount atomic.Int64
	var symbolsReqCount atomic.Int64
	var flowReqCount atomic.Int64
	var explainReqCount atomic.Int64
	var guideReqCount atomic.Int64

	// Signal channel to ensure workers have actively started before re-indexing triggers
	workersStarted := make(chan struct{})
	var readyOnce sync.Once

	// Launch 15 concurrent worker goroutines hitting HTTP endpoints
	for workerID := 0; workerID < 15; workerID++ {
		wg.Add(1)
		go func(wID int) {
			defer wg.Done()
			for {
				select {
				case <-stopChan:
					return
				default:
					readyOnce.Do(func() { close(workersStarted) })

					targetEndpoint := wID % 5
					var httpReq *http.Request
					var httpRec *httptest.ResponseRecorder

					reqStart := time.Now()
					switch targetEndpoint {
					case 0:
						httpReq = httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/graph", nil)
						httpRec = httptest.NewRecorder()
						server.Router().ServeHTTP(httpRec, httpReq)
						graphReqCount.Add(1)

						if httpRec.Code != http.StatusOK {
							errChan <- fmt.Sprintf("GET /graph returned status %d; want 200", httpRec.Code)
						} else {
							var res map[string]interface{}
							if err := json.Unmarshal(httpRec.Body.Bytes(), &res); err != nil {
								errChan <- fmt.Sprintf("GET /graph invalid JSON: %v", err)
							} else if res["repository_id"] != repo.ID {
								errChan <- fmt.Sprintf("GET /graph repo_id mismatch: got %v, want %s", res["repository_id"], repo.ID)
							} else if res["nodes"] == nil {
								errChan <- "GET /graph returned nil nodes list"
							}
						}

					case 1:
						httpReq = httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/symbols", nil)
						httpRec = httptest.NewRecorder()
						server.Router().ServeHTTP(httpRec, httpReq)
						symbolsReqCount.Add(1)

						if httpRec.Code != http.StatusOK {
							errChan <- fmt.Sprintf("GET /symbols returned status %d; want 200", httpRec.Code)
						} else {
							var symList []*models.Symbol
							if err := json.Unmarshal(httpRec.Body.Bytes(), &symList); err != nil {
								errChan <- fmt.Sprintf("GET /symbols invalid JSON: %v", err)
							} else {
								for _, s := range symList {
									if s.RepositoryID != repo.ID {
										errChan <- fmt.Sprintf("GET /symbols symbol repo_id mismatch: got %s", s.RepositoryID)
										break
									}
								}
							}
						}

					case 2:
						httpReq = httptest.NewRequest("GET", "/api/repositories/"+repo.ID+"/flow?root="+rootSym, nil)
						httpRec = httptest.NewRecorder()
						server.Router().ServeHTTP(httpRec, httpReq)
						flowReqCount.Add(1)

						if httpRec.Code != http.StatusOK {
							errChan <- fmt.Sprintf("GET /flow returned status %d; want 200", httpRec.Code)
						} else {
							var flowRes models.StaticFlowResult
							if err := json.Unmarshal(httpRec.Body.Bytes(), &flowRes); err != nil {
								errChan <- fmt.Sprintf("GET /flow invalid JSON: %v", err)
							} else if flowRes.RepositoryID != repo.ID {
								errChan <- fmt.Sprintf("GET /flow repo_id mismatch: got %s", flowRes.RepositoryID)
							}
						}

					case 3:
						body, _ := json.Marshal(map[string]interface{}{"query": "Main", "symbol_id": rootSym})
						httpReq = httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/explain", bytes.NewReader(body))
						httpReq.Header.Set("Content-Type", "application/json")
						httpRec = httptest.NewRecorder()
						server.Router().ServeHTTP(httpRec, httpReq)
						explainReqCount.Add(1)

						if httpRec.Code != http.StatusOK {
							errChan <- fmt.Sprintf("POST /explain returned status %d; want 200", httpRec.Code)
						} else {
							var expRes models.ExplanationResponse
							if err := json.Unmarshal(httpRec.Body.Bytes(), &expRes); err != nil {
								errChan <- fmt.Sprintf("POST /explain invalid JSON: %v", err)
							} else if expRes.RepositoryID != repo.ID {
								errChan <- fmt.Sprintf("POST /explain repo_id mismatch: got %s", expRes.RepositoryID)
							}
						}

					case 4:
						body, _ := json.Marshal(map[string]interface{}{"action": "START"})
						httpReq = httptest.NewRequest("POST", "/api/repositories/"+repo.ID+"/guide", bytes.NewReader(body))
						httpReq.Header.Set("Content-Type", "application/json")
						httpRec = httptest.NewRecorder()
						server.Router().ServeHTTP(httpRec, httpReq)
						guideReqCount.Add(1)

						if httpRec.Code != http.StatusOK {
							errChan <- fmt.Sprintf("POST /guide returned status %d; want 200", httpRec.Code)
						} else {
							var invRes models.Investigation
							if err := json.Unmarshal(httpRec.Body.Bytes(), &invRes); err != nil {
								errChan <- fmt.Sprintf("POST /guide invalid JSON: %v", err)
							} else if invRes.RepositoryID != repo.ID {
								errChan <- fmt.Sprintf("POST /guide repo_id mismatch: got %s", invRes.RepositoryID)
							} else if len(invRes.Steps) == 0 {
								errChan <- "POST /guide returned empty steps"
							}
						}
					}
					durationsChan <- time.Since(reqStart)

					time.Sleep(1 * time.Millisecond)
				}
			}
		}(workerID)
	}

	// Wait for workers to begin active requests before starting measurement window & re-indexing cycles
	<-workersStarted
	measurementStart := time.Now()

	// Writer goroutine invalidating and re-indexing graph concurrently while workers send traffic
	for cycle := 0; cycle < 3; cycle++ {
		reIndexStart := time.Now()
		indexer.InvalidateEngine(repo.ID)
		reJob := &models.IndexJob{ID: fmt.Sprintf("job-resilience-reindex-%d", cycle), RepositoryID: repo.ID, Status: models.JobStatusPending, StartedAt: time.Now()}
		_ = store.CreateIndexJob(ctx, reJob)
		if err := indexer.RunIndex(ctx, reJob, &repo); err != nil {
			t.Fatalf("re-indexing cycle %d failed: %v", cycle, err)
		}
		t.Logf("Re-indexing cycle %d completed in %v while active HTTP requests were processing", cycle+1, time.Since(reIndexStart))
		time.Sleep(10 * time.Millisecond)
	}

	measurementDuration := time.Since(measurementStart)

	close(stopChan)
	wg.Wait()
	close(errChan)
	close(durationsChan)

	var errList []string
	for e := range errChan {
		errList = append(errList, e)
	}

	var durations []time.Duration
	for d := range durationsChan {
		durations = append(durations, d)
	}

	totalGraph := graphReqCount.Load()
	totalSymbols := symbolsReqCount.Load()
	totalFlow := flowReqCount.Load()
	totalExplain := explainReqCount.Load()
	totalGuide := guideReqCount.Load()
	totalAll := totalGraph + totalSymbols + totalFlow + totalExplain + totalGuide

	var throughput float64
	if measurementDuration.Seconds() > 0 {
		throughput = float64(totalAll) / measurementDuration.Seconds()
	}

	var p95Latency time.Duration
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		p95Idx := int(float64(len(durations)) * 0.95)
		if p95Idx >= len(durations) {
			p95Idx = len(durations) - 1
		}
		if p95Idx < 0 {
			p95Idx = 0
		}
		p95Latency = durations[p95Idx]
	}

	t.Logf("Measurement Window Duration: %v", measurementDuration)
	t.Logf("Concurrency & Resilience Test Metrics -> Total Requests: %d | Errors: %d | Throughput: %.2f req/sec | p95 Latency: %v",
		totalAll, len(errList), throughput, p95Latency)
	t.Logf("Endpoint breakdown -> GET /graph: %d, GET /symbols: %d, GET /flow: %d, POST /explain: %d, POST /guide: %d",
		totalGraph, totalSymbols, totalFlow, totalExplain, totalGuide)

	if totalGraph == 0 || totalSymbols == 0 || totalFlow == 0 || totalExplain == 0 || totalGuide == 0 {
		t.Errorf("expected non-zero requests for all endpoints, got: graph=%d, symbols=%d, flow=%d, explain=%d, guide=%d",
			totalGraph, totalSymbols, totalFlow, totalExplain, totalGuide)
	}

	if len(errList) > 0 {
		t.Fatalf("encountered %d HTTP errors during resilience test. First error: %v", len(errList), errList[0])
	}
}
