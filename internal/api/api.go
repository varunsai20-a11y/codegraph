package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"codegraph/internal/config"
	"codegraph/internal/graph"
	"codegraph/internal/guide"
	"codegraph/internal/ingestion"
	"codegraph/internal/llm"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/retrieval"
	"codegraph/internal/security"
	"codegraph/internal/storage"
	"codegraph/internal/vector"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type Server struct {
	cfg                *config.Config
	store              storage.Storage
	wsMgr              *repository.WorkspaceManager
	indexer            *ingestion.Indexer
	explanationService llm.ExplanationService
	guideOrchestrator  guide.GuideOrchestrator
	router             chi.Router
}

func (s *Server) SetExplanationService(svc llm.ExplanationService) {
	s.explanationService = svc
}

func (s *Server) SetGuideOrchestrator(orch guide.GuideOrchestrator) {
	s.guideOrchestrator = orch
}

func NewServer(cfg *config.Config, store storage.Storage, wsMgr *repository.WorkspaceManager, indexer *ingestion.Indexer) *Server {
	r := chi.NewRouter()
	s := &Server{
		cfg:     cfg,
		store:   store,
		wsMgr:   wsMgr,
		indexer: indexer,
		router:  r,
	}

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(s.corsMiddleware)

	s.routes()
	return s
}

func (s *Server) Router() chi.Router {
	return s.router
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := false
			if s.cfg != nil {
				for _, o := range s.cfg.AllowedOrigins {
					if o == origin {
						allowed = true
						break
					}
				}
			}
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token, X-Requested-With")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			} else {
				if r.Method == http.MethodOptions {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "CORS policy violation: unauthorized origin"})
					return
				}
			}
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	s.router.Route("/api", func(r chi.Router) {
		r.Post("/repositories", s.handleRegisterRepository)
		r.Get("/repositories", s.handleListRepositories)
		r.Get("/repositories/{id}", s.handleGetRepository)
		r.Post("/repositories/{id}/index", s.handleStartIndexJob)
		r.Get("/repositories/{id}/files", s.handleGetFileManifest)
		r.Get("/repositories/{id}/file", s.handleGetSourceFile)
		r.Get("/repositories/{id}/symbols", s.handleGetSymbols)
		r.Get("/repositories/{id}/relationships", s.handleGetRelationships)
		r.Get("/repositories/{id}/analysis", s.handleGetAnalysisSummary)
		r.Get("/repositories/{id}/graph", s.handleGetGraph)
		r.Get("/repositories/{id}/graph/callers", s.handleGetGraphCallers)
		r.Get("/repositories/{id}/graph/callees", s.handleGetGraphCallees)
		r.Get("/repositories/{id}/graph/dependencies", s.handleGetGraphDependencies)
		r.Get("/repositories/{id}/graph/impact", s.handleGetGraphImpact)
		r.Get("/repositories/{id}/graph/hierarchy", s.handleGetGraphHierarchy)
		r.Get("/repositories/{id}/architecture-flow", s.handleGetArchitectureFlow)
		r.Get("/repositories/{id}/flow", s.handleGetFlow)
		r.Post("/repositories/{id}/explain", s.handleExplainRepository)
		r.Post("/repositories/{id}/guide", s.handleGetGuide)
		r.Get("/index-jobs/{id}", s.handleGetIndexJob)
	})
}

type FileContentResponse struct {
	RepositoryID string `json:"repository_id"`
	RelativePath string `json:"relative_path"`
	TotalLines   int    `json:"total_lines"`
	Content      string `json:"content"`
	Language     string `json:"language"`
}

