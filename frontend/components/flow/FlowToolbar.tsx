"use client";

import React from "react";
import {
  GitFork,
  Sliders,
  RefreshCw,
  Info,
  Code2,
  Target,
} from "lucide-react";

interface FlowToolbarProps {
  rootSymbolLabel?: string;
  targetSymbolLabel?: string;
  maxDepth: number;
  onSetMaxDepth: (depth: number) => void;
  onRefresh: () => void;
  isLoading: boolean;
}

export const FlowToolbar: React.FC<FlowToolbarProps> = ({
  rootSymbolLabel,
  targetSymbolLabel,
  maxDepth,
  onSetMaxDepth,
  onRefresh,
  isLoading,
}) => {
  return (
    <div className="border-b border-border bg-surface px-4 py-2.5 shrink-0 flex flex-col space-y-2 select-none">
      <div className="flex items-center justify-between">
        {/* Left: Section Header & Root/Target Badges */}
        <div className="flex items-center space-x-3 truncate">
          <div className="flex items-center space-x-2 text-xs font-bold text-gray-200">
            <GitFork className="w-4 h-4 text-accent shrink-0" />
            <span>Static Call Flow</span>
          </div>

          <div className="h-4 w-px bg-border shrink-0" />

          <div className="flex items-center space-x-2 text-xs font-mono truncate">
            <span className="flex items-center gap-1.5 px-2 py-0.5 rounded bg-purple-500/10 border border-purple-500/30 text-purple-300">
              <Code2 className="w-3 h-3 text-purple-400 shrink-0" />
              <strong className="text-gray-200 truncate">{rootSymbolLabel || "No Root Selected"}</strong>
            </span>

            {targetSymbolLabel && (
              <>
                <span className="text-gray-500">➔</span>
                <span className="flex items-center gap-1.5 px-2 py-0.5 rounded bg-blue-500/10 border border-blue-500/30 text-blue-300 truncate">
                  <Target className="w-3 h-3 text-blue-400 shrink-0" />
                  <span className="truncate">{targetSymbolLabel}</span>
                </span>
              </>
            )}
          </div>
        </div>

        {/* Right: Depth Control & Actions */}
        <div className="flex items-center space-x-4 text-xs">
          <div className="flex items-center space-x-2 bg-background border border-border px-2.5 py-1 rounded">
            <Sliders className="w-3.5 h-3.5 text-gray-400" />
            <span className="text-[11px] text-gray-300 font-medium">Max Depth:</span>
            <input
              type="range"
              min="1"
              max="20"
              value={maxDepth}
              onChange={(e) => onSetMaxDepth(parseInt(e.target.value, 10))}
              className="w-20 accent-accent cursor-pointer"
            />
            <span className="text-xs font-bold font-mono text-accent w-5 text-right">{maxDepth}</span>
          </div>

          <button
            onClick={onRefresh}
            disabled={isLoading || !rootSymbolLabel}
            className="px-3 py-1 rounded bg-accent text-white font-semibold text-xs hover:bg-accent/90 transition disabled:opacity-50 flex items-center space-x-1.5"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? "animate-spin" : ""}`} />
            <span>Re-trace</span>
          </button>
        </div>
      </div>

      {/* Static Analysis Disclaimer Notice */}
      <div className="bg-surface-hover/60 border border-border/80 rounded px-3 py-1 flex items-center space-x-2 text-[11px] text-gray-400">
        <Info className="w-3.5 h-3.5 text-accent shrink-0" />
        <span>
          <strong className="text-gray-300">Static Call Flow:</strong> Derived entirely from statically analyzed CALLS relationships in the knowledge graph. CodeGraph does not observe or execute runtime behavior.
        </span>
      </div>
    </div>
  );
};
