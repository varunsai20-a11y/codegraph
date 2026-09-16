"use client";

import React from "react";
import { FlowStep } from "@/lib/types";
import {
  Info,
  MapPin,
  FolderTree,
  Network,
  Code2,
  Package,
  FileCode,
  ArrowRight,
  ShieldCheck,
  HelpCircle,
  Bot,
} from "lucide-react";
import { useAppStore } from "@/store/useAppStore";

interface FlowInspectorProps {
  selectedStep: FlowStep | null;
  onViewSource: (nodeID: string) => void;
  onViewInGraph: (nodeID: string) => void;
}

export const FlowInspector: React.FC<FlowInspectorProps> = ({
  selectedStep,
  onViewSource,
  onViewInGraph,
}) => {
  if (!selectedStep) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-6 text-center text-xs text-gray-400 select-none">
        <Info className="w-8 h-8 text-gray-500 mb-2" />
        <h4 className="font-semibold text-gray-300 mb-1">No Flow Step Selected</h4>
        <p className="text-[11px] text-gray-400 max-w-xs">
          Click any step in the static call pipeline to inspect its symbol properties, source location evidence, and call edge status.
        </p>
      </div>
    );
  }

  const node = selectedStep.node;
  const isExternal = node?.kind === "NODE_EXTERNAL_MODULE";
  const edge = selectedStep.outgoing_edge || selectedStep.incoming_edge;

  const getNodeIcon = () => {
    if (isExternal) return <Package className="w-4 h-4 text-amber-400 shrink-0" />;
    if (node?.kind === "NODE_SYMBOL") return <Code2 className="w-4 h-4 text-purple-400 shrink-0" />;
    return <FileCode className="w-4 h-4 text-blue-400 shrink-0" />;
  };

  return (
    <div className="h-full flex flex-col justify-between p-4 bg-surface text-xs select-none overflow-y-auto space-y-4">
      <div className="space-y-4">
        {/* Step Header */}
        <div className="border-b border-border pb-3">
          <div className="flex items-center space-x-2">
            <span className="px-2 py-0.5 rounded bg-accent/20 border border-accent/40 text-accent font-mono font-bold text-[10px]">
              STEP #{selectedStep.sequence}
            </span>
            {getNodeIcon()}
            <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded bg-surface border border-border text-gray-300 font-mono">
              {node?.kind ? node.kind.replace("NODE_", "") : "SYMBOL"}
            </span>
          </div>
          <h3 className="text-sm font-bold text-gray-100 font-mono mt-2 break-all">
            {node?.label || selectedStep.node_id}
          </h3>
          <p className="text-[11px] text-gray-400 font-mono truncate mt-0.5" title={node?.qualified_name}>
            {node?.qualified_name || selectedStep.node_id}
          </p>
        </div>

        {/* Property Grid */}
        <div className="space-y-2.5">
          <div className="bg-background border border-border p-2.5 rounded space-y-1">
            <span className="text-[10px] text-gray-400 uppercase font-semibold">Node ID</span>
            <p className="text-[11px] font-mono text-gray-300 truncate" title={selectedStep.node_id}>
              {selectedStep.node_id}
            </p>
          </div>

          {node?.relative_path && (
            <div className="bg-background border border-border p-2.5 rounded space-y-1">
              <span className="text-[10px] text-gray-400 uppercase font-semibold flex items-center gap-1">
                <MapPin className="w-3 h-3 text-accent" /> Relative Path
              </span>
              <p className="text-[11px] font-mono text-gray-200 truncate" title={node.relative_path}>
                {node.relative_path}
              </p>
            </div>
          )}

          {node?.location && node.location.start_line > 0 && (
            <div className="bg-background border border-border p-2.5 rounded space-y-1">
              <span className="text-[10px] text-gray-400 uppercase font-semibold">Source Location</span>
              <p className="text-[11px] font-mono text-accent font-semibold">
                Lines {node.location.start_line} – {node.location.end_line}
              </p>
            </div>
          )}

          {/* Call Edge Evidence */}
          {edge && (
            <div className="bg-background border border-border p-2.5 rounded space-y-1.5">
              <span className="text-[10px] text-gray-400 uppercase font-semibold flex items-center gap-1">
                <ArrowRight className="w-3 h-3 text-accent" /> Call Edge Evidence
              </span>
              <div className="text-[11px] font-mono space-y-1 text-gray-300">
                <div className="flex justify-between">
                  <span className="text-gray-500">Kind:</span>
                  <span className="text-white font-bold">{edge.kind}</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-gray-500">Resolution:</span>
                  <span className={`font-bold flex items-center gap-1 ${
                    edge.status === "RESOLVED" ? "text-emerald-400" : "text-amber-400"
                  }`}>
                    {edge.status === "RESOLVED" ? <ShieldCheck className="w-3 h-3" /> : <HelpCircle className="w-3 h-3" />}
                    {edge.status || "RESOLVED"}
                  </span>
                </div>
                {edge.id && (
                  <div className="text-[10px] text-gray-500 truncate" title={edge.id}>
                    Edge ID: {edge.id}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Action Buttons */}
      <div className="space-y-2 pt-3 border-t border-border">
        <button
          onClick={() => {
            const store = useAppStore.getState();
            if (store.flowResult) {
              store.fetchExplanation(
                `Explain flow step #${selectedStep.sequence}: ${selectedStep.node?.label || selectedStep.node_id}`,
                {
                  rootSymbol: store.flowResult.root_node_id,
                  targetNode: store.flowResult.target_node_id,
                  flow: true,
                }
              );
              store.setActiveTab("AI");
            }
          }}
          className="w-full flex items-center justify-center space-x-2 px-3 py-2 rounded-md bg-sky-600/20 border border-sky-500/40 hover:bg-sky-600/30 text-sky-200 font-semibold text-xs transition"
        >
          <Bot className="w-3.5 h-3.5 text-sky-400" />
          <span>Explain Flow Step with AI</span>
        </button>

        {node?.relative_path && (
          <button
            onClick={() => onViewSource(selectedStep.node_id)}
            className="w-full flex items-center justify-center space-x-2 px-3 py-2 rounded-md bg-accent text-white font-semibold text-xs hover:bg-accent/90 transition"
          >
            <FolderTree className="w-3.5 h-3.5" />
            <span>View Source Code</span>
          </button>
        )}

        <button
          onClick={() => onViewInGraph(selectedStep.node_id)}
          className="w-full flex items-center justify-center space-x-2 px-3 py-2 rounded-md bg-background border border-border hover:bg-surface-hover text-gray-200 font-medium text-xs transition"
        >
          <Network className="w-3.5 h-3.5 text-accent" />
          <span>View in Graph Canvas</span>
        </button>
      </div>
    </div>
  );
};