func (s *Server) handleGetSourceFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	relPath := r.URL.Query().Get("path")

	if id == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "repository_id is required"})
		return
	}

	if relPath == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "file path query parameter is required"})
		return
	}

	// 1. Sanitize & reject path traversal components upfront
	if strings.Contains(relPath, "..") || filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "/") || strings.HasPrefix(relPath, "\\") {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "security violation: path traversal or absolute path detected"})
		return
	}

	// 2. Validate repository exists
	repo, err := s.store.GetRepository(r.Context(), id)
	if err != nil || repo == nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	// 3. Prepare workspace path
	workspacePath, err := s.wsMgr.PrepareWorkspace(r.Context(), repo)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository workspace unavailable"})
		return
	}

	// 4. Security checks: Secret file protection, path normalization, and workspace containment
	if security.IsSecretFile(relPath) {
		s.respondJSON(w, http.StatusForbidden, map[string]string{"error": "security violation: access to secret file is forbidden"})
		return
	}

	normPath, err := security.NormalizeRelativePath(workspacePath, relPath)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "security violation: invalid relative path"})
		return
	}

	targetPath := filepath.Join(workspacePath, filepath.FromSlash(normPath))

	if err := security.ValidatePathWithinWorkspace(workspacePath, targetPath); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "security violation: path outside workspace boundary"})
		return
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to stat file"})
		return
	}

	if info.IsDir() {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	isSym, realPath, err := security.CheckSymlinkSafety(workspacePath, targetPath)
	if err != nil || (isSym && security.ValidatePathWithinWorkspace(workspacePath, realPath) != nil) {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "security violation: symlink escape detected"})
		return
	}

	contentBytes, err := os.ReadFile(targetPath)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read file content"})
		return
	}

	contentStr := string(contentBytes)
	totalLines := 0
	if len(contentStr) > 0 {
		totalLines = strings.Count(contentStr, "\n")
		if !strings.HasSuffix(contentStr, "\n") {
			totalLines++
		}
	}

	lang := detectLanguageFromPath(normPath)

	s.respondJSON(w, http.StatusOK, FileContentResponse{
		RepositoryID: id,
		RelativePath: normPath,
		TotalLines:   totalLines,
		Content:      contentStr,
		Language:     lang,
	})
}

func detectLanguageFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "GO"
	case ".ts", ".tsx":
		return "TYPESCRIPT"
	case ".js", ".jsx":
		return "JAVASCRIPT"
	case ".py":
		return "PYTHON"
	case ".java":
		return "JAVA"
	case ".json":
		return "JSON"
	case ".md":
		return "MARKDOWN"
	default:
		return "UNKNOWN"
	}
}

type RegisterRepoRequest struct {
	Name       string            `json:"name"`
	SourceType models.SourceType `json:"source_type"`
	SourceURL  string            `json:"source_url,omitempty"`
	LocalPath  string            `json:"local_path,omitempty"`
}

func (s *Server) handleRegisterRepository(w http.ResponseWriter, r *http.Request) {
	var req RegisterRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
		return
	}

	if req.SourceType != models.SourceTypeLocal && req.SourceType != models.SourceTypeGit {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "source_type must be LOCAL or GIT"})
		return
	}

	var canonicalURL string
	if req.SourceType == models.SourceTypeGit {
		var extractedName string
		var err error
		canonicalURL, extractedName, err = security.ValidateGitHubURL(req.SourceURL)
		if err != nil {
			s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if req.Name == "" {
			req.Name = extractedName
		}
	} else {
		if req.Name == "" {
			s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "repository name is required"})
			return
		}
	}

	// Single process registration mutex lock for fallback mode & race safety
	s.store.RegistrationLock().Lock()
	defer s.store.RegistrationLock().Unlock()

	// Application-level lookup for deduplication
	if canonicalURL != "" {
		if existing, err := s.store.GetRepositoryByCanonicalURL(r.Context(), canonicalURL); err == nil {
			s.respondJSON(w, http.StatusOK, existing)
			return
		}
	} else if req.SourceURL != "" {
		if existing, err := s.store.GetRepositoryBySourceURL(r.Context(), req.SourceURL); err == nil {
			s.respondJSON(w, http.StatusOK, existing)
			return
		}
	}

	repoID := uuid.New().String()
	now := time.Now()
	repo := &models.Repository{
		ID:           repoID,
		Name:         req.Name,
		SourceType:   req.SourceType,
		SourceURL:    req.SourceURL,
		CanonicalURL: canonicalURL,
		LocalPath:    req.LocalPath,
		Status:       models.RepoStatusRegistered,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.CreateRepository(r.Context(), repo); err != nil {
		// Catch UNIQUE constraint collision and resolve to existing repository
		if canonicalURL != "" {
			if existing, lookupErr := s.store.GetRepositoryByCanonicalURL(r.Context(), canonicalURL); lookupErr == nil {
				s.respondJSON(w, http.StatusOK, existing)
				return
			}
		}
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.respondJSON(w, http.StatusCreated, repo)
}

func (s *Server) handleListRepositories(w http.ResponseWriter, r *http.Request) {
	repos, err := s.store.ListRepositories(r.Context())
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if repos == nil {
		repos = []*models.Repository{}
	}
	s.respondJSON(w, http.StatusOK, repos)
}

func (s *Server) handleGetRepository(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepository(r.Context(), id)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, repo)
}

