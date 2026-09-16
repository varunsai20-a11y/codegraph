"use client";

import React from "react";
import { GraphNode, GraphEdge } from "@/lib/types";
import {
  Info,
  Maximize2,
  FolderTree,
  HardDrive,
  FileCode,
  Code2,
  Package,
  MapPin,
  ArrowUpRight,
  ArrowDownLeft,
  GitFork,
} from "lucide-react";

interface GraphInspectorProps {
  selectedNode: GraphNode | null;
  nodes: GraphNode[];
  edges: GraphEdge[];
  onExpandNode: (nodeID: string) => void;
  onViewInExplorer: (nodeID: string) => void;
  onTraceFlow?: (symbolID: string) => void;
  isLoading: boolean;
}

export const GraphInspector: React.FC<GraphInspectorProps> = ({
  selectedNode,
  edges,
  onExpandNode,
  onViewInExplorer,
  onTraceFlow,
  isLoading,
}) => {
  if (!selectedNode) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-6 text-center text-xs text-gray-400 select-none">
        <Info className="w-8 h-8 text-gray-500 mb-2" />
        <h4 className="font-semibold text-gray-300 mb-1">No Node Selected</h4>
        <p className="text-[11px] text-gray-400 max-w-xs">
          Click any node on the graph canvas to inspect its architecture properties, relationships, and neighborhood.
        </p>
      </div>
    );
  }

  const outgoingCount = edges.filter((e) => e.source_id === selectedNode.id).length;
  const incomingCount = edges.filter((e) => e.target_id === selectedNode.id).length;

  const getNodeIcon = () => {
    switch (selectedNode.kind) {
      case "NODE_REPOSITORY":
        return <HardDrive className="w-4 h-4 text-emerald-400 shrink-0" />;
      case "NODE_FILE":
        return <FileCode className="w-4 h-4 text-blue-400 shrink-0" />;
      case "NODE_SYMBOL":
        return <Code2 className="w-4 h-4 text-purple-400 shrink-0" />;
      case "NODE_EXTERNAL_MODULE":
        return <Package className="w-4 h-4 text-amber-400 shrink-0" />;
      default:
        return <FileCode className="w-4 h-4 text-gray-400 shrink-0" />;
    }
  };

  const isSymbol = selectedNode.kind === "NODE_SYMBOL";

  return (
    <div className="h-full flex flex-col justify-between p-4 bg-surface text-xs select-none overflow-y-auto space-y-4">
      <div className="space-y-4">
        {/* Node Header */}
        <div className="border-b border-border pb-3">
          <div className="flex items-center space-x-2">
            {getNodeIcon()}
            <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded bg-accent/15 border border-accent/30 text-accent font-mono">
              {selectedNode.kind.replace("NODE_", "")}
            </span>
          </div>
          <h3 className="text-sm font-bold text-gray-100 font-mono mt-2 break-all">
            {selectedNode.label}
          </h3>
          <p className="text-[11px] text-gray-400 font-mono truncate mt-0.5" title={selectedNode.qualified_name}>
            {selectedNode.qualified_name}
          </p>
        </div>

        {/* Property Grid */}
        <div className="space-y-2.5">
          <div className="bg-background border border-border p-2.5 rounded space-y-1">
            <span className="text-[10px] text-gray-400 uppercase font-semibold">Node ID</span>
            <p className="text-[11px] font-mono text-gray-300 truncate" title={selectedNode.id}>
              {selectedNode.id}
            </p>
          </div>

          {selectedNode.relative_path && (
            <div className="bg-background border border-border p-2.5 rounded space-y-1">
              <span className="text-[10px] text-gray-400 uppercase font-semibold flex items-center gap-1">
                <MapPin className="w-3 h-3 text-accent" /> Relative Path
              </span>
              <p className="text-[11px] font-mono text-gray-200 truncate" title={selectedNode.relative_path}>
                {selectedNode.relative_path}
              </p>
            </div>
          )}

          {selectedNode.location && (
            <div className="bg-background border border-border p-2.5 rounded space-y-1">
              <span className="text-[10px] text-gray-400 uppercase font-semibold">Source Location</span>
              <p className="text-[11px] font-mono text-gray-300">
                Line {selectedNode.location.start_line} - {selectedNode.location.end_line}
              </p>
            </div>
          )}

          {/* Relationship Counts */}
          <div className="grid grid-cols-2 gap-2">
            <div className="bg-background border border-border p-2.5 rounded space-y-0.5">
              <span className="text-[10px] text-gray-400 uppercase font-semibold flex items-center gap-1">
                <ArrowUpRight className="w-3 h-3 text-blue-400" /> Outgoing
              </span>
              <p className="text-sm font-bold text-white">{outgoingCount}</p>
            </div>
            <div className="bg-background border border-border p-2.5 rounded space-y-0.5">
              <span className="text-[10px] text-gray-400 uppercase font-semibold flex items-center gap-1">
                <ArrowDownLeft className="w-3 h-3 text-emerald-400" /> Incoming
              </span>
              <p className="text-sm font-bold text-white">{incomingCount}</p>
            </div>
          </div>
        </div>
      </div>

      {/* Action Buttons */}
      <div className="space-y-2 pt-3 border-t border-border">
        {isSymbol && onTraceFlow && (
          <button
            onClick={() => onTraceFlow(selectedNode.id)}
            className="w-full flex items-center justify-center space-x-2 px-3 py-2 rounded-md bg-purple-600/20 border border-purple-500/40 hover:bg-purple-600/30 text-purple-200 font-semibold text-xs transition"
          >
            <GitFork className="w-3.5 h-3.5 text-purple-400" />
            <span>Trace Static Flow</span>
          </button>
        )}

        <button
          onClick={() => onExpandNode(selectedNode.id)}
          disabled={isLoading}
          className="w-full flex items-center justify-center space-x-2 px-3 py-2 rounded-md bg-accent text-white font-semibold text-xs hover:bg-accent/90 transition disabled:opacity-50"
        >
          <Maximize2 className="w-3.5 h-3.5" />
          <span>Expand Neighborhood</span>
        </button>

        {selectedNode.relative_path && (
          <button
            onClick={() => onViewInExplorer(selectedNode.id)}
            className="w-full flex items-center justify-center space-x-2 px-3 py-2 rounded-md bg-background border border-border hover:bg-surface-hover text-gray-200 font-medium text-xs transition"
          >
            <FolderTree className="w-3.5 h-3.5 text-accent" />
            <span>{isSymbol ? "View Source" : "View in Explorer"}</span>
          </button>
        )}
      </div>
    </div>
  );
};
