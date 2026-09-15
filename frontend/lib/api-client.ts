import { Repository, FileContentResponse, APIError } from "./types";

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

    const signal = options?.signal
      ? AbortSignal.any([options.signal, controller.signal])
      : controller.signal;

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
}

export const apiClient = new CodeGraphAPIClient();
