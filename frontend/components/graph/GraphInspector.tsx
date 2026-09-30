"use client";

import React from "react";
import { GraphNode, GraphEdge } from "@/lib/types";
import { useAppStore } from "@/store/useAppStore";
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
  Bot,
  FileText,
  ShieldAlert,
  CheckCircle2,
  Clock,
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
  nodes,
  edges,
  onExpandNode,
  onViewInExplorer,
  onTraceFlow,
  isLoading,
}) => {
  const { activeRepo, fileManifest, fetchExplanation, setActiveTab, selectFileAndHighlight } = useAppStore();

  if (!selectedNode) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-6 text-center text-xs font-mono text-gray-400 select-none bg-surface/60 border-l border-border">
        <div className="w-9 h-9 rounded bg-background border border-border flex items-center justify-center text-cyan-400 mb-3">
          <Info className="w-4 h-4" />
        </div>
        <h4 className="font-bold text-gray-200 uppercase tracking-wider mb-1">INSPECTOR INACTIVE</h4>
        <p className="text-[11px] text-gray-500 max-w-xs leading-relaxed">
          Select any node or symbol on the graph canvas to inspect callers, callees, evidence, and groundings.
        </p>

        {activeRepo && (
          <div className="mt-6 p-3 bg-background border border-border rounded-sm w-full text-left space-y-1.5">
            <span className="text-[9px] uppercase font-bold text-gray-500 block">REPOSITORY SUMMARY</span>
            <div className="flex justify-between text-[11px]">
              <span className="text-gray-400">Target:</span>
              <span className="text-gray-200 font-bold">{activeRepo.name}</span>
            </div>
            <div className="flex justify-between text-[11px]">
              <span className="text-gray-400">Status:</span>
              <span className="text-emerald-400 font-bold">{activeRepo.status}</span>
            </div>
            <div className="flex justify-between text-[11px]">
              <span className="text-gray-400">Indexed Files:</span>
              <span className="text-cyan-300 font-bold">{fileManifest.length}</span>
            </div>
          </div>
        )}
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
        return <Code2 className="w-4 h-4 text-cyan-400 shrink-0" />;
      case "NODE_EXTERNAL_MODULE":
        return <Package className="w-4 h-4 text-amber-400 shrink-0" />;
      default:
        return <FileCode className="w-4 h-4 text-gray-400 shrink-0" />;
    }
  };

  const isSymbol = selectedNode.kind === "NODE_SYMBOL";
  const isFile = selectedNode.kind === "NODE_FILE";
  const isRepo = selectedNode.kind === "NODE_REPOSITORY";

  const handleAskAIAboutNode = () => {
    const q = isSymbol
      ? `Explain the implementation, callers, callees, and dependencies for symbol ${selectedNode.label}.`
      : `Explain the architectural responsibilities and key symbols of file ${selectedNode.relative_path || selectedNode.label}.`;
    fetchExplanation(q, { symbolId: selectedNode.id });
    setActiveTab("AI");
  };

  const handleOpenSource = () => {
    if (selectedNode.relative_path) {
      const line = selectedNode.location?.start_line || 1;
      selectFileAndHighlight(selectedNode.relative_path, line);
      setActiveTab("SYNC");
    }
  };

  return (
    <div className="h-full flex flex-col justify-between p-3.5 bg-surface text-xs font-mono select-none overflow-y-auto space-y-4">
      <div className="space-y-3.5">
        {/* Node Header */}
        <div className="border-b border-border pb-3">
          <div className="flex items-center space-x-2">
            {getNodeIcon()}
            <span className="text-[9px] uppercase font-bold tracking-wider px-1.5 py-0.5 rounded-sm bg-cyan-950/70 border border-cyan-500/40 text-cyan-300">
              {selectedNode.kind.replace("NODE_", "")}
            </span>
          </div>
          <h3 className="text-xs font-bold text-gray-100 mt-2 break-all font-mono">
            {selectedNode.label}
          </h3>
          <p className="text-[10px] text-gray-500 font-mono truncate mt-0.5" title={selectedNode.qualified_name}>
            {selectedNode.qualified_name}
          </p>
        </div>

        {/* Details Cards */}
        <div className="space-y-2">
          <div className="bg-background border border-border p-2 rounded-sm space-y-0.5">
            <span className="text-[9px] text-gray-500 uppercase font-bold">STABLE ID</span>
            <p className="text-[10px] font-mono text-gray-300 truncate" title={selectedNode.id}>
              {selectedNode.id}
            </p>
          </div>

          {selectedNode.relative_path && (
            <div className="bg-background border border-border p-2 rounded-sm space-y-0.5">
              <span className="text-[9px] text-gray-500 uppercase font-bold flex items-center gap-1">
                <MapPin className="w-3 h-3 text-cyan-400" /> SOURCE FILE
              </span>
              <p className="text-[10px] font-mono text-cyan-300 truncate" title={selectedNode.relative_path}>
                {selectedNode.relative_path}
              </p>
            </div>
          )}

          {selectedNode.location && selectedNode.location.start_line > 0 ? (
            <div className="bg-background border border-border p-2 rounded-sm space-y-0.5">
              <span className="text-[9px] text-gray-500 uppercase font-bold">AST LOCATION</span>
              <p className="text-[10px] font-mono text-gray-300">
                Lines {selectedNode.location.start_line}–{selectedNode.location.end_line}
              </p>
            </div>
          ) : !selectedNode.relative_path && !isRepo ? (
            <div className="bg-amber-950/30 border border-amber-500/30 p-2 rounded-sm text-[10px] text-amber-300">
              External / Virtual Node — AST Source Unavailable
            </div>
          ) : null}

          {/* Incoming & Outgoing Dependencies */}
          <div className="space-y-2 pt-1">
            <div className="bg-background border border-border p-2 rounded-sm space-y-1">
              <span className="text-[9px] text-gray-400 uppercase font-bold flex items-center gap-1">
                <ArrowUpRight className="w-3 h-3 text-cyan-400" /> OUTGOING CALLS / DEPS ({outgoingCount})
              </span>
              {outgoingCount === 0 ? (
                <p className="text-[10px] text-gray-600 italic">No outgoing connections in current view</p>
              ) : (
                <div className="space-y-1 max-h-28 overflow-y-auto pr-1">
                  {edges
                    .filter((e) => e.source_id === selectedNode.id)
                    .map((e) => {
                      const targetNode = nodes.find((n) => n.id === e.target_id);
                      return (
                        <button
                          key={e.id}
                          onClick={() => useAppStore.getState().selectGraphNode(e.target_id)}
                          className="w-full text-left flex items-center justify-between px-1.5 py-1 rounded-sm bg-surface hover:bg-surface-hover border border-border transition text-[10px] text-gray-300 font-mono"
                        >
                          <span className="truncate">{targetNode?.label || e.target_id}</span>
                          <span className="text-[8px] uppercase font-bold px-1 rounded-sm bg-cyan-950 text-cyan-300 shrink-0">
                            {e.kind.replace("EDGE_", "")}
                          </span>
                        </button>
                      );
                    })}
                </div>
              )}
            </div>

            <div className="bg-background border border-border p-2 rounded-sm space-y-1">
              <span className="text-[9px] text-gray-400 uppercase font-bold flex items-center gap-1">
                <ArrowDownLeft className="w-3 h-3 text-emerald-400" /> INCOMING CALLERS / DEPENDENTS ({incomingCount})
              </span>
              {incomingCount === 0 ? (
                <p className="text-[10px] text-gray-600 italic">No incoming connections in current view</p>
              ) : (
                <div className="space-y-1 max-h-28 overflow-y-auto pr-1">
                  {edges
                    .filter((e) => e.target_id === selectedNode.id)
                    .map((e) => {
                      const sourceNode = nodes.find((n) => n.id === e.source_id);
                      return (
                        <button
                          key={e.id}
                          onClick={() => useAppStore.getState().selectGraphNode(e.source_id)}
                          className="w-full text-left flex items-center justify-between px-1.5 py-1 rounded-sm bg-surface hover:bg-surface-hover border border-border transition text-[10px] text-gray-300 font-mono"
                        >
                          <span className="truncate">{sourceNode?.label || e.source_id}</span>
                          <span className="text-[8px] uppercase font-bold px-1 rounded-sm bg-emerald-950 text-emerald-300 shrink-0">
                            {e.kind.replace("EDGE_", "")}
                          </span>
                        </button>
                      );
                    })}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Action Buttons */}
      <div className="space-y-1.5 pt-2 border-t border-border">
        {selectedNode.relative_path && (
          <button
            onClick={handleOpenSource}
            className="w-full flex items-center justify-center space-x-1.5 px-2.5 py-1.5 rounded-sm bg-background border border-border hover:bg-surface-hover text-gray-200 font-bold transition text-[11px]"
          >
            <FileText className="w-3.5 h-3.5 text-cyan-400" />
            <span>Open Source View</span>
          </button>
        )}

        <button
          onClick={handleAskAIAboutNode}
          className="w-full flex items-center justify-center space-x-1.5 px-2.5 py-1.5 rounded-sm bg-violet-950/50 border border-violet-500/40 hover:bg-violet-900/60 text-violet-200 font-bold transition text-[11px]"
        >
          <Bot className="w-3.5 h-3.5 text-violet-400" />
          <span>Ask AI About This</span>
        </button>

        {isSymbol && onTraceFlow && (
          <button
            onClick={() => onTraceFlow(selectedNode.id)}
            className="w-full flex items-center justify-center space-x-1.5 px-2.5 py-1.5 rounded-sm bg-cyan-950/50 border border-cyan-500/40 hover:bg-cyan-900/60 text-cyan-200 font-bold transition text-[11px]"
          >
            <GitFork className="w-3.5 h-3.5 text-cyan-400" />
            <span>Trace Call Flow</span>
          </button>
        )}

        <button
          onClick={() => onExpandNode(selectedNode.id)}
          disabled={isLoading}
          className="w-full flex items-center justify-center space-x-1.5 px-2.5 py-1.5 rounded-sm bg-cyan-600 hover:bg-cyan-500 text-white font-bold transition disabled:opacity-50 text-[11px]"
        >
          <Maximize2 className="w-3.5 h-3.5" />
          <span>Expand Neighborhood</span>
        </button>
      </div>
    </div>
  );
};
