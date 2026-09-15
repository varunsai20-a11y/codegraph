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
