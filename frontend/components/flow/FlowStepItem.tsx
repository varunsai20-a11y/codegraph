"use client";

import React from "react";
import { FlowStep } from "@/lib/types";
import {
  Code2,
  Package,
  FileCode,
  ArrowDown,
  AlertCircle,
  HelpCircle,
  MapPin,
} from "lucide-react";

interface FlowStepItemProps {
  step: FlowStep;
  isSelected: boolean;
  isLast: boolean;
  onSelect: () => void;
}

export const FlowStepItem: React.FC<FlowStepItemProps> = ({
  step,
  isSelected,
  isLast,
  onSelect,
}) => {
  const node = step.node;
  const isExternal = node?.kind === "NODE_EXTERNAL_MODULE";
  const isUnresolved =
    step.incoming_edge?.status === "UNRESOLVED" ||
    step.outgoing_edge?.status === "UNRESOLVED";

  const getNodeIcon = () => {
    if (isExternal) return <Package className="w-4 h-4 text-amber-400 shrink-0" />;
    if (node?.kind === "NODE_SYMBOL") return <Code2 className="w-4 h-4 text-purple-400 shrink-0" />;
    return <FileCode className="w-4 h-4 text-blue-400 shrink-0" />;
  };

  return (
    <div className="flex flex-col items-center w-full max-w-xl select-none">
      {/* Step Card */}
      <div
        onClick={onSelect}
        className={`w-full p-3.5 rounded-lg border transition cursor-pointer flex items-start justify-between space-x-3 ${
          isSelected
            ? "bg-accent/15 border-accent shadow-md shadow-accent/10"
            : "bg-surface border-border hover:bg-surface-hover hover:border-border/80"
        }`}
      >
        <div className="flex items-start space-x-3 min-w-0 flex-1">
          {/* Sequence badge */}
          <span className="w-6 h-6 rounded-full bg-background border border-border text-[11px] font-bold font-mono text-gray-300 flex items-center justify-center shrink-0 mt-0.5">
            {step.sequence}
          </span>

          <div className="min-w-0 flex-1">
            <div className="flex items-center space-x-2">
              {getNodeIcon()}
              <h4 className="text-xs font-bold text-gray-100 font-mono truncate">
                {node?.label || step.node_id}
              </h4>

              {isExternal && (
                <span className="text-[9px] uppercase font-bold tracking-wider px-1.5 py-0.2 bg-amber-500/10 border border-amber-500/30 text-amber-400 rounded">
                  EXTERNAL MODULE
                </span>
              )}

              {isUnresolved && (
                <span className="text-[9px] uppercase font-bold tracking-wider px-1.5 py-0.2 bg-red-500/10 border border-red-500/30 text-red-400 rounded flex items-center gap-1">
                  <HelpCircle className="w-2.5 h-2.5" /> UNRESOLVED TARGET
                </span>
              )}
            </div>

            <p className="text-[11px] text-gray-400 font-mono truncate mt-0.5" title={node?.qualified_name}>
              {node?.qualified_name || step.node_id}
            </p>

            {node?.relative_path && (
              <div className="flex items-center space-x-2 text-[10px] text-gray-400 font-mono mt-1">
                <span className="flex items-center gap-1 text-gray-300 truncate">
                  <MapPin className="w-3 h-3 text-accent shrink-0" />
                  <span className="truncate">{node.relative_path}</span>
                </span>
                {node.location && node.location.start_line > 0 && (
                  <span className="text-accent font-semibold px-1 rounded bg-accent/10 border border-accent/20">
                    L{node.location.start_line}–{node.location.end_line}
                  </span>
                )}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Downward CALLS Arrow */}
      {!isLast && (
        <div className="py-2 flex flex-col items-center select-none text-gray-500">
          <div className="h-4 w-0.5 bg-border" />
          <div className="flex items-center space-x-1.5 px-2 py-0.5 rounded bg-surface border border-border/60 text-[10px] font-mono text-gray-400">
            <span className="text-accent font-bold">CALLS</span>
            {step.outgoing_edge?.file_id && step.outgoing_edge.location?.start_line ? (
              <span className="text-gray-400">L{step.outgoing_edge.location.start_line}</span>
            ) : null}
          </div>
          <ArrowDown className="w-4 h-4 text-accent mt-0.5" />
        </div>
      )}
    </div>
  );
};
