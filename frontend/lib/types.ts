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