func (s *Server) handleStartIndexJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepository(r.Context(), id)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	// Race safety: Check if active job already exists
	if activeJob, err := s.store.GetActiveIndexJobForRepository(r.Context(), repo.ID); err == nil && activeJob != nil {
		s.respondJSON(w, http.StatusOK, activeJob)
		return
	}

	jobID := uuid.New().String()
	job := &models.IndexJob{
		ID:           jobID,
		RepositoryID: repo.ID,
		Status:       models.JobStatusPending,
		StartedAt:    time.Now(),
	}

	if err := s.store.CreateIndexJob(r.Context(), job); err != nil {
		// Catch UNIQUE constraint collision on active job index
		if activeJob, lookupErr := s.store.GetActiveIndexJobForRepository(r.Context(), repo.ID); lookupErr == nil && activeJob != nil {
			s.respondJSON(w, http.StatusOK, activeJob)
			return
		}
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Background indexer goroutine is launched ONLY when job creation succeeds
	go func() {
		ctx := context.Background()
		_ = s.indexer.RunIndex(ctx, job, repo)
	}()

	s.respondJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleGetFileManifest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	items, err := s.store.GetManifestForRepository(r.Context(), id)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if items == nil {
		items = []*models.FileManifestItem{}
	}
	s.respondJSON(w, http.StatusOK, items)
}

func (s *Server) handleGetSymbols(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	symbols, err := s.store.GetSymbolsForRepository(r.Context(), id)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if symbols == nil {
		symbols = []*models.Symbol{}
	}
	s.respondJSON(w, http.StatusOK, symbols)
}

func (s *Server) handleGetRelationships(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rels, err := s.store.GetRelationshipsForRepository(r.Context(), id)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rels == nil {
		rels = []*models.Relationship{}
	}
	s.respondJSON(w, http.StatusOK, rels)
}

func (s *Server) handleGetAnalysisSummary(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	symbols, _ := s.store.GetSymbolsForRepository(r.Context(), id)
	rels, _ := s.store.GetRelationshipsForRepository(r.Context(), id)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"repository_id":       id,
		"total_symbols":       len(symbols),
		"total_relationships": len(rels),
		"symbols":             symbols,
		"relationships":       rels,
	})
}

