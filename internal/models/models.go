package models

import "time"

type SourceType string

const (
	SourceTypeLocal SourceType = "LOCAL"
	SourceTypeGit   SourceType = "GIT"
)

type RepositoryStatus string

const (
	RepoStatusRegistered RepositoryStatus = "REGISTERED"
	RepoStatusAcquired   RepositoryStatus = "ACQUIRED"
	RepoStatusIndexing   RepositoryStatus = "INDEXING"
	RepoStatusIndexed    RepositoryStatus = "INDEXED"
	RepoStatusFailed     RepositoryStatus = "FAILED"
)

type Repository struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	SourceType    SourceType       `json:"source_type"`
	SourceURL     string           `json:"source_url,omitempty"`
	LocalPath     string           `json:"local_path"`
	DefaultBranch string           `json:"default_branch,omitempty"`
	CommitSHA     string           `json:"commit_sha,omitempty"`
	Status        RepositoryStatus `json:"status"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
}

type FileStatus string

const (
	FileStatusIndexed     FileStatus = "INDEXED"
	FileStatusIgnored     FileStatus = "IGNORED"
	FileStatusBinary      FileStatus = "BINARY"
	FileStatusSecret      FileStatus = "SECRET"
	FileStatusOversized   FileStatus = "OVERSIZED"
	FileStatusUnsupported FileStatus = "UNSUPPORTED"
	FileStatusFailed      FileStatus = "FAILED"
)

type FileManifestItem struct {
	ID           string     `json:"id"`
	RepositoryID string     `json:"repository_id"`
	RelativePath string     `json:"relative_path"`
	Language     string     `json:"language"`
	Extension    string     `json:"extension"`
	Size         int64      `json:"size"`
	SHA256       string     `json:"sha256"`
	Status       FileStatus `json:"status"`
	ErrorMessage string     `json:"error_message,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type JobStatus string

const (
	JobStatusPending   JobStatus = "PENDING"
	JobStatusRunning   JobStatus = "RUNNING"
	JobStatusCompleted JobStatus = "COMPLETED"
	JobStatusFailed    JobStatus = "FAILED"
)

