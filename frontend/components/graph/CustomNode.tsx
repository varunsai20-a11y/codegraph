"use client";

import React, { memo } from "react";
import { Handle, Position, NodeProps } from "@xyflow/react";
import { GraphNode } from "@/lib/types";
import { HardDrive, FileCode, Code2, Package } from "lucide-react";

export interface CustomNodeData {
  graphNode: GraphNode;
  isSelected?: boolean;
  isExpanded?: boolean;
}

export const CustomNode = memo(({ data }: NodeProps) => {
  const nodeData = data as unknown as CustomNodeData;
  const { graphNode, isSelected, isExpanded } = nodeData;

  const getStyleAndIcon = () => {
    switch (graphNode.kind) {
      case "NODE_REPOSITORY":
        return {
          bg: "bg-emerald-950/80 border-emerald-500/60 text-emerald-200",
          badge: "bg-emerald-500/20 text-emerald-300 border-emerald-500/40",
          icon: <HardDrive className="w-4 h-4 text-emerald-400 shrink-0" />,
          typeLabel: "REPO",
        };
      case "NODE_FILE":
        return {
          bg: "bg-blue-950/80 border-blue-500/60 text-blue-200",
          badge: "bg-blue-500/20 text-blue-300 border-blue-500/40",
          icon: <FileCode className="w-4 h-4 text-blue-400 shrink-0" />,
          typeLabel: "FILE",
        };
      case "NODE_SYMBOL":
        return {
          bg: "bg-purple-950/80 border-purple-500/60 text-purple-200",
          badge: "bg-purple-500/20 text-purple-300 border-purple-500/40",
          icon: <Code2 className="w-4 h-4 text-purple-400 shrink-0" />,
          typeLabel: "SYMBOL",
        };
      case "NODE_EXTERNAL_MODULE":
        return {
          bg: "bg-amber-950/80 border-amber-500/60 text-amber-200",
          badge: "bg-amber-500/20 text-amber-300 border-amber-500/40",
          icon: <Package className="w-4 h-4 text-amber-400 shrink-0" />,
          typeLabel: "EXTERNAL",
        };
      default:
        return {
          bg: "bg-surface border-border text-gray-200",
          badge: "bg-gray-500/20 text-gray-300 border-gray-500/40",
          icon: <FileCode className="w-4 h-4 text-gray-400 shrink-0" />,
          typeLabel: "NODE",
        };
    }
  };

  const style = getStyleAndIcon();

  return (
    <div
      className={`px-3 py-2 rounded-lg border shadow-lg backdrop-blur-md transition-all select-none min-w-[160px] max-w-[260px] ${
        style.bg
      } ${
        isSelected
          ? "ring-2 ring-accent ring-offset-2 ring-offset-background scale-105 z-10"
          : "hover:border-accent/80 hover:shadow-accent/10"
      }`}
    >
      <Handle type="target" position={Position.Top} className="!bg-accent !w-2 !h-2" />

      <div className="flex items-center space-x-2">
        {style.icon}
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-1">
            <span className="text-[9px] uppercase font-bold px-1.5 py-0.5 rounded border font-mono truncate" style={{ fontSize: "9px" }}>
              {style.typeLabel}
            </span>
            {isExpanded && (
              <span className="text-[9px] px-1 py-0.2 rounded bg-accent/20 border border-accent/40 text-accent font-mono">
                +EXPANDED
              </span>
            )}
          </div>
          <p className="text-xs font-semibold font-mono truncate mt-0.5" title={graphNode.label}>
            {graphNode.label}
          </p>
        </div>
      </div>

      <Handle type="source" position={Position.Bottom} className="!bg-accent !w-2 !h-2" />
    </div>
  );
});

CustomNode.displayName = "CustomNode";