func (s *Server) handleGetGraph(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo, err := s.store.GetRepository(r.Context(), id)
	if err != nil || repo == nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	query := r.URL.Query()
	scope := strings.ToUpper(query.Get("scope"))
	if scope == "" {
		scope = "OVERVIEW"
	}

	targetNodeID := query.Get("target")

	maxHops := 1
	if hStr := query.Get("depth"); hStr != "" {
		if h, err := strconv.Atoi(hStr); err == nil && h > 0 {
			maxHops = h
		}
	}
	if maxHops > 2 {
		maxHops = 2
	}

	nodeLimit := 25
	if scope == "NEIGHBORHOOD" {
		nodeLimit = 50
	}
	if lStr := query.Get("node_limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			nodeLimit = l
		}
	}
	if lStr := query.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			nodeLimit = l
		}
	}
	if nodeLimit > 100 {
		nodeLimit = 100
	}

	edgeLimit := 50
	if scope == "NEIGHBORHOOD" {
		edgeLimit = 100
	}
	if elStr := query.Get("edge_limit"); elStr != "" {
		if el, err := strconv.Atoi(elStr); err == nil && el > 0 {
			edgeLimit = el
		}
	}
	if edgeLimit > 200 {
		edgeLimit = 200
	}

	nodeTypesMap := make(map[models.NodeKind]bool)
	if ntStr := query.Get("node_types"); ntStr != "" {
		for _, part := range strings.Split(ntStr, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				nodeTypesMap[models.NodeKind(part)] = true
			}
		}
	}

	edgeTypesMap := make(map[models.EdgeKind]bool)
	if etStr := query.Get("edge_types"); etStr != "" {
		for _, part := range strings.Split(etStr, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				edgeTypesMap[models.EdgeKind(part)] = true
			}
		}
	}

	engine, found := s.getOrLoadEngine(r.Context(), id)
	if !found {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{
			"repository_id": id,
			"node_count":    0,
			"edge_count":    0,
			"nodes":         []*models.Node{},
			"edges":         []*models.Edge{},
		})
		return
	}

	qParams := graph.QueryParams{
		StartNodeID: targetNodeID,
		MaxHops:     maxHops,
		NodeLimit:   nodeLimit,
		EdgeLimit:   edgeLimit,
		NodeTypes:   nodeTypesMap,
		EdgeTypes:   edgeTypesMap,
	}

	var nodes []*models.Node
	var edges []*models.Edge

	if scope == "NEIGHBORHOOD" && targetNodeID != "" {
		nodes, edges = engine.GetBoundedNeighborhood(qParams)
	} else {
		nodes, edges = engine.GetOverviewGraph(qParams)
	}

	if nodes == nil {
		nodes = []*models.Node{}
	}
	if edges == nil {
		edges = []*models.Edge{}
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"repository_id": id,
		"node_count":    len(nodes),
		"edge_count":    len(edges),
		"nodes":         nodes,
		"edges":         edges,
	})
}

func (s *Server) handleGetGraphCallers(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	symbolID := r.URL.Query().Get("symbol_id")
	if symbolID == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "symbol_id query parameter is required"})
		return
	}

	engine, found := s.getOrLoadEngine(r.Context(), id)
	if !found {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository graph not found"})
		return
	}

	callers := engine.GetCallers(symbolID)
	if callers == nil {
		callers = []*models.CallSite{}
	}
	s.respondJSON(w, http.StatusOK, callers)
}

func (s *Server) handleGetGraphCallees(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	symbolID := r.URL.Query().Get("symbol_id")
	if symbolID == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "symbol_id query parameter is required"})
		return
	}

	engine, found := s.getOrLoadEngine(r.Context(), id)
	if !found {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository graph not found"})
		return
	}

	callees := engine.GetCallees(symbolID)
	if callees == nil {
		callees = []*models.CallSite{}
	}
	s.respondJSON(w, http.StatusOK, callees)
}

func (s *Server) handleGetGraphDependencies(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "file_id query parameter is required"})
		return
	}

	engine, found := s.getOrLoadEngine(r.Context(), id)
	if !found {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository graph not found"})
		return
	}

	deps := engine.GetModuleDependencies(fileID)
	if deps == nil {
		deps = []*models.Node{}
	}
	s.respondJSON(w, http.StatusOK, deps)
}

