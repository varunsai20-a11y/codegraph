import { create } from "zustand";
import {
  Repository,
  FileManifestItem,
  FileContentResponse,
  GraphNode,
  GraphEdge,
  GraphQueryParams,
  StaticFlowResult,
  FlowQueryParams,
} from "@/lib/types";
import { apiClient } from "@/lib/api-client";

export type NavigationTab = "EXPLORER" | "GRAPH" | "SYNC" | "FLOW" | "AI" | "GUIDE";

let manifestAbortController: AbortController | null = null;
let sourceAbortController: AbortController | null = null;
let graphAbortController: AbortController | null = null;
let flowAbortController: AbortController | null = null;

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

  // Graph C3/C4 State
  graphNodes: GraphNode[];
  graphEdges: GraphEdge[];
  graphScope: "OVERVIEW" | "NEIGHBORHOOD";
  graphNodeTypesFilter: Set<string>;
  graphEdgeTypesFilter: Set<string>;
  graphNodeLimit: number;
  expandedNodeIDs: Set<string>;
  isLoadingGraph: boolean;
  graphError: string | null;

  // C5 Static Flow State
  flowResult: StaticFlowResult | null;
  flowRootNodeID: string | null;
  flowTargetNodeID: string | null;
  flowMaxDepth: number;
  selectedFlowStepIndex: number | null;
  isLoadingFlow: boolean;
  flowError: string | null;

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
  fetchGraph: (repoID: string, params?: GraphQueryParams) => Promise<void>;
  expandGraphNode: (repoID: string, nodeID: string) => Promise<void>;
  setGraphScope: (scope: "OVERVIEW" | "NEIGHBORHOOD") => void;
  setGraphNodeTypesFilter: (types: Set<string>) => void;
  setGraphEdgeTypesFilter: (types: Set<string>) => void;
  setGraphNodeLimit: (limit: number) => void;

  // C5 Flow Actions
  fetchStaticFlow: (
    repoID: string,
    rootID: string,
    targetID?: string,
    maxDepth?: number
  ) => Promise<void>;
  setFlowRootNodeID: (nodeID: string | null) => void;
  setFlowTargetNodeID: (nodeID: string | null) => void;
  setFlowMaxDepth: (depth: number) => void;
  selectFlowStep: (index: number | null) => void;
  traceFlowFromSymbol: (symbolID: string, targetTab?: NavigationTab) => void;

  resetGraph: (repoID: string) => Promise<void>;
  toggleExpandPath: (path: string) => void;
  setExpandedPaths: (paths: Set<string>) => void;
  autoExpandParentPaths: (relativePath: string) => void;
  setSearchQuery: (query: string) => void;
  setActiveTab: (tab: NavigationTab) => void;

  // C4 Synchronization Actions
  selectGraphNode: (nodeID: string | null) => void;
  syncSourceFromGraphNode: (nodeID: string) => void;
  selectSourceFileAndSyncGraph: (relativePath: string) => void;
  navigateToSourceFromGraph: (nodeID: string, targetTab?: NavigationTab) => void;
  navigateToGraphFromSource: () => void;

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

  // Graph C3 State defaults
  graphNodes: [],
  graphEdges: [],
  graphScope: "OVERVIEW",
  graphNodeTypesFilter: new Set<string>([
    "NODE_REPOSITORY",
    "NODE_FILE",
    "NODE_SYMBOL",
    "NODE_EXTERNAL_MODULE",
  ]),
  graphEdgeTypesFilter: new Set<string>([
    "EDGE_CONTAINS",
    "EDGE_IMPORTS",
    "EDGE_EXPORTS",
    "EDGE_CALLS",
    "EDGE_EXTENDS",
    "EDGE_IMPLEMENTS",
  ]),
  graphNodeLimit: 20,
  expandedNodeIDs: new Set<string>(),
  isLoadingGraph: false,
  graphError: null,

  isLoadingRepos: false,
  isLoadingManifest: false,
  isLoadingSource: false,

  // C5 Flow Defaults
  flowResult: null,
  flowRootNodeID: null,
  flowTargetNodeID: null,
  flowMaxDepth: 10,
  selectedFlowStepIndex: null,
  isLoadingFlow: false,
  flowError: null,

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
        get().fetchGraph(active.id, { scope: "OVERVIEW", node_limit: 20 });
      } else {
        set({ fileManifest: [], selectedPath: null, sourceContent: null, graphNodes: [], graphEdges: [] });
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
    if (graphAbortController) {
      graphAbortController.abort();
      graphAbortController = null;
    }
    if (flowAbortController) {
      flowAbortController.abort();
      flowAbortController = null;
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
      graphNodes: [],
      graphEdges: [],
      graphScope: "OVERVIEW",
      expandedNodeIDs: new Set<string>(),
      flowResult: null,
      flowRootNodeID: null,
      flowTargetNodeID: null,
      selectedFlowStepIndex: null,
      manifestError: null,
      sourceError: null,
      graphError: null,
      flowError: null,
    });

    if (active) {
      await Promise.all([
        get().fetchFileManifest(id),
        get().fetchGraph(id, { scope: "OVERVIEW", node_limit: 20 }),
      ]);
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

  fetchGraph: async (repoID: string, params?: GraphQueryParams) => {
    if (get().activeRepoID !== repoID) {
      return;
    }

    if (graphAbortController) {
      graphAbortController.abort();
    }
    graphAbortController = new AbortController();
    const signal = graphAbortController.signal;

    set({ isLoadingGraph: true, graphError: null });

    try {
      const res = await apiClient.getGraph(repoID, params, signal);
      if (get().activeRepoID !== repoID) {
        return;
      }

      set({
        graphNodes: res.nodes,
        graphEdges: res.edges,
        graphScope: params?.scope || "OVERVIEW",
        isLoadingGraph: false,
      });
    } catch (err: any) {
      if (err.message === "Request cancelled" || signal.aborted) {
        return;
      }
      if (get().activeRepoID === repoID) {
        set({
          isLoadingGraph: false,
          graphError: err.message || "Failed to load repository graph",
        });
      }
    }
  },

  expandGraphNode: async (repoID: string, nodeID: string) => {
    if (get().activeRepoID !== repoID) return;

    if (graphAbortController) {
      graphAbortController.abort();
    }
    graphAbortController = new AbortController();
    const signal = graphAbortController.signal;

    set({ isLoadingGraph: true, graphError: null });

    try {
      const res = await apiClient.getGraph(
        repoID,
        {
          scope: "NEIGHBORHOOD",
          target: nodeID,
          depth: 1,
          node_limit: 50,
          edge_limit: 100,
        },
        signal
      );

      if (get().activeRepoID !== repoID) return;

      const nextExpanded = new Set<string>();
      nextExpanded.add(nodeID);

      set({
        graphNodes: res.nodes,
        graphEdges: res.edges,
        graphScope: "NEIGHBORHOOD",
        expandedNodeIDs: nextExpanded,
        selectedNodeID: nodeID,
        isLoadingGraph: false,
      });
    } catch (err: any) {
      if (err.message === "Request cancelled" || signal.aborted) return;
      if (get().activeRepoID === repoID) {
        set({
          isLoadingGraph: false,
          graphError: err.message || "Failed to expand node neighborhood",
        });
      }
    }
  },

  setGraphScope: (scope) => set({ graphScope: scope }),
  setGraphNodeTypesFilter: (types) => set({ graphNodeTypesFilter: new Set(types) }),
  setGraphEdgeTypesFilter: (types) => set({ graphEdgeTypesFilter: new Set(types) }),
  setGraphNodeLimit: (limit) => set({ graphNodeLimit: limit }),

  resetGraph: async (repoID: string) => {
    set({
      graphScope: "OVERVIEW",
      selectedNodeID: null,
      expandedNodeIDs: new Set<string>(),
    });
    await get().fetchGraph(repoID, { scope: "OVERVIEW", node_limit: 20 });
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

  autoExpandParentPaths: (relativePath: string) => {
    if (!relativePath) return;
    const parts = relativePath.split("/").filter(Boolean);
    if (parts.length <= 1) return;

    const newAncestors = new Set<string>();
    let currentPath = "";
    for (let i = 0; i < parts.length - 1; i++) {
      currentPath = currentPath ? `${currentPath}/${parts[i]}` : parts[i];
      newAncestors.add(currentPath);
    }

    set((state) => {
      const next = new Set(state.expandedPaths);
      let changed = false;
      newAncestors.forEach((p) => {
        if (!next.has(p)) {
          next.add(p);
          changed = true;
        }
      });
      return changed ? { expandedPaths: next } : state;
    });
  },

  // C4 Synchronization Actions
  selectGraphNode: (nodeID: string | null) => {
    set({ selectedNodeID: nodeID });

    // Non-intrusive in GRAPH tab: only sync source automatically in SYNC tab!
    if (nodeID && get().activeTab === "SYNC") {
      get().syncSourceFromGraphNode(nodeID);
    }
  },

  syncSourceFromGraphNode: (nodeID: string) => {
    const node = get().graphNodes.find((n) => n.id === nodeID);
    if (!node) return;

    const repoID = get().activeRepoID;
    if (!repoID) return;

    if (node.relative_path) {
      set({ selectedPath: node.relative_path });
      get().autoExpandParentPaths(node.relative_path);
      get().fetchSourceFile(repoID, node.relative_path);
    }

    if (node.kind === "NODE_SYMBOL") {
      set({ selectedSymbolID: node.id });
      if (node.location && node.location.start_line > 0 && node.location.end_line >= node.location.start_line) {
        set({ highlightLineRange: [node.location.start_line, node.location.end_line] });
      } else {
        set({ highlightLineRange: null });
      }
    } else {
      set({ selectedSymbolID: null, highlightLineRange: null });
    }
  },

  selectSourceFileAndSyncGraph: (relativePath: string) => {
    const repoID = get().activeRepoID;
    if (!repoID) return;

    set({
      selectedPath: relativePath,
      selectedSymbolID: null,
      highlightLineRange: null,
    });

    get().autoExpandParentPaths(relativePath);
    get().fetchSourceFile(repoID, relativePath);

    // Sync matching file node in graph if present in graphNodes (do NOT fabricate)
    const matchingFileNode = get().graphNodes.find(
      (n) => n.kind === "NODE_FILE" && n.relative_path === relativePath
    );

    if (matchingFileNode) {
      set({ selectedNodeID: matchingFileNode.id });
    }
  },

  navigateToSourceFromGraph: (nodeID: string, targetTab: NavigationTab = "EXPLORER") => {
    get().syncSourceFromGraphNode(nodeID);
    set({ activeTab: targetTab });
  },

  navigateToGraphFromSource: () => {
    set({ activeTab: "GRAPH" });
  },

  // C5 Flow Actions
  fetchStaticFlow: async (
    repoID: string,
    rootID: string,
    targetID?: string,
    maxDepth?: number
  ) => {
    if (flowAbortController) {
      flowAbortController.abort();
    }
    flowAbortController = new AbortController();
    const signal = flowAbortController.signal;

    set({
      isLoadingFlow: true,
      flowError: null,
      flowRootNodeID: rootID,
      flowTargetNodeID: targetID || null,
    });

    try {
      const res = await apiClient.getStaticFlow(
        repoID,
        { root: rootID, target: targetID, max_depth: maxDepth || get().flowMaxDepth },
        signal
      );

      if (get().activeRepoID !== repoID) return;

      const steps = res.path?.steps || [];
      const hasSteps = steps.length > 0;

      set({
        flowResult: res,
        selectedFlowStepIndex: hasSteps ? 0 : null,
        isLoadingFlow: false,
      });

      if (hasSteps) {
        get().selectFlowStep(0);
      }
    } catch (err: any) {
      if (err.message === "Request cancelled" || signal.aborted) return;
      if (get().activeRepoID === repoID) {
        set({
          isLoadingFlow: false,
          flowError: err.message || "Failed to trace static call flow",
        });
      }
    }
  },

  setFlowRootNodeID: (nodeID: string | null) => set({ flowRootNodeID: nodeID }),
  setFlowTargetNodeID: (nodeID: string | null) => set({ flowTargetNodeID: nodeID }),
  setFlowMaxDepth: (depth: number) => set({ flowMaxDepth: depth }),

  selectFlowStep: (index: number | null) => {
    set({ selectedFlowStepIndex: index });
    if (index === null) return;

    const res = get().flowResult;
    const steps = res?.path?.steps;
    if (!steps || index < 0 || index >= steps.length) return;

    const step = steps[index];
    if (step && step.node) {
      set({ selectedNodeID: step.node_id });
      if (step.node.relative_path) {
        set({ selectedPath: step.node.relative_path });
        get().autoExpandParentPaths(step.node.relative_path);
        const repoID = get().activeRepoID;
        if (repoID) {
          get().fetchSourceFile(repoID, step.node.relative_path);
        }
      }
      if (step.node.kind === "NODE_SYMBOL") {
        set({ selectedSymbolID: step.node.id });
        if (step.node.location && step.node.location.start_line > 0) {
          set({ highlightLineRange: [step.node.location.start_line, step.node.location.end_line] });
        } else {
          set({ highlightLineRange: null });
        }
      }
    }
  },

  traceFlowFromSymbol: (symbolID: string, targetTab: NavigationTab = "FLOW") => {
    const repoID = get().activeRepoID;
    set({
      activeTab: targetTab,
      flowRootNodeID: symbolID,
      flowTargetNodeID: null,
      selectedNodeID: symbolID,
    });
    if (repoID) {
      get().fetchStaticFlow(repoID, symbolID);
    }
  },

  setSearchQuery: (query: string) => set({ searchQuery: query }),
  setActiveTab: (tab: NavigationTab) => set({ activeTab: tab }),
  setSelectedPath: (path: string | null) => set({ selectedPath: path }),
  setSelectedSymbolID: (id: string | null) => set({ selectedSymbolID: id }),
  setSelectedNodeID: (id: string | null) => set({ selectedNodeID: id }),
  setHighlightLineRange: (range: [number, number] | null) => set({ highlightLineRange: range }),
  setError: (err: string | null) => set({ error: err }),
}));
