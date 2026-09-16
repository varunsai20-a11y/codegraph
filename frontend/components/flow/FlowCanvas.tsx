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
  selectedStepIndex: number | null;
  onSelectStep: (index: number) => void;
  isLoading: boolean;
}

export const FlowCanvas: React.FC<FlowCanvasProps> = ({
  flowResult,
  selectedStepIndex,
  onSelectStep,
  isLoading,
}) => {
  if (isLoading) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background">
        <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin mb-3" />
        <p className="text-xs text-gray-400 font-mono">Computing static call flow...</p>
      </div>
    );
  }

  if (!flowResult) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
        <GitFork className="w-10 h-10 text-gray-500 mb-3" />
        <h3 className="text-sm font-bold text-gray-200">No Static Call Flow Loaded</h3>
        <p className="text-xs text-gray-400 max-w-sm mt-1">
          Select a symbol from the Knowledge Graph or Repository Explorer and click &quot;Trace Static Flow&quot; to inspect its static call path.
        </p>
      </div>
    );
  }

  if (flowResult.termination_reason === "INVALID_ROOT") {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
        <div className="w-12 h-12 rounded-full bg-red-500/10 border border-red-500/30 flex items-center justify-center text-red-400 mb-3">
          <AlertTriangle className="w-6 h-6" />
        </div>
        <h3 className="text-sm font-bold text-red-300">Invalid Static Flow Root</h3>
        <p className="text-xs text-red-400/90 max-w-md mt-1">
          Static call flow requires a valid symbol root (<code className="font-mono text-white">NODE_SYMBOL</code>). Selected target is not a valid executable symbol in the graph.
        </p>
      </div>
    );
  }

  if (flowResult.termination_reason === "TARGET_NOT_FOUND") {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
        <div className="w-12 h-12 rounded-full bg-amber-500/10 border border-amber-500/30 flex items-center justify-center text-amber-400 mb-3">
          <HelpCircle className="w-6 h-6" />
        </div>
        <h3 className="text-sm font-bold text-amber-300">Target Node Not Found</h3>
        <p className="text-xs text-amber-400/90 max-w-md mt-1">
          Target symbol <code className="font-mono text-white">{flowResult.target_node_id}</code> does not exist in the repository graph.
        </p>
      </div>
    );
  }

  if (flowResult.termination_reason === "NO_PATH") {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
        <div className="w-12 h-12 rounded-full bg-amber-500/10 border border-amber-500/30 flex items-center justify-center text-amber-400 mb-3">
          <GitFork className="w-6 h-6 rotate-180" />
        </div>
        <h3 className="text-sm font-bold text-amber-300">No Directed Call Path Found</h3>
        <p className="text-xs text-amber-400/90 max-w-md mt-1">
          No static <code className="font-mono text-white">EDGE_CALLS</code> path connects root symbol to target symbol within depth limit ({flowResult.max_depth}).
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