func (s *Server) handleGetGraphImpact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "file_id query parameter is required"})
		return
	}

	engine, found := s.getOrLoadEngine(r.Context(), id)
	if !found {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository graph not found"})
		return
	}

	impact := engine.GetTransitiveDependents(fileID)
	s.respondJSON(w, http.StatusOK, impact)
}

func (s *Server) handleGetGraphHierarchy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	symbolID := r.URL.Query().Get("symbol_id")
	if symbolID == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "symbol_id query parameter is required"})
		return
	}

	engine, found := s.getOrLoadEngine(r.Context(), id)
	if !found {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository graph not found"})
		return
	}

	nodes, edges := engine.GetInheritanceHierarchy(symbolID)
	if nodes == nil {
		nodes = []*models.Node{}
	}
	if edges == nil {
		edges = []*models.Edge{}
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"symbol_id": symbolID,
		"nodes":     nodes,
		"edges":     edges,
	})
}

func (s *Server) handleGetArchitectureFlow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepository(r.Context(), id)
	if err != nil || repo == nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	engine, found := s.getOrLoadEngine(r.Context(), id)
	if !found {
		s.respondJSON(w, http.StatusOK, &models.ArchitectureDiagram{
			RepositoryID: id,
			Modules:      []*models.ArchitectureModule{},
			Edges:        []*models.ArchitectureEdge{},
			MermaidCode:  "graph TD\n    empty[\"No graph data indexed\"]",
			TotalModules: 0,
			TotalEdges:   0,
		})
		return
	}

	diag := engine.GetArchitectureFlow()
	s.respondJSON(w, http.StatusOK, diag)
}

func (s *Server) getOrLoadEngine(ctx context.Context, repoID string) (*graph.Engine, bool) {
	if s.indexer != nil {
		engine, found := s.indexer.GetGraphEngine(repoID)
		if found {
			return engine, true
		}
	}

	// Fallback load from SQLite
	nodes, edges, err := s.store.GetGraphForRepository(ctx, repoID)
	if err != nil || len(nodes) == 0 {
		return nil, false
	}

	engine := graph.NewEngine(repoID)
	engine.LoadGraph(nodes, edges)
	if s.indexer != nil {
		s.indexer.SetGraphEngine(repoID, engine)
	}
	return engine, true
}

func (s *Server) handleGetIndexJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := s.store.GetIndexJob(r.Context(), id)
	if err != nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, job)
}

func (s *Server) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	repo, err := s.store.GetRepository(r.Context(), id)
	if err != nil || repo == nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	query := r.URL.Query()
	rootID := query.Get("root")
	if rootID == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "root query parameter is required"})
		return
	}

	targetID := query.Get("target")

	maxDepth := 10
	if mdStr := query.Get("max_depth"); mdStr != "" {
		if md, err := strconv.Atoi(mdStr); err == nil && md > 0 {
			maxDepth = md
		}
	}

	maxNodes := 50
	if mnStr := query.Get("max_nodes"); mnStr != "" {
		if mn, err := strconv.Atoi(mnStr); err == nil && mn > 0 {
			maxNodes = mn
		}
	}

	engine, found := s.getOrLoadEngine(r.Context(), id)
	if !found {
		s.respondJSON(w, http.StatusOK, &models.StaticFlowResult{
			RepositoryID:      id,
			RootNodeID:        rootID,
			TargetNodeID:      targetID,
			FlowType:          "STATIC_CALL_GRAPH",
			MaxDepth:          maxDepth,
			MaxNodes:          maxNodes,
			TerminationReason: models.FlowReasonInvalidRoot,
			Notice:            "STATIC FLOW ≠ RUNTIME TRACE: CodeGraph derives this flow from statically analyzed CALLS relationships. It does not observe runtime execution.",
		})
		return
	}

	flowResult := engine.TraceStaticFlow(rootID, targetID, maxDepth, maxNodes)
	s.respondJSON(w, http.StatusOK, flowResult)
}

