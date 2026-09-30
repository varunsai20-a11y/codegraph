"use client";

import React, { useMemo, useCallback, useState } from "react";
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  useNodesState,
  useEdgesState,
  Node,
  Edge,
  MarkerType,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import dagre from "@dagrejs/dagre";
import { GraphNode, GraphEdge } from "@/lib/types";
import { CustomNode, ModuleNode, calculateNodeDimensions } from "./CustomNode";
import { useAppStore } from "@/store/useAppStore";
import { Copy, Check, X, Code, Layers, GitFork } from "lucide-react";

interface GraphCanvasProps {
  nodes: GraphNode[];
  edges: GraphEdge[];
  selectedNodeID: string | null;
  expandedNodeIDs: Set<string>;
  onSelectNode: (nodeID: string | null) => void;
}

const nodeTypes = {
  customNode: CustomNode,
  moduleNode: ModuleNode,
};

export const GraphCanvas: React.FC<GraphCanvasProps> = ({
  nodes,
  edges,
  selectedNodeID,
  expandedNodeIDs,
  onSelectNode,
}) => {
  const {
    graphViewMode,
    setGraphViewMode,
    architectureDiagram,
  } = useAppStore();

  const [isMermaidModalOpen, setIsMermaidModalOpen] = useState(false);
  const [copiedMermaid, setCopiedMermaid] = useState(false);

  // Compute Layout for Detailed Call Graph
  const detailedLayout = useMemo(() => {
    if (graphViewMode !== "DETAILED") return { flowNodes: [], flowEdges: [] };

    const dagreGraph = new dagre.graphlib.Graph();
    dagreGraph.setDefaultEdgeLabel(() => ({}));
    dagreGraph.setGraph({ rankdir: "TB", nodesep: 60, ranksep: 90 });

    nodes.forEach((node) => {
      const dim = calculateNodeDimensions(node.label, node.kind, expandedNodeIDs.has(node.id));
      dagreGraph.setNode(node.id, { width: dim.width, height: dim.height });
    });

    edges.forEach((edge) => {
      dagreGraph.setEdge(edge.source_id, edge.target_id);
    });

    dagre.layout(dagreGraph);

    const connectedEdgeIDs = new Set<string>();
    const connectedNodeIDs = new Set<string>();

    if (selectedNodeID) {
      connectedNodeIDs.add(selectedNodeID);
      edges.forEach((e) => {
        if (e.source_id === selectedNodeID || e.target_id === selectedNodeID) {
          connectedEdgeIDs.add(e.id);
          connectedNodeIDs.add(e.source_id);
          connectedNodeIDs.add(e.target_id);
        }
      });
    }

    const hasSelection = selectedNodeID !== null;

    const layoutedNodes: Node[] = nodes.map((n) => {
      const pos = dagreGraph.node(n.id);
      const dim = calculateNodeDimensions(n.label, n.kind, expandedNodeIDs.has(n.id));

      const isSelected = selectedNodeID === n.id;
      const isConnectedNeighbor = hasSelection && !isSelected && connectedNodeIDs.has(n.id);
      const isDimmed = hasSelection && !connectedNodeIDs.has(n.id);

      return {
        id: n.id,
        type: "customNode",
        position: {
          x: pos ? pos.x - dim.width / 2 : Math.random() * 400,
          y: pos ? pos.y - dim.height / 2 : Math.random() * 400,
        },
        data: {
          graphNode: n,
          isSelected,
          isNeighbor: isConnectedNeighbor,
          isDimmed,
          isExpanded: expandedNodeIDs.has(n.id),
          width: dim.width,
          height: dim.height,
        },
      };
    });

    const layoutedEdges: Edge[] = edges.map((e) => {
      let strokeColor = "#64748b";
      if (e.kind === "EDGE_CALLS") strokeColor = "#a855f7";
      if (e.kind === "EDGE_IMPORTS") strokeColor = "#3b82f6";
      if (e.kind === "EDGE_CONTAINS") strokeColor = "#10b981";
      if (e.kind === "EDGE_EXTENDS" || e.kind === "EDGE_IMPLEMENTS") strokeColor = "#f59e0b";

      const isConnected = hasSelection && connectedEdgeIDs.has(e.id);
      const isDimmed = hasSelection && !isConnected;

      if (isConnected) {
        strokeColor = "#38bdf8";
      }

      return {
        id: e.id,
        source: e.source_id,
        target: e.target_id,
        label: e.kind.replace("EDGE_", ""),
        type: "smoothstep",
        animated: isConnected || e.kind === "EDGE_CALLS",
        style: {
          stroke: strokeColor,
          strokeWidth: isConnected ? 2.5 : 1.2,
          opacity: isDimmed ? 0.15 : 1.0,
          transition: "stroke-width 0.2s, opacity 0.2s",
        },
        markerEnd: {
          type: MarkerType.ArrowClosed,
          color: strokeColor,
          width: 14,
          height: 14,
        },
        labelStyle: { fill: isDimmed ? "#475569" : "#94a3b8", fontSize: 9, fontFamily: "monospace" },
        labelBgPadding: [4, 2],
        labelBgBorderRadius: 4,
        labelBgStyle: { fill: "#1e293b", fillOpacity: isDimmed ? 0.3 : 0.85 },
      };
    });

    return { flowNodes: layoutedNodes, flowEdges: layoutedEdges };
  }, [nodes, edges, selectedNodeID, expandedNodeIDs, graphViewMode]);

  // Compute Layout for GitDiagram Architecture Overview
  const architectureLayout = useMemo(() => {
    if (graphViewMode !== "ARCHITECTURE" || !architectureDiagram) {
      return { flowNodes: [], flowEdges: [] };
    }

    const modules = architectureDiagram.modules || [];
    const archEdges = architectureDiagram.edges || [];

    const dagreGraph = new dagre.graphlib.Graph();
    dagreGraph.setDefaultEdgeLabel(() => ({}));
    dagreGraph.setGraph({ rankdir: "TB", nodesep: 60, ranksep: 90 });

    modules.forEach((mod) => {
      dagreGraph.setNode(mod.id, { width: 240, height: 95 });
    });

    archEdges.forEach((e) => {
      dagreGraph.setEdge(e.source_module, e.target_module);
    });

    dagre.layout(dagreGraph);

    const layoutedNodes: Node[] = modules.map((m) => {
      const pos = dagreGraph.node(m.id);
      return {
        id: m.id,
        type: "moduleNode",
        position: {
          x: pos ? pos.x - 120 : Math.random() * 400,
          y: pos ? pos.y - 47.5 : Math.random() * 400,
        },
        data: {
          module: m,
          isSelected: selectedNodeID === m.id,
        },
      };
    });

    const layoutedEdges: Edge[] = archEdges.map((e) => {
      let strokeColor = "#38bdf8";
      if (e.kinds.includes("EDGE_CALLS")) strokeColor = "#a855f7";

      return {
        id: e.id,
        source: e.source_module,
        target: e.target_module,
        label: e.label,
        type: "smoothstep",
        animated: true,
        style: {
          stroke: strokeColor,
          strokeWidth: 2.0,
        },
        markerEnd: {
          type: MarkerType.ArrowClosed,
          color: strokeColor,
          width: 14,
          height: 14,
        },
        labelStyle: { fill: "#cbd5e1", fontSize: 10, fontFamily: "monospace", fontWeight: 600 },
        labelBgPadding: [6, 3],
        labelBgBorderRadius: 4,
        labelBgStyle: { fill: "#0f172a", fillOpacity: 0.9 },
      };
    });

    return { flowNodes: layoutedNodes, flowEdges: layoutedEdges };
  }, [architectureDiagram, graphViewMode, selectedNodeID]);

  const activeFlowNodes = graphViewMode === "ARCHITECTURE" ? architectureLayout.flowNodes : detailedLayout.flowNodes;
  const activeFlowEdges = graphViewMode === "ARCHITECTURE" ? architectureLayout.flowEdges : detailedLayout.flowEdges;

  const [rfNodes, setNodes, onNodesChange] = useNodesState(activeFlowNodes);
  const [rfEdges, setEdges, onEdgesChange] = useEdgesState(activeFlowEdges);

  React.useEffect(() => {
    setNodes(activeFlowNodes);
    setEdges(activeFlowEdges);
  }, [activeFlowNodes, activeFlowEdges, setNodes, setEdges]);

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      onSelectNode(node.id);
    },
    [onSelectNode]
  );

  const handlePaneClick = useCallback(() => {
    onSelectNode(null);
  }, [onSelectNode]);

  const handleCopyMermaid = () => {
    if (architectureDiagram?.mermaid_code) {
      navigator.clipboard.writeText(architectureDiagram.mermaid_code);
      setCopiedMermaid(true);
      setTimeout(() => setCopiedMermaid(false), 2000);
    }
  };

  return (
    <div className="w-full h-full bg-[#0b0f19] relative">
      {/* Top Floating Bar: View Toggle & Mermaid Action */}
      <div className="absolute top-4 left-4 z-10 flex flex-wrap items-center gap-2 bg-slate-900/90 backdrop-blur border border-slate-700/80 p-1.5 rounded-lg shadow-2xl">
        <div className="flex bg-slate-950 p-1 rounded-md border border-slate-800">
          <button
            onClick={() => setGraphViewMode("ARCHITECTURE")}
            className={`px-3 py-1.5 text-xs font-bold rounded-md transition flex items-center space-x-1.5 ${
              graphViewMode === "ARCHITECTURE"
                ? "bg-sky-600 text-white shadow-md"
                : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/60"
            }`}
          >
            <Layers className="w-3.5 h-3.5" />
            <span>Architecture Overview (GitDiagram)</span>
          </button>
          <button
            onClick={() => setGraphViewMode("DETAILED")}
            className={`px-3 py-1.5 text-xs font-bold rounded-md transition flex items-center space-x-1.5 ${
              graphViewMode === "DETAILED"
                ? "bg-sky-600 text-white shadow-md"
                : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/60"
            }`}
          >
            <GitFork className="w-3.5 h-3.5" />
            <span>Detailed Call Graph</span>
          </button>
        </div>

        {graphViewMode === "ARCHITECTURE" && (
          <button
            onClick={() => setIsMermaidModalOpen(true)}
            className="px-3 py-1.5 text-xs font-bold rounded-md bg-indigo-600/90 hover:bg-indigo-500 text-white transition flex items-center space-x-1.5 border border-indigo-500/40 shadow-sm"
          >
            <Code className="w-3.5 h-3.5" />
            <span>Copy / View Mermaid Diagram</span>
          </button>
        )}
      </div>

      <ReactFlow
        nodes={rfNodes}
        edges={rfEdges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={handleNodeClick}
        onPaneClick={handlePaneClick}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.35, includeHiddenNodes: false }}
        minZoom={0.15}
        maxZoom={2.0}
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#1e293b" gap={20} size={1} />
        <Controls className="!bg-surface !border-border !text-gray-300" />
        <MiniMap
          nodeColor={(n) => {
            const data = n.data as any;
            if (n.type === "moduleNode") {
              const cat = data?.module?.category;
              if (cat === "ENTRYPOINT") return "#10b981";
              if (cat === "STORAGE") return "#f59e0b";
              if (cat === "EXTERNAL") return "#a855f7";
              return "#38bdf8";
            }
            const kind = data?.graphNode?.kind;
            if (kind === "NODE_REPOSITORY") return "#10b981";
            if (kind === "NODE_FILE") return "#3b82f6";
            if (kind === "NODE_SYMBOL") return "#a855f7";
            if (kind === "NODE_EXTERNAL_MODULE") return "#f59e0b";
            return "#64748b";
          }}
          className="!bg-surface !border-border"
          maskColor="rgba(15, 23, 42, 0.7)"
        />
      </ReactFlow>

      {/* Mermaid.js Modal */}
      {isMermaidModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-sm">
          <div className="bg-slate-900 border border-slate-700 rounded-xl w-full max-w-3xl flex flex-col max-h-[85vh] shadow-2xl">
            <div className="flex items-center justify-between p-4 border-b border-slate-800">
              <div className="flex items-center space-x-2">
                <Code className="w-5 h-5 text-indigo-400" />
                <h3 className="text-sm font-bold text-slate-100">
                  System Architecture Mermaid.js Diagram
                </h3>
              </div>
              <button
                onClick={() => setIsMermaidModalOpen(false)}
                className="p-1 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="p-4 flex-1 overflow-y-auto">
              <pre className="text-xs font-mono bg-slate-950 p-4 rounded-lg border border-slate-800 text-indigo-200 overflow-x-auto whitespace-pre-wrap leading-relaxed">
                {architectureDiagram?.mermaid_code || "graph TD\n    empty[\"No diagram data\"]"}
              </pre>
            </div>

            <div className="p-4 border-t border-slate-800 flex items-center justify-between bg-slate-950/50">
              <span className="text-xs text-slate-400">
                Copy and paste into GitHub, Notion, or Mermaid Live Editor
              </span>
              <div className="flex space-x-2">
                <button
                  onClick={handleCopyMermaid}
                  className="px-4 py-2 text-xs font-bold rounded-lg bg-indigo-600 hover:bg-indigo-500 text-white transition flex items-center space-x-1.5 shadow-sm"
                >
                  {copiedMermaid ? (
                    <>
                      <Check className="w-4 h-4 text-emerald-300" />
                      <span>Copied!</span>
                    </>
                  ) : (
                    <>
                      <Copy className="w-4 h-4" />
                      <span>Copy Code</span>
                    </>
                  )}
                </button>
                <button
                  onClick={() => setIsMermaidModalOpen(false)}
                  className="px-4 py-2 text-xs font-bold rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 transition"
                >
                  Close
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
