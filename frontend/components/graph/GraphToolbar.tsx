"use client";

import React from "react";
import { Layers, Sliders, RefreshCw, GitFork, LayoutGrid } from "lucide-react";
import { useAppStore } from "@/store/useAppStore";

interface GraphToolbarProps {
  scope: "OVERVIEW" | "NEIGHBORHOOD";
  nodeLimit: number;
  nodeTypesFilter: Set<string>;
  edgeTypesFilter: Set<string>;
  onSetScope: (scope: "OVERVIEW" | "NEIGHBORHOOD") => void;
  onSetNodeLimit: (limit: number) => void;
  onSetNodeTypesFilter: (types: Set<string>) => void;
  onSetEdgeTypesFilter: (types: Set<string>) => void;
  onResetGraph: () => void;
  isLoading: boolean;
}

export const GraphToolbar: React.FC<GraphToolbarProps> = ({
  scope,
  nodeLimit,
  nodeTypesFilter,
  edgeTypesFilter,
  onSetNodeLimit,
  onSetNodeTypesFilter,
  onSetEdgeTypesFilter,
  onResetGraph,
  isLoading,
}) => {
  const { graphViewMode, setGraphViewMode } = useAppStore();

  const toggleNodeType = (type: string) => {
    const next = new Set(nodeTypesFilter);
    if (next.has(type)) {
      if (next.size > 1) next.delete(type);
    } else {
      next.add(type);
    }
    onSetNodeTypesFilter(next);
  };

  const toggleEdgeType = (type: string) => {
    const next = new Set(edgeTypesFilter);
    if (next.has(type)) {
      if (next.size > 1) next.delete(type);
    } else {
      next.add(type);
    }
    onSetEdgeTypesFilter(next);
  };

  return (
    <div className="h-11 border-b border-border bg-surface px-3.5 flex items-center justify-between select-none shrink-0 text-xs font-mono">
      <div className="flex items-center space-x-3">
        {/* Architecture Mode Toggle */}
        <div className="flex bg-background border border-border rounded p-0.5">
          <button
            onClick={() => setGraphViewMode("ARCHITECTURE")}
            className={`px-2.5 py-1 rounded text-[11px] font-semibold transition flex items-center gap-1.5 ${
              graphViewMode === "ARCHITECTURE"
                ? "bg-cyan-950/60 text-cyan-300 border border-cyan-500/40"
                : "text-gray-400 hover:text-gray-200"
            }`}
            title="Repository-level structural module overview"
          >
            <LayoutGrid className="w-3.5 h-3.5 text-cyan-400" />
            <span>ARCHITECTURE</span>
          </button>
          <button
            onClick={() => setGraphViewMode("DETAILED")}
            className={`px-2.5 py-1 rounded text-[11px] font-semibold transition flex items-center gap-1.5 ${
              graphViewMode === "DETAILED"
                ? "bg-cyan-950/60 text-cyan-300 border border-cyan-500/40"
                : "text-gray-400 hover:text-gray-200"
            }`}
            title="Interactive code & symbol intelligence graph"
          >
            <GitFork className="w-3.5 h-3.5 text-cyan-400" />
            <span>CODEGRAPH</span>
          </button>
        </div>

        <div className="h-4 w-[1px] bg-border" />

        {/* Node Limit Selector */}
        <div className="flex items-center space-x-1.5 text-gray-400 font-mono">
          <Sliders className="w-3.5 h-3.5 text-cyan-400" />
          <span className="text-[10px] text-gray-400 font-bold uppercase tracking-wider">MAX RENDER LIMIT:</span>
          <select
            value={nodeLimit}
            onChange={(e) => onSetNodeLimit(Number(e.target.value))}
            className="bg-background border border-border text-[11px] font-mono rounded px-2 py-0.5 text-cyan-300 font-bold focus:outline-none focus:border-cyan-500"
            title="Maximum number of graph nodes rendered in current view"
          >
            <option value={25}>25 Nodes</option>
            <option value={50}>50 Nodes</option>
            <option value={100}>100 Nodes</option>
            <option value={200}>200 Nodes</option>
          </select>
        </div>

        <div className="h-4 w-[1px] bg-border" />

        {/* Node Presentation Filters */}
        <div className="flex items-center space-x-1">
          <span className="text-gray-500 mr-1">NODES:</span>
          <button
            onClick={() => toggleNodeType("NODE_FILE")}
            className={`px-2 py-0.5 rounded border text-[10px] transition ${
              nodeTypesFilter.has("NODE_FILE")
                ? "bg-gray-800 text-gray-200 border-gray-600"
                : "bg-background text-gray-600 border-border"
            }`}
          >
            FILES
          </button>
          <button
            onClick={() => toggleNodeType("NODE_SYMBOL")}
            className={`px-2 py-0.5 rounded border text-[10px] transition ${
              nodeTypesFilter.has("NODE_SYMBOL")
                ? "bg-cyan-950/60 text-cyan-300 border-cyan-500/40"
                : "bg-background text-gray-600 border-border"
            }`}
          >
            SYMBOLS
          </button>
          <button
            onClick={() => toggleNodeType("NODE_EXTERNAL_MODULE")}
            className={`px-2 py-0.5 rounded border text-[10px] transition ${
              nodeTypesFilter.has("NODE_EXTERNAL_MODULE")
                ? "bg-amber-950/40 text-amber-300 border-amber-500/40"
                : "bg-background text-gray-600 border-border"
            }`}
          >
            EXTERNALS
          </button>
        </div>
      </div>

      <div className="flex items-center space-x-2">
        <button
          onClick={onResetGraph}
          disabled={isLoading}
          className="px-2.5 py-1 rounded border border-border hover:bg-surface-hover text-gray-300 hover:text-white transition flex items-center gap-1.5"
          title="Reset Graph Overview"
        >
          <RefreshCw className={`w-3 h-3 ${isLoading ? "animate-spin text-cyan-400" : ""}`} />
          <span>Reset</span>
        </button>
      </div>
    </div>
  );
};