func (s *Server) respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type ExplainRequestBody struct {
	Query      string               `json:"query"`
	Question   string               `json:"question,omitempty"`
	SymbolID   string               `json:"symbol_id,omitempty"`
	NodeIDs    []string             `json:"node_ids,omitempty"`
	EdgeIDs    []string             `json:"edge_ids,omitempty"`
	RootSymbol string               `json:"root_symbol,omitempty"`
	TargetNode string               `json:"target_node,omitempty"`
	Flow       bool                 `json:"flow,omitempty"`
	History    []models.ChatMessage `json:"history,omitempty"`
	Provider   string               `json:"provider,omitempty"`
	Model      string               `json:"model,omitempty"`
}

func (s *Server) handleExplainRepository(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepository(r.Context(), id)
	if err != nil || repo == nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	var body ExplainRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
		return
	}

	query := strings.TrimSpace(body.Query)
	if query == "" {
		query = strings.TrimSpace(body.Question)
	}
	if query == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "query or question is required"})
		return
	}

	scope, err := models.NewRepositoryScope(id)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	expReq, err := models.NewExplanationRequest(scope, query)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	expReq.SymbolID = body.SymbolID
	expReq.NodeIDs = body.NodeIDs
	expReq.EdgeIDs = body.EdgeIDs
	expReq.History = body.History
	expReq.Provider = body.Provider
	expReq.Model = body.Model

	// Attach C5 static flow context if requested or root_symbol provided
	if (body.Flow || body.RootSymbol != "") && body.RootSymbol != "" {
		engine, found := s.getOrLoadEngine(r.Context(), id)
		if found {
			flowResult := engine.TraceStaticFlow(body.RootSymbol, body.TargetNode, 10, 50)
			expReq.StaticFlow = flowResult
		}
	}

	svc := s.explanationService
	if svc == nil {
		lexRetriever := retrieval.NewLexicalRetriever(s.store, s.wsMgr)
		graphRetriever := retrieval.NewGraphRetriever(s.store)

		var semRetriever retrieval.Retriever
		if sqlStore, ok := s.store.(*storage.SQLiteStorage); ok && sqlStore != nil {
			vStore, vErr := vector.NewSQLiteSemanticStore(sqlStore.DB())
			if vErr == nil {
				var embedProv vector.EmbeddingProvider
				if s.cfg != nil && s.cfg.LLM.APIKey != "" {
					embedProv = vector.NewGeminiEmbeddingProvider(s.cfg.LLM.APIKey, "")
				} else {
					embedProv = vector.NewMockEmbeddingProvider()
				}
				semRetriever = retrieval.NewSemanticRetriever(vStore, embedProv)
			}
		}

		hybridEngine := retrieval.NewHybridRetrieverEngine(nil, lexRetriever, semRetriever, graphRetriever, retrieval.DefaultHybridRetrievalConfig())
		hybridAdapter := retrieval.NewHybridRetrieverAdapter(hybridEngine)
		composer := retrieval.NewDefaultEvidenceComposer(hybridAdapter, nil, s.store)

		var provider llm.LLMProvider
		if s.cfg != nil && s.cfg.LLM.Provider != "" {
			llmCfg := llm.LLMConfig{
				Provider: s.cfg.LLM.Provider,
				Model:    s.cfg.LLM.Model,
				APIKey:   s.cfg.LLM.APIKey,
				Endpoint: s.cfg.LLM.Endpoint,
			}
			httpProv, err := llm.NewHTTPLLMProvider(llmCfg, nil)
			if err == nil {
				provider = httpProv
			}
		}
		svc = llm.NewGroundedExplanationService(provider, nil, composer, nil)
	}

	resp, err := svc.ExplainRequest(r.Context(), expReq)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		if errors.Is(err, llm.ErrProviderUnavailable) || errors.Is(err, llm.ErrProviderTimeout) ||
			errors.Is(err, llm.ErrAuthFailed) || errors.Is(err, llm.ErrModelUnavailable) ||
			errors.Is(err, llm.ErrRateLimited) || errors.Is(err, llm.ErrMalformedResponse) {
			log.Printf("[LLM] Gemini request failed: %v", err)
			s.respondJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"error":         fmt.Sprintf("CodeGraph AI Explanation Error: %v", err),
				"provider_mode": "LLM_PROVIDER_ERROR",
				"status":        "ERROR",
				"details":       err.Error(),
			})
			return
		}
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("[LLM] Gemini request succeeded (model: %s)", resp.Model)
	s.respondJSON(w, http.StatusOK, resp)
}

