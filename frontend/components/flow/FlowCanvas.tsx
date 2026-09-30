"use client";

import React from "react";
import { StaticFlowResult } from "@/lib/types";
import { FlowStepItem } from "./FlowStepItem";
import {
  GitFork,
  CheckCircle2,
  AlertTriangle,
  HelpCircle,
  Repeat,
  Layers,
  Layers3,
} from "lucide-react";

interface FlowCanvasProps {
  flowResult: StaticFlowResult | null;
  flowRootNodeID?: string | null;
  selectedStepIndex: number | null;
  onSelectStep: (index: number) => void;
  isLoading: boolean;
}

export const FlowCanvas: React.FC<FlowCanvasProps> = ({
  flowResult,
  flowRootNodeID,
  selectedStepIndex,
  onSelectStep,
  isLoading,
}) => {
  if (isLoading) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background">
        <div className="w-6 h-6 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin mb-3" />
        <p className="text-xs text-gray-400 font-mono">Tracing static call graph paths...</p>
      </div>
    );
  }

  // State 1: No Root Selected
  if (!flowRootNodeID && !flowResult) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none font-mono">
        <GitFork className="w-10 h-10 text-cyan-400 mb-3" />
        <h3 className="text-sm font-bold text-gray-200 uppercase tracking-wider">No Root Selected</h3>
        <p className="text-xs text-gray-400 max-w-sm mt-1 leading-relaxed font-sans">
          Select a symbol from the Knowledge Graph, Code Viewer, or Explorer and click <strong className="text-cyan-300 font-mono">&quot;Trace Static Flow&quot;</strong> to inspect static call paths.
        </p>
      </div>
    );
  }

  // State 4: Insufficient Static Evidence / Invalid Root
  if (flowResult?.termination_reason === "INVALID_ROOT") {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none font-mono">
        <div className="w-12 h-12 rounded-full bg-red-500/10 border border-red-500/30 flex items-center justify-center text-red-400 mb-3">
          <AlertTriangle className="w-6 h-6" />
        </div>
        <h3 className="text-sm font-bold text-red-300 uppercase tracking-wider">Insufficient Static Evidence</h3>
        <p className="text-xs text-red-400/90 max-w-md mt-1 leading-relaxed font-sans">
          Static call flow requires a valid symbol node in AST graph storage. The selected target does not contain static call edges.
        </p>
      </div>
    );
  }

  if (flowResult?.termination_reason === "TARGET_NOT_FOUND") {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none font-mono">
        <div className="w-12 h-12 rounded-full bg-amber-500/10 border border-amber-500/30 flex items-center justify-center text-amber-400 mb-3">
          <HelpCircle className="w-6 h-6" />
        </div>
        <h3 className="text-sm font-bold text-amber-300 uppercase tracking-wider">Target Node Not Found</h3>
        <p className="text-xs text-amber-400/90 max-w-md mt-1 leading-relaxed font-sans">
          Target symbol <code className="font-mono text-white">{flowResult.target_node_id}</code> was not found in the static relationship index.
        </p>
      </div>
    );
  }

  // State 2: Root Selected but No Call Relationships Found
  if (
    !flowResult ||
    flowResult.termination_reason === "NO_PATH" ||
    (flowResult.path?.steps && flowResult.path.steps.length <= 1)
  ) {
    const rootLabel = flowRootNodeID || flowResult?.root_node_id || "Root Symbol";
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none font-mono">
        <div className="w-12 h-12 rounded-full bg-amber-500/10 border border-amber-500/30 flex items-center justify-center text-amber-400 mb-3">
          <GitFork className="w-6 h-6 rotate-180" />
        </div>
        <h3 className="text-sm font-bold text-amber-300 uppercase tracking-wider">Root Selected — No Call Relationships Found</h3>
        <p className="text-xs text-amber-400/90 max-w-md mt-1 leading-relaxed font-sans">
          Root symbol <code className="font-mono text-white">{rootLabel}</code> was analyzed, but no static <code className="font-mono text-cyan-300">EDGE_CALLS</code> outgoing relationships exist in AST static analysis.
        </p>
      </div>
    );
  }

  const steps = flowResult.path?.steps || [];

  return (
    <div className="h-full flex flex-col bg-background overflow-hidden">
      {/* Flow Meta Header */}
      <div className="h-9 border-b border-border bg-surface/50 px-4 flex items-center justify-between shrink-0 select-none text-xs">
        <div className="flex items-center space-x-3">
          <span className="text-gray-400 font-medium">
            Path Length: <strong className="text-gray-200 font-mono">{steps.length}</strong> steps
          </span>

          <span className="text-gray-400 font-medium">
            Nodes Visited: <strong className="text-gray-200 font-mono">{flowResult.nodes_visited}</strong>
          </span>

          <span className="text-gray-400 font-medium">
            Latency: <strong className="text-gray-200 font-mono">{flowResult.query_latency_ms.toFixed(2)}ms</strong>
          </span>
        </div>

        <div className="flex items-center space-x-2">
          {flowResult.cycle_detected && (
            <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded bg-amber-500/10 border border-amber-500/30 text-amber-400 flex items-center gap-1">
              <Repeat className="w-3 h-3" /> CYCLE DETECTED
            </span>
          )}

          {flowResult.multiple_paths_possible && (
            <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded bg-blue-500/10 border border-blue-500/30 text-blue-400 flex items-center gap-1" title="Multiple call paths exist; displaying deterministic shortest path">
              <Layers3 className="w-3 h-3" /> MULTIPLE PATHS POSSIBLE
            </span>
          )}

          {flowResult.termination_reason === "TARGET_REACHED" && (
            <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 flex items-center gap-1">
              <CheckCircle2 className="w-3 h-3" /> TARGET REACHED
            </span>
          )}

          {flowResult.termination_reason === "DEPTH_LIMIT" && (
            <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded bg-amber-500/10 border border-amber-500/30 text-amber-400 flex items-center gap-1">
              <Layers className="w-3 h-3" /> DEPTH CEILING REACHED
            </span>
          )}
        </div>
      </div>

      {/* Vertical Call Pipeline Canvas */}
      <div className="flex-1 overflow-y-auto p-6 flex flex-col items-center space-y-0">
        {steps.map((step, idx) => (
          <FlowStepItem
            key={step.node_id + idx}
            step={step}
            isSelected={selectedStepIndex === idx}
            isLast={idx === steps.length - 1}
            onSelect={() => onSelectStep(idx)}
          />
        ))}
      </div>
    </div>
  );
};
