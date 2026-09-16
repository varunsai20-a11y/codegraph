export type SourceType = "LOCAL" | "GIT";
export type RepoStatus = "REGISTERED" | "INDEXING" | "INDEXED" | "FAILED";

export interface Repository {
  id: string;
  name: string;
  source_type: SourceType;
  source_url?: string;
  local_path?: string;
  status: RepoStatus;
  created_at: string;
  updated_at: string;
}

export type FileStatus =
  | "INDEXED"
  | "IGNORED"
  | "BINARY"
  | "SECRET"
  | "OVERSIZED"
  | "UNSUPPORTED"
  | "FAILED";

export interface FileManifestItem {
  id: string;
  repository_id: string;
  relative_path: string;
  language: string;
  extension: string;
  size: number;
  sha256: string;
  status: FileStatus;
  error_message?: string;
  updated_at: string;
}

export interface FileContentResponse {
  repository_id: string;
  relative_path: string;
  total_lines: number;
  content: string;
  language: string;
}

export interface APIError {
  error: string;
}

export interface SourceLocation {
  relative_path: string;
  start_line: number;
  end_line: number;
  start_column?: number;
  end_column?: number;
}

export interface TreeNode {
  name: string;
  path: string;
  isFolder: boolean;
  children?: TreeNode[];
  fileItem?: FileManifestItem;
}

export type NodeKind =
  | "NODE_REPOSITORY"
  | "NODE_FILE"
  | "NODE_SYMBOL"
  | "NODE_EXTERNAL_MODULE";

export type EdgeKind =
  | "EDGE_CONTAINS"
  | "EDGE_IMPORTS"
  | "EDGE_EXPORTS"
  | "EDGE_CALLS"
  | "EDGE_EXTENDS"
  | "EDGE_IMPLEMENTS";

export interface GraphNode {
  id: string;
  repository_id: string;
  kind: NodeKind;
  label: string;
  qualified_name: string;
  file_id?: string;
  relative_path?: string;
  location?: SourceLocation;
  updated_at: string;
}

export interface GraphEdge {
  id: string;
  repository_id: string;
  source_id: string;
  target_id: string;
  target_kind?: string;
  kind: EdgeKind;
  status?: string;
  file_id?: string;
  location?: SourceLocation;
  updated_at: string;
}

export interface GraphResponse {
  repository_id: string;
  node_count: number;
  edge_count: number;
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface GraphQueryParams {
  scope?: "OVERVIEW" | "NEIGHBORHOOD";
  target?: string;
  depth?: number;
  node_limit?: number;
  edge_limit?: number;
  node_types?: string;
  edge_types?: string;
}

export type FlowTerminationReason =
  | "TARGET_REACHED"
  | "DEPTH_LIMIT"
  | "NODE_LIMIT"
  | "NO_PATH"
  | "CYCLE_BOUNDARY"
  | "INVALID_ROOT"
  | "TARGET_NOT_FOUND"
  | "MAX_PATHS_REACHED";

export interface FlowStep {
  sequence: number;
  node_id: string;
  node: GraphNode;
  incoming_edge?: GraphEdge;
  outgoing_edge?: GraphEdge;
}

export interface FlowPath {
  path_id: string;
  steps: FlowStep[];
  length: number;
  contains_cycle: boolean;
  reaches_target: boolean;
  has_external_call: boolean;
  has_unresolved_call: boolean;
}

export interface StaticFlowResult {
  repository_id: string;
  root_node_id: string;
  target_node_id?: string;
  flow_type: string;
  max_depth: number;
  max_nodes: number;
  nodes_visited: number;
  nodes: GraphNode[];
  edges: GraphEdge[];
  path?: FlowPath;
  termination_reason: FlowTerminationReason;
  truncated: boolean;
  cycle_detected: boolean;
  multiple_paths_possible: boolean;
  query_latency_ms: number;
  notice: string;
}

export interface FlowQueryParams {
  root: string;
  target?: string;
  max_depth?: number;
  max_nodes?: number;
  max_paths?: number;
}

export interface EvidenceReference {
  label: string;
  stable_id: string;
}

export interface ExplanationClaim {
  id: string;
  text: string;
  evidence_references?: EvidenceReference[];
  grounding_status: string;
  is_valid: boolean;
  reason?: string;
}

export interface ExplanationEvidence {
  id: string;
  stable_id: string;
  label: string;
  repository_id: string;
  type: string;
  file_id?: string;
  relative_path: string;
  location: SourceLocation;
  symbol_id?: string;
  symbol_name?: string;
  content: string;
  retriever_type: string;
}

export interface Citation {
  evidence_id: string;
  stable_id: string;
  relative_path: string;
  location: SourceLocation;
  is_valid: boolean;
  reason?: string;
}

export interface GroundingMetadata {
  evidence_count: number;
  cited_evidence_count: number;
  citation_validation_status: string;
  total_claims: number;
  grounded_claims: number;
  unsupported_claim_count: number;
  status: string;
  sufficiency: string;
}

export interface ExplanationResponse {
  repository_id: string;
  question: string;
  status: string;
  answer: string;
  claims?: ExplanationClaim[];
  evidence?: ExplanationEvidence[];
  citations?: Citation[];
  grounding: GroundingMetadata;
  provider?: string;
  model?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  latency_ms?: number;
  citation_validation_status: string;
  is_insufficient_evidence: boolean;
  error_message?: string;
}
