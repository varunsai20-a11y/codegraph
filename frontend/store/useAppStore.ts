import { create } from "zustand";
import { Repository } from "@/lib/types";
import { apiClient } from "@/lib/api-client";

export type NavigationTab = "explorer" | "graph" | "sync" | "flow" | "ai" | "guide";

interface AppState {
  repositories: Repository[];
  activeRepoID: string | null;
  activeRepo: Repository | null;
  activeTab: NavigationTab;
  selectedPath: string | null;
  selectedSymbolID: string | null;
  selectedNodeID: string | null;
  highlightLineRange: [number, number] | null;
  isLoading: boolean;
  error: string | null;

  // Actions
  fetchRepositories: () => Promise<void>;
  selectRepository: (id: string) => void;
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
  activeTab: "explorer",
  selectedPath: null,
  selectedSymbolID: null,
  selectedNodeID: null,
  highlightLineRange: null,
  isLoading: false,
  error: null,

  fetchRepositories: async () => {
    set({ isLoading: true, error: null });
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
        isLoading: false,
      });
    } catch (err: any) {
      set({
        isLoading: false,
        error: err.message || "Failed to connect to CodeGraph backend",
      });
    }
  },

  selectRepository: (id: string) => {
    const repos = get().repositories;
    const active = repos.find((r) => r.id === id) || null;
    set({
      activeRepoID: id,
      activeRepo: active,
      selectedPath: null,
      selectedSymbolID: null,
      selectedNodeID: null,
      highlightLineRange: null,
    });
  },

  setActiveTab: (tab: NavigationTab) => set({ activeTab: tab }),
  setSelectedPath: (path: string | null) => set({ selectedPath: path }),
  setSelectedSymbolID: (id: string | null) => set({ selectedSymbolID: id }),
  setSelectedNodeID: (id: string | null) => set({ selectedNodeID: id }),
  setHighlightLineRange: (range: [number, number] | null) => set({ highlightLineRange: range }),
  setError: (err: string | null) => set({ error: err }),
}));
