"use client";

import React from "react";
import { Filter, RefreshCw, Layers, Sliders } from "lucide-react";

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
    <div className="h-11 border-b border-border bg-surface px-4 flex items-center justify-between select-none shrink-0 text-xs">
      <div className="flex items-center space-x-4">
        {/* Scope Indicator */}
        <div className="flex items-center space-x-1.5 font-semibold text-gray-200">
          <Layers className="w-3.5 h-3.5 text-accent" />
          <span>Scope:</span>
          <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded bg-accent/15 border border-accent/30 text-accent font-mono">
            {scope}
          </span>
        </div>

        {/* Node Limit Selector */}
        <div className="flex items-center space-x-1 text-gray-400 border-l border-border pl-3">
          <Sliders className="w-3.5 h-3.5 text-gray-400" />
          <span>Max Nodes:</span>
          <select
            value={nodeLimit}
            onChange={(e) => onSetNodeLimit(Number(e.target.value))}
            className="bg-background border border-border text-xs rounded px-2 py-0.5 text-gray-200 focus:outline-none focus:border-accent"
          >
            <option value={20}>20 (Overview)</option>
            <option value={50}>50 (Neighborhood)</option>
            <option value={100}>100 (Max Bounded)</option>
          </select>
        </div>

        {/* Node Type Presentation Filters */}
        <div className="flex items-center space-x-1.5 border-l border-border pl-3">
          <span className="text-gray-400 font-medium">Nodes:</span>
          <button
            onClick={() => toggleNodeType("NODE_FILE")}
            className={`px-2 py-0.5 rounded border text-[10px] font-mono transition ${
              nodeTypesFilter.has("NODE_FILE")
                ? "bg-blue-500/20 text-blue-300 border-blue-500/40"
                : "bg-background text-gray-500 border-border"
            }`}
          >
            FILES
          </button>
          <button
            onClick={() => toggleNodeType("NODE_SYMBOL")}
            className={`px-2 py-0.5 rounded border text-[10px] font-mono transition ${
              nodeTypesFilter.has("NODE_SYMBOL")
                ? "bg-purple-500/20 text-purple-300 border-purple-500/40"
                : "bg-background text-gray-500 border-border"
            }`}
          >
            SYMBOLS
          </button>
          <button
            onClick={() => toggleNodeType("NODE_EXTERNAL_MODULE")}
            className={`px-2 py-0.5 rounded border text-[10px] font-mono transition ${
              nodeTypesFilter.has("NODE_EXTERNAL_MODULE")
                ? "bg-amber-500/20 text-amber-300 border-amber-500/40"
                : "bg-background text-gray-500 border-border"
            }`}
          >
            EXTERNALS
          </button>
        </div>

        {/* Edge Type Presentation Filters */}
        <div className="flex items-center space-x-1.5 border-l border-border pl-3">
          <span className="text-gray-400 font-medium">Edges:</span>
          <button
            onClick={() => toggleEdgeType("EDGE_CONTAINS")}
            className={`px-2 py-0.5 rounded border text-[10px] font-mono transition ${
              edgeTypesFilter.has("EDGE_CONTAINS")
                ? "bg-emerald-500/20 text-emerald-300 border-emerald-500/40"
                : "bg-background text-gray-500 border-border"
            }`}
          >
            CONTAINS
          </button>
          <button
            onClick={() => toggleEdgeType("EDGE_IMPORTS")}
            className={`px-2 py-0.5 rounded border text-[10px] font-mono transition ${
              edgeTypesFilter.has("EDGE_IMPORTS")
                ? "bg-blue-500/20 text-blue-300 border-blue-500/40"
                : "bg-background text-gray-500 border-border"
            }`}
          >
            IMPORTS
          </button>
          <button
            onClick={() => toggleEdgeType("EDGE_CALLS")}
            className={`px-2 py-0.5 rounded border text-[10px] font-mono transition ${
              edgeTypesFilter.has("EDGE_CALLS")
                ? "bg-purple-500/20 text-purple-300 border-purple-500/40"
                : "bg-background text-gray-500 border-border"
            }`}
          >
            CALLS
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
          <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? "animate-spin text-accent" : ""}`} />
          Reset Graph
        </button>
      </div>
    </div>
  );
};