type GuideRequestBody struct {
	Action             string                       `json:"action,omitempty"`
	RequestedAction    string                       `json:"requested_action,omitempty"`
	MaxSteps           int                          `json:"max_steps,omitempty"`
	CompletedStepIDs   []string                     `json:"completed_step_ids,omitempty"`
	CurrentStepIndex   int                          `json:"current_step_index,omitempty"`
	StepType           models.InvestigationStepType `json:"step_type,omitempty"`
	TargetFileID       string                       `json:"target_file_id,omitempty"`
	TargetSymbolID     string                       `json:"target_symbol_id,omitempty"`
	RootSymbolID       string                       `json:"root_symbol_id,omitempty"`
	TargetNodeID       string                       `json:"target_node_id,omitempty"`
	InvestigationState *models.Investigation        `json:"investigation_state,omitempty"`
}

func (s *Server) handleGetGuide(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	repo, err := s.store.GetRepository(r.Context(), id)
	if err != nil || repo == nil {
		s.respondJSON(w, http.StatusNotFound, map[string]string{"error": "repository not found"})
		return
	}

	var body GuideRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON payload"})
		return
	}

	if body.InvestigationState != nil && body.InvestigationState.RepositoryID != "" && body.InvestigationState.RepositoryID != id {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "repository scope mismatch in investigation state"})
		return
	}

	scope, err := models.NewRepositoryScope(id)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	actionStr := body.Action
	if actionStr == "" {
		actionStr = body.RequestedAction
	}

	req, err := models.NewInvestigationRequest(scope, actionStr)
	if err != nil {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.MaxSteps > 0 {
		req.MaxSteps = body.MaxSteps
	}

	if body.CompletedStepIDs != nil && len(body.CompletedStepIDs) > 0 {
		req.CompletedStepIDs = body.CompletedStepIDs
	} else if body.InvestigationState != nil && len(body.InvestigationState.CompletedStepIDs) > 0 {
		req.CompletedStepIDs = body.InvestigationState.CompletedStepIDs
	}

	if body.CurrentStepIndex > 0 {
		req.CurrentStepIndex = body.CurrentStepIndex
	} else if body.InvestigationState != nil && body.InvestigationState.CurrentStepIndex > 0 {
		req.CurrentStepIndex = body.InvestigationState.CurrentStepIndex
	}

	req.StepType = body.StepType
	req.TargetFileID = body.TargetFileID
	req.TargetSymbolID = body.TargetSymbolID
	req.RootSymbolID = body.RootSymbolID
	req.TargetNodeID = body.TargetNodeID

	orch := s.guideOrchestrator
	if orch == nil {
		engine, _ := s.getOrLoadEngine(r.Context(), id)
		summaryGen := guide.NewDefaultArchitectureSummaryGenerator(s.store, engine)
		composer := retrieval.NewDefaultEvidenceComposer(nil, nil, s.store)
		expSvc := llm.NewGroundedExplanationService(nil, nil, composer, nil)
		orch = guide.NewDefaultGuideOrchestrator(s.store, summaryGen, composer, expSvc)
	}

	inv, err := orch.Guide(r.Context(), req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	s.respondJSON(w, http.StatusOK, inv)
}
