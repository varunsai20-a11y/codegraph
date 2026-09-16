package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"codegraph/internal/config"
	"codegraph/internal/graph"
	"codegraph/internal/ingestion"
	"codegraph/internal/models"
	"codegraph/internal/repository"
	"codegraph/internal/security"
	"codegraph/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type Server struct {
	cfg     *config.Config
	store   storage.Storage
	wsMgr   *repository.WorkspaceManager
	indexer *ingestion.Indexer
	router  chi.Router
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
		r.Get("/repositories/{id}/flow", s.handleGetFlow)
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
	workspacePath, err := s.wsMgr.PrepareWorkspace(repo)
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

	if req.Name == "" {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "repository name is required"})
		return
	}

	if req.SourceType != models.SourceTypeLocal && req.SourceType != models.SourceTypeGit {
		s.respondJSON(w, http.StatusBadRequest, map[string]string{"error": "source_type must be LOCAL or GIT"})
		return
	}

	repoID := uuid.New().String()
	now := time.Now()
	repo := &models.Repository{
		ID:         repoID,
		Name:       req.Name,
		SourceType: req.SourceType,
		SourceURL:  req.SourceURL,
		LocalPath:  req.LocalPath,
		Status:     models.RepoStatusRegistered,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.store.CreateRepository(r.Context(), repo); err != nil {
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

	jobID := uuid.New().String()
	job := &models.IndexJob{
		ID:           jobID,
		RepositoryID: repo.ID,
		Status:       models.JobStatusPending,
		StartedAt:    time.Now(),
	}

	if err := s.store.CreateIndexJob(r.Context(), job); err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

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

	nodeLimit := 20
	if scope == "NEIGHBORHOOD" {
		nodeLimit = 50
	}
	if lStr := query.Get("node_limit"); lStr != "" {
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

func (s *Server) getOrLoadEngine(ctx context.Context, repoID string) (*graph.Engine, bool) {
	engine, found := s.indexer.GetGraphEngine(repoID)
	if found {
		return engine, true
	}

	// Fallback load from SQLite
	nodes, edges, err := s.store.GetGraphForRepository(ctx, repoID)
	if err != nil || len(nodes) == 0 {
		return nil, false
	}

	engine = graph.NewEngine(repoID)
	engine.LoadGraph(nodes, edges)
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
