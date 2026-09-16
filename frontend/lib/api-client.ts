import {
  Repository,
  FileManifestItem,
  FileContentResponse,
  GraphResponse,
  GraphQueryParams,
  StaticFlowResult,
  FlowQueryParams,
  APIError,
} from "./types";

export class CodeGraphAPIClient {
  private baseURL: string;

  constructor(baseURL?: string) {
    this.baseURL = baseURL || process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  }

  private async request<T>(
    endpoint: string,
    options?: RequestInit & { signal?: AbortSignal; timeoutMs?: number }
  ): Promise<T> {
    const controller = new AbortController();
    const timeoutMs = options?.timeoutMs || 15000;
    const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

    let signal = controller.signal;
    if (options?.signal) {
      if (typeof AbortSignal.any === "function") {
        signal = AbortSignal.any([options.signal, controller.signal]);
      } else {
        options.signal.addEventListener("abort", () => controller.abort());
      }
    }

    try {
      const res = await fetch(`${this.baseURL}${endpoint}`, {
        ...options,
        signal,
        headers: {
          "Content-Type": "application/json",
          ...options?.headers,
        },
      });

      clearTimeout(timeoutId);

      if (!res.ok) {
        let errMessage = `HTTP Error ${res.status}: ${res.statusText}`;
        try {
          const errData: APIError = await res.json();
          if (errData?.error) {
            errMessage = errData.error;
          }
        } catch {
          // Fallback to default HTTP status text
        }
        throw new Error(errMessage);
      }

      return (await res.json()) as T;
    } catch (err: any) {
      clearTimeout(timeoutId);
      if (err.name === "AbortError") {
        if (options?.signal?.aborted) {
          throw new Error("Request cancelled");
        }
        throw new Error(`Request to ${endpoint} timed out after ${timeoutMs}ms`);
      }
      throw err;
    }
  }

  async listRepositories(signal?: AbortSignal): Promise<Repository[]> {
    return this.request<Repository[]>("/api/repositories", { signal });
  }

  async getRepository(id: string, signal?: AbortSignal): Promise<Repository> {
    return this.request<Repository>(`/api/repositories/${id}`, { signal });
  }

  async getRepositoryFiles(id: string, signal?: AbortSignal): Promise<FileManifestItem[]> {
    return this.request<FileManifestItem[]>(`/api/repositories/${id}/files`, { signal });
  }

  async getFileContent(
    id: string,
    relativePath: string,
    signal?: AbortSignal
  ): Promise<FileContentResponse> {
    const encodedPath = encodeURIComponent(relativePath);
    return this.request<FileContentResponse>(
      `/api/repositories/${id}/file?path=${encodedPath}`,
      { signal }
    );
  }

  async getSourceFile(
    id: string,
    relativePath: string,
    signal?: AbortSignal
  ): Promise<FileContentResponse> {
    return this.getFileContent(id, relativePath, signal);
  }

  async getGraph(
    id: string,
    params?: GraphQueryParams,
    signal?: AbortSignal
  ): Promise<GraphResponse> {
    const query = new URLSearchParams();
    if (params?.scope) query.set("scope", params.scope);
    if (params?.target) query.set("target", params.target);
    if (params?.depth) query.set("depth", params.depth.toString());
    if (params?.node_limit) query.set("node_limit", params.node_limit.toString());
    if (params?.edge_limit) query.set("edge_limit", params.edge_limit.toString());
    if (params?.node_types) query.set("node_types", params.node_types);
    if (params?.edge_types) query.set("edge_types", params.edge_types);

    const queryString = query.toString();
    const endpoint = `/api/repositories/${id}/graph${queryString ? `?${queryString}` : ""}`;
    return this.request<GraphResponse>(endpoint, { signal });
  }

  async getStaticFlow(
    id: string,
    params: FlowQueryParams,
    signal?: AbortSignal
  ): Promise<StaticFlowResult> {
    const query = new URLSearchParams();
    query.set("root", params.root);
    if (params.target) query.set("target", params.target);
    if (params.max_depth) query.set("max_depth", params.max_depth.toString());
    if (params.max_nodes) query.set("max_nodes", params.max_nodes.toString());
    if (params.max_paths) query.set("max_paths", params.max_paths.toString());

    const queryString = query.toString();
    const endpoint = `/api/repositories/${id}/flow?${queryString}`;
    return this.request<StaticFlowResult>(endpoint, { signal });
  }

  async explainCode(
    id: string,
    payload: {
      query: string;
      symbol_id?: string;
      root_symbol?: string;
      target_node?: string;
      flow?: boolean;
      provider?: string;
    },
    signal?: AbortSignal
  ): Promise<any> {
    return this.request<any>(`/api/repositories/${id}/explain`, {
      method: "POST",
      body: JSON.stringify(payload),
      signal,
    });
  }
}

export const apiClient = new CodeGraphAPIClient();