type IndexJob struct {
	ID              string     `json:"id"`
	RepositoryID    string     `json:"repository_id"`
	Status          JobStatus  `json:"status"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	FilesDiscovered int        `json:"files_discovered"`
	FilesIndexed    int        `json:"files_indexed"`
	FilesSkipped    int        `json:"files_skipped"`
	FilesFailed     int        `json:"files_failed"`
	Error           string     `json:"error,omitempty"`
}

type IndexStats struct {
	FilesDiscovered  int           `json:"files_discovered"`
	FilesIndexed     int           `json:"files_indexed"`
	FilesIgnored     int           `json:"files_ignored"`
	FilesBinary      int           `json:"files_binary"`
	FilesSecret      int           `json:"files_secret"`
	FilesOversized   int           `json:"files_oversized"`
	FilesUnsupported int           `json:"files_unsupported"`
	FilesFailed      int           `json:"files_failed"`
	TotalDuration    time.Duration `json:"total_duration"`
	TotalSourceBytes int64         `json:"total_source_bytes"`
	IndexedBytes     int64         `json:"indexed_bytes"`
}

// Phase 2 Static Code Intelligence Domain Models

type SymbolKind string

const (
	SymbolKindFunction  SymbolKind = "FUNCTION"
	SymbolKindMethod    SymbolKind = "METHOD"
	SymbolKindClass     SymbolKind = "CLASS"
	SymbolKindInterface SymbolKind = "INTERFACE"
	SymbolKindStruct    SymbolKind = "STRUCT"
	SymbolKindEnum      SymbolKind = "ENUM"
	SymbolKindType      SymbolKind = "TYPE"
	SymbolKindVariable  SymbolKind = "VARIABLE"
	SymbolKindModule    SymbolKind = "MODULE"
)

type Location struct {
	StartLine   int `json:"start_line"`
	StartColumn int `json:"start_column"`
	EndLine     int `json:"end_line"`
	EndColumn   int `json:"end_column"`
}

type Symbol struct {
	ID            string     `json:"id"`
	RepositoryID  string     `json:"repository_id"`
	FileID        string     `json:"file_id"`
	RelativePath  string     `json:"relative_path"`
	Name          string     `json:"name"`
	QualifiedName string     `json:"qualified_name"`
	Kind          SymbolKind `json:"kind"`
	ParentID      string     `json:"parent_id,omitempty"`
	Location      Location   `json:"location"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type RelationType string

const (
	RelTypeContains   RelationType = "CONTAINS"
	RelTypeImports    RelationType = "IMPORTS"
	RelTypeExports    RelationType = "EXPORTS"
	RelTypeExtends    RelationType = "EXTENDS"
	RelTypeImplements RelationType = "IMPLEMENTS"
	RelTypeCalls      RelationType = "CALLS"
)

type RelationStatus string

const (
	RelStatusResolved   RelationStatus = "RESOLVED"
	RelStatusPartial    RelationStatus = "PARTIAL"
	RelStatusUnresolved RelationStatus = "UNRESOLVED"
)

type TargetKind string

const (
	TargetKindInternal TargetKind = "INTERNAL"
	TargetKindExternal TargetKind = "EXTERNAL"
)

type Relationship struct {
	ID           string         `json:"id"`
	RepositoryID string         `json:"repository_id"`
	SourceID     string         `json:"source_id"`
	TargetID     string         `json:"target_id"`
	TargetKind   TargetKind     `json:"target_kind"`
	Type         RelationType   `json:"type"`
	Status       RelationStatus `json:"status"`
	FileID       string         `json:"file_id"`
	Location     Location       `json:"location"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type FileAnalysis struct {
	FileID        string          `json:"file_id"`
	RelativePath  string          `json:"relative_path"`
	Language      string          `json:"language"`
	Symbols       []*Symbol       `json:"symbols"`
	Relationships []*Relationship `json:"relationships"`
	Warnings      []string        `json:"warnings,omitempty"`
}

type AnalysisResult struct {
	RepositoryID  string            `json:"repository_id"`
	FilesAnalyzed int               `json:"files_analyzed"`
	Symbols       []*Symbol         `json:"symbols"`
	Relationships []*Relationship   `json:"relationships"`
	Errors        map[string]string `json:"errors,omitempty"`
	Duration      time.Duration     `json:"duration"`
}

// Phase 3 Code Knowledge Graph Domain Models

type NodeKind string

const (
	NodeKindRepository     NodeKind = "NODE_REPOSITORY"
	NodeKindFile           NodeKind = "NODE_FILE"
	NodeKindSymbol         NodeKind = "NODE_SYMBOL"
	NodeKindExternalModule NodeKind = "NODE_EXTERNAL_MODULE"
)

type Node struct {
	ID            string    `json:"id"`
	RepositoryID  string    `json:"repository_id"`
	Kind          NodeKind  `json:"kind"`
	Label         string    `json:"label"`
	QualifiedName string    `json:"qualified_name"`
	FileID        string    `json:"file_id,omitempty"`
	RelativePath  string    `json:"relative_path,omitempty"`
	Location      Location  `json:"location"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type EdgeKind string

const (
	EdgeKindContains   EdgeKind = "EDGE_CONTAINS"
	EdgeKindImports    EdgeKind = "EDGE_IMPORTS"
	EdgeKindExports    EdgeKind = "EDGE_EXPORTS"
	EdgeKindExtends    EdgeKind = "EDGE_EXTENDS"
	EdgeKindImplements EdgeKind = "EDGE_IMPLEMENTS"
	EdgeKindCalls      EdgeKind = "EDGE_CALLS"
)

type Edge struct {
	ID           string         `json:"id"`
	RepositoryID string         `json:"repository_id"`
	SourceID     string         `json:"source_id"`
	TargetID     string         `json:"target_id"`
	TargetKind   TargetKind     `json:"target_kind"`
	Kind         EdgeKind       `json:"kind"`
	Status       RelationStatus `json:"status"`
	FileID       string         `json:"file_id"`
	Location     Location       `json:"location"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type CallSite struct {
	CallerNode *Node `json:"caller"`
	CalleeNode *Node `json:"callee"`
	Edge       *Edge `json:"edge"`
}

type ImpactAnalysisResult struct {
	TargetFileID    string   `json:"target_file_id"`
	ImpactedFiles   []string `json:"impacted_files"`
	ImpactedSymbols []string `json:"impacted_symbols"`
}

type FlowTerminationReason string

const (
	FlowReasonTargetReached  FlowTerminationReason = "TARGET_REACHED"
	FlowReasonDepthLimit     FlowTerminationReason = "DEPTH_LIMIT"
	FlowReasonNodeLimit      FlowTerminationReason = "NODE_LIMIT"
	FlowReasonNoPath         FlowTerminationReason = "NO_PATH"
	FlowReasonInvalidRoot    FlowTerminationReason = "INVALID_ROOT"
	FlowReasonTargetNotFound FlowTerminationReason = "TARGET_NOT_FOUND"
)

type FlowStep struct {
	Sequence     int    `json:"sequence"`
	NodeID       string `json:"node_id"`
	Node         *Node  `json:"node"`
	IncomingEdge *Edge  `json:"incoming_edge,omitempty"`
	OutgoingEdge *Edge  `json:"outgoing_edge,omitempty"`
}

type FlowPath struct {
	PathID            string      `json:"path_id"`
	Steps             []*FlowStep `json:"steps"`
	Length            int         `json:"length"`
	ContainsCycle     bool        `json:"contains_cycle"`
	ReachesTarget     bool        `json:"reaches_target"`
	HasExternalCall   bool        `json:"has_external_call"`
	HasUnresolvedCall bool        `json:"has_unresolved_call"`
}

type StaticFlowResult struct {
	RepositoryID          string                `json:"repository_id"`
	RootNodeID            string                `json:"root_node_id"`
	TargetNodeID          string                `json:"target_node_id,omitempty"`
	FlowType              string                `json:"flow_type"` // Always "STATIC_CALL_GRAPH"
	MaxDepth              int                   `json:"max_depth"`
	MaxNodes              int                   `json:"max_nodes"`
	NodesVisited          int                   `json:"nodes_visited"`
	Nodes                 []*Node               `json:"nodes"`
	Edges                 []*Edge               `json:"edges"`
	Path                  *FlowPath             `json:"path,omitempty"`
	TerminationReason     FlowTerminationReason `json:"termination_reason"`
	Truncated             bool                  `json:"truncated"`
	CycleDetected         bool                  `json:"cycle_detected"`
	MultiplePathsPossible bool                  `json:"multiple_paths_possible"`
	QueryLatencyMs        float64               `json:"query_latency_ms"`
	Notice                string                `json:"notice"`
}
