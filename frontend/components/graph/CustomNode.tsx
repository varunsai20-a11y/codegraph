"use client";

import React, { memo } from "react";
import { Handle, Position, NodeProps } from "@xyflow/react";
import { GraphNode } from "@/lib/types";
import { HardDrive, FileCode, Code2, Package, Layers, ExternalLink } from "lucide-react";

export interface CustomNodeData {
  graphNode: GraphNode;
  isSelected?: boolean;
  isNeighbor?: boolean;
  isDimmed?: boolean;
  isExpanded?: boolean;
  width?: number;
  height?: number;
}

export function calculateNodeDimensions(label: string, kind?: string, isExpanded?: boolean) {
  const charWidth = 8.5;
  const basePadding = 84;
  const calculatedWidth = Math.round((label || "").length * charWidth + basePadding);
  const width = Math.max(195, Math.min(295, calculatedWidth));
  const height = isExpanded ? 78 : 68;
  return { width, height };
}

export const CustomNode = memo(({ data }: NodeProps) => {
  const nodeData = data as unknown as CustomNodeData;
  const { graphNode, isSelected, isNeighbor, isDimmed, isExpanded, width, height } = nodeData;

  const getStyleAndIcon = () => {
    switch (graphNode.kind) {
      case "NODE_REPOSITORY":
        return {
          bg: "bg-surface border-border text-gray-200",
          badge: "bg-background text-gray-400 border-border",
          icon: <HardDrive className="w-3.5 h-3.5 text-gray-400 shrink-0" />,
          typeLabel: "REPO",
          accentLine: "border-l-2 border-l-gray-400",
        };
      case "NODE_FILE":
        return {
          bg: "bg-surface border-border text-gray-200",
          badge: "bg-background text-blue-400 border-blue-500/30",
          icon: <FileCode className="w-3.5 h-3.5 text-blue-400 shrink-0" />,
          typeLabel: "FILE",
          accentLine: "border-l-2 border-l-blue-400",
        };
      case "NODE_SYMBOL":
        return {
          bg: "bg-cyan-950/40 border-cyan-500/50 text-cyan-100",
          badge: "bg-cyan-950/70 text-cyan-300 border-cyan-500/50",
          icon: <Code2 className="w-3.5 h-3.5 text-cyan-400 shrink-0" />,
          typeLabel: "SYMBOL",
          accentLine: "border-l-2 border-l-cyan-400",
        };
      case "NODE_EXTERNAL_MODULE":
        return {
          bg: "bg-amber-950/25 border-amber-500/40 text-amber-100",
          badge: "bg-amber-950/60 text-amber-300 border-amber-500/40",
          icon: <Package className="w-3.5 h-3.5 text-amber-400 shrink-0" />,
          typeLabel: "EXTERNAL",
          accentLine: "border-l-2 border-l-amber-400",
        };
      default:
        return {
          bg: "bg-surface border-border text-gray-300",
          badge: "bg-background text-gray-400 border-border",
          icon: <FileCode className="w-3.5 h-3.5 text-gray-400 shrink-0" />,
          typeLabel: "NODE",
          accentLine: "border-l-2 border-l-gray-500",
        };
    }
  };

  const style = getStyleAndIcon();

  const selectionClass = isSelected
    ? "ring-1 ring-cyan-400 border-cyan-400 scale-[1.03] z-30 shadow-[0_0_15px_rgba(6,182,212,0.25)]"
    : isNeighbor
    ? "border-cyan-500/70 scale-[1.01] z-20"
    : isDimmed
    ? "opacity-25 grayscale-[40%] transition-all duration-150 z-0"
    : "hover:border-cyan-500/60 hover:scale-[1.01] transition-all duration-150";

  return (
    <div
      style={{
        width: width ? `${width}px` : undefined,
        height: height ? `${height}px` : undefined,
      }}
      className={`px-3 py-2 rounded-sm border select-none box-border flex flex-col justify-between relative group font-mono ${
        style.bg
      } ${style.accentLine} ${selectionClass}`}
    >
      <Handle
        type="target"
        position={Position.Top}
        className="!bg-cyan-400 !w-2 !h-2 !border-2 !border-background shadow-sm"
      />

      <div className="flex items-center space-x-2.5 min-w-0 flex-1">
        <div className="p-1 rounded-sm bg-background border border-border shrink-0">
          {style.icon}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-1">
            <span
              className={`text-[8px] uppercase font-bold font-mono tracking-wider px-1 py-0.2 rounded-sm border truncate ${style.badge}`}
            >
              {style.typeLabel}
            </span>
            {isExpanded && (
              <span className="text-[8px] px-1 py-0.2 rounded-sm bg-cyan-950/70 border border-cyan-500/40 text-cyan-300 font-mono">
                +NEIGHBORS
              </span>
            )}
          </div>
          <p
            className="text-xs font-bold font-mono truncate mt-1 text-gray-100 group-hover:text-cyan-300 transition-colors"
            title={graphNode.label}
          >
            {graphNode.label}
          </p>
        </div>
      </div>

      <Handle
        type="source"
        position={Position.Bottom}
        className="!bg-cyan-400 !w-2 !h-2 !border-2 !border-background shadow-sm"
      />
    </div>
  );
});

