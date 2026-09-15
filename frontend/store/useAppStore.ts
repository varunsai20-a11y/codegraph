import { create } from "zustand";
import { Repository, FileManifestItem, FileContentResponse } from "@/lib/types";
import { apiClient } from "@/lib/api-client";

export type NavigationTab = "EXPLORER" | "GRAPH" | "SYNC" | "FLOW" | "AI" | "GUIDE";

let manifestAbortController: AbortController | null = null;
let sourceAbortController: AbortController | null = null;

interface AppState {
  repositories: Repository[];
  activeRepoID: string | null;
  activeRepo: Repository | null;
  activeTab: NavigationTab;
  fileManifest: FileManifestItem[];
  selectedPath: string | null;
  sourceContent: FileContentResponse | null;
  expandedPaths: Set<string>;
  searchQuery: string;
  selectedSymbolID: string | null;
  selectedNodeID: string | null;
  highlightLineRange: [number, number] | null;

  isLoadingRepos: boolean;
  isLoadingManifest: boolean;
  isLoadingSource: boolean;

  error: string | null;
  manifestError: string | null;
  sourceError: string | null;

  // Actions
  fetchRepositories: () => Promise<void>;
  selectRepository: (id: string) => Promise<void>;
  fetchFileManifest: (repoID: string) => Promise<void>;
  fetchSourceFile: (repoID: string, relativePath: string) => Promise<void>;
  toggleExpandPath: (path: string) => void;
  setExpandedPaths: (paths: Set<string>) => void;
  setSearchQuery: (query: string) => void;
  setActiveTab: (tab: NavigationTab) => void;
  setSelectedPath: (path: string | null) => void;
  setSelectedSymbolID: (id: string | null) => void;
  setSelectedNodeID: (id: string | null) => void;
  setHighlightLineRange: (range: [number, number] | null) => void;
  setError: (err: string | null) => void;
}

export const useAppStore = create<AppState>((set, get) => ({
  repositories: [],
  activeRepoID: null,
  activeRepo: null,
  activeTab: "EXPLORER",
  fileManifest: [],
  selectedPath: null,
  sourceContent: null,
  expandedPaths: new Set<string>(),
  searchQuery: "",
  selectedSymbolID: null,
  selectedNodeID: null,
  highlightLineRange: null,

  isLoadingRepos: false,
  isLoadingManifest: false,
  isLoadingSource: false,

  error: null,
  manifestError: null,
  sourceError: null,

  fetchRepositories: async () => {
    set({ isLoadingRepos: true, error: null });
    try {
      const repos = await apiClient.listRepositories();
      const currentActiveID = get().activeRepoID;
      let active = repos.find((r) => r.id === currentActiveID) || null;

      if (!active && repos.length > 0) {
        active = repos[0];
      }

      set({
        repositories: repos,
        activeRepo: active,
        activeRepoID: active ? active.id : null,
        isLoadingRepos: false,
      });

      if (active) {
        get().fetchFileManifest(active.id);
      } else {
        set({ fileManifest: [], selectedPath: null, sourceContent: null });
      }
    } catch (err: any) {
      set({
        isLoadingRepos: false,
        error: err.message || "Failed to connect to CodeGraph backend",
      });
    }
  },

  selectRepository: async (id: string) => {
    if (manifestAbortController) {
      manifestAbortController.abort();
      manifestAbortController = null;
    }
    if (sourceAbortController) {
      sourceAbortController.abort();
      sourceAbortController = null;
    }

    const repos = get().repositories;
    const active = repos.find((r) => r.id === id) || null;

    set({
      activeRepoID: id,
      activeRepo: active,
      fileManifest: [],
      selectedPath: null,
      sourceContent: null,
      expandedPaths: new Set<string>(),
      searchQuery: "",
      selectedSymbolID: null,
      selectedNodeID: null,
      highlightLineRange: null,
      manifestError: null,
      sourceError: null,
    });

    if (active) {
      await get().fetchFileManifest(id);
    }
  },

  fetchFileManifest: async (repoID: string) => {
    if (manifestAbortController) {
      manifestAbortController.abort();
    }
    manifestAbortController = new AbortController();
    const signal = manifestAbortController.signal;

    set({ isLoadingManifest: true, manifestError: null });

    try {
      const manifest = await apiClient.getRepositoryFiles(repoID, signal);
      if (get().activeRepoID !== repoID) {
        return;
      }
      set({
        fileManifest: manifest,
        isLoadingManifest: false,
      });
    } catch (err: any) {
      if (err.message === "Request cancelled" || signal.aborted) {
        return;
      }
      if (get().activeRepoID === repoID) {
        set({
          isLoadingManifest: false,
          manifestError: err.message || "Failed to load repository file manifest",
        });
      }
    }
  },

  fetchSourceFile: async (repoID: string, relativePath: string) => {
    if (get().activeRepoID !== repoID) {
      return;
    }

    if (sourceAbortController) {
      sourceAbortController.abort();
    }

    set({
      selectedPath: relativePath,
      sourceContent: null,
      sourceError: null,
    });

    const manifestItem = get().fileManifest.find((f) => f.relative_path === relativePath);

    if (manifestItem?.status === "SECRET") {
      set({
        isLoadingSource: false,
        sourceContent: {
          repository_id: repoID,
          relative_path: relativePath,
          total_lines: 0,
          content: "Source unavailable for security reasons.",
          language: "SECURITY",
        },
      });
      return;
    }

    if (manifestItem?.status === "BINARY") {
      set({
        isLoadingSource: false,
        sourceContent: {
          repository_id: repoID,
          relative_path: relativePath,
          total_lines: 0,
          content: "Binary file — source preview unavailable.",
          language: "BINARY",
        },
      });
      return;
    }

    sourceAbortController = new AbortController();
    const signal = sourceAbortController.signal;

    set({ isLoadingSource: true });

    try {
      const source = await apiClient.getSourceFile(repoID, relativePath, signal);
      if (get().activeRepoID !== repoID || get().selectedPath !== relativePath) {
        return;
      }
      set({
        sourceContent: source,
        isLoadingSource: false,
      });
    } catch (err: any) {
      if (err.message === "Request cancelled" || signal.aborted) {
        return;
      }
      if (get().activeRepoID === repoID && get().selectedPath === relativePath) {
        set({
          isLoadingSource: false,
          sourceError: err.message || "Failed to load source file content",
        });
      }
    }
  },

  toggleExpandPath: (path: string) => {
    set((state) => {
      const next = new Set(state.expandedPaths);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return { expandedPaths: next };
    });
  },

  setExpandedPaths: (paths: Set<string>) => {
    set({ expandedPaths: new Set(paths) });
  },

  setSearchQuery: (query: string) => set({ searchQuery: query }),
  setActiveTab: (tab: NavigationTab) => set({ activeTab: tab }),
  setSelectedPath: (path: string | null) => set({ selectedPath: path }),
  setSelectedSymbolID: (id: string | null) => set({ selectedSymbolID: id }),
  setSelectedNodeID: (id: string | null) => set({ selectedNodeID: id }),
  setHighlightLineRange: (range: [number, number] | null) => set({ highlightLineRange: range }),
  setError: (err: string | null) => set({ error: err }),
}));