CustomNode.displayName = "CustomNode";

export interface ModuleNodeData {
  module: {
    id: string;
    name: string;
    path: string;
    category: string;
    file_count: number;
    symbol_count: number;
  };
  isSelected?: boolean;
}

export const ModuleNode = memo(({ data }: NodeProps) => {
  const nodeData = data as unknown as ModuleNodeData;
  const { module, isSelected } = nodeData;

  const getCategoryStyles = () => {
    switch (module.category) {
      case "ENTRYPOINT":
        return {
          bg: "bg-emerald-950/30 border-emerald-500/50 text-emerald-100",
          badge: "bg-emerald-950/70 text-emerald-300 border-emerald-500/40",
          icon: <HardDrive className="w-3.5 h-3.5 text-emerald-400 shrink-0" />,
          categoryLabel: "ENTRYPOINT",
          borderLeft: "border-l-2 border-l-emerald-400",
        };
      case "STORAGE":
        return {
          bg: "bg-amber-950/25 border-amber-500/40 text-amber-100",
          badge: "bg-amber-950/60 text-amber-300 border-amber-500/40",
          icon: <Package className="w-3.5 h-3.5 text-amber-400 shrink-0" />,
          categoryLabel: "STORAGE",
          borderLeft: "border-l-2 border-l-amber-400",
        };
      case "EXTERNAL":
        return {
          bg: "bg-violet-950/25 border-violet-500/40 text-violet-100",
          badge: "bg-violet-950/60 text-violet-300 border-violet-500/40",
          icon: <Package className="w-3.5 h-3.5 text-violet-400 shrink-0" />,
          categoryLabel: "EXTERNAL",
          borderLeft: "border-l-2 border-l-violet-400",
        };
      default:
        return {
          bg: "bg-cyan-950/30 border-cyan-500/50 text-cyan-100",
          badge: "bg-cyan-950/70 text-cyan-300 border-cyan-500/40",
          icon: <Layers className="w-3.5 h-3.5 text-cyan-400 shrink-0" />,
          categoryLabel: "MODULE",
          borderLeft: "border-l-2 border-l-cyan-400",
        };
    }
  };

  const style = getCategoryStyles();

  return (
    <div
      style={{
        width: "240px",
        minHeight: "85px",
      }}
      className={`px-3 py-2.5 rounded-sm border select-none box-border flex flex-col justify-between relative group font-mono ${
        style.bg
      } ${style.borderLeft} ${
        isSelected
          ? "ring-1 ring-cyan-400 border-cyan-400 scale-[1.02] shadow-[0_0_15px_rgba(6,182,212,0.25)]"
          : "hover:border-cyan-400/70 transition-all"
      }`}
    >
      <Handle
        type="target"
        position={Position.Top}
        className="!bg-cyan-400 !w-2 !h-2 !border-2 !border-background shadow-sm"
      />

      <div className="flex items-center space-x-2.5 min-w-0">
        <div className="p-1 rounded-sm bg-background border border-border shrink-0">
          {style.icon}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-1">
            <span
              className={`text-[8px] uppercase font-bold tracking-wider px-1 py-0.2 rounded-sm border font-mono truncate ${style.badge}`}
            >
              {style.categoryLabel}
            </span>
          </div>
          <p
            className="text-xs font-bold font-mono truncate mt-1 text-gray-100 group-hover:text-cyan-300 transition-colors"
            title={module.name}
          >
            {module.name}
          </p>
        </div>
      </div>

      <div className="flex items-center justify-between mt-2 pt-1.5 border-t border-border text-[9px] text-gray-400 font-mono">
        <span>
          Files: <strong className="text-gray-200">{module.file_count}</strong>
        </span>
        <span>
          Symbols: <strong className="text-gray-200">{module.symbol_count}</strong>
        </span>
      </div>

      <Handle
        type="source"
        position={Position.Bottom}
        className="!bg-cyan-400 !w-2 !h-2 !border-2 !border-background shadow-sm"
      />
    </div>
  );
});

ModuleNode.displayName = "ModuleNode";
