"use client";

import React, { useMemo, useCallback } from "react";
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  useNodesState,
  useEdgesState,
  Node,
  Edge,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import dagre from "@dagrejs/dagre";
import { GraphNode, GraphEdge } from "@/lib/types";
import { CustomNode } from "./CustomNode";

interface GraphCanvasProps {
  nodes: GraphNode[];
  edges: GraphEdge[];
  selectedNodeID: string | null;
  expandedNodeIDs: Set<string>;
  onSelectNode: (nodeID: string | null) => void;
}

const nodeTypes = {
  customNode: CustomNode,
};

export const GraphCanvas: React.FC<GraphCanvasProps> = ({
  nodes,
  edges,
  selectedNodeID,
  expandedNodeIDs,
  onSelectNode,
}) => {
  const { flowNodes, flowEdges } = useMemo(() => {
    const dagreGraph = new dagre.graphlib.Graph();
    dagreGraph.setDefaultEdgeLabel(() => ({}));
    dagreGraph.setGraph({ rankdir: "TB", nodesep: 60, ranksep: 80 });

    nodes.forEach((node) => {
      dagreGraph.setNode(node.id, { width: 220, height: 70 });
    });

    edges.forEach((edge) => {
      dagreGraph.setEdge(edge.source_id, edge.target_id);
    });

    dagre.layout(dagreGraph);

    const layoutedNodes: Node[] = nodes.map((n) => {
      const pos = dagreGraph.node(n.id);
      return {
        id: n.id,
        type: "customNode",
        position: {
          x: pos ? pos.x - 110 : Math.random() * 400,
          y: pos ? pos.y - 35 : Math.random() * 400,
        },
        data: {
          graphNode: n,
          isSelected: selectedNodeID === n.id,
          isExpanded: expandedNodeIDs.has(n.id),
        },
      };
    });

    const layoutedEdges: Edge[] = edges.map((e) => {
      let strokeColor = "#64748b";
      if (e.kind === "EDGE_CALLS") strokeColor = "#a855f7";
      if (e.kind === "EDGE_IMPORTS") strokeColor = "#3b82f6";
      if (e.kind === "EDGE_CONTAINS") strokeColor = "#10b981";

      return {
        id: e.id,
        source: e.source_id,
        target: e.target_id,
        label: e.kind.replace("EDGE_", ""),
        type: "smoothstep",
        animated: e.kind === "EDGE_CALLS",
        style: { stroke: strokeColor, strokeWidth: 1.5 },
        labelStyle: { fill: "#94a3b8", fontSize: 9, fontFamily: "monospace" },
        labelBgPadding: [4, 2],
        labelBgBorderRadius: 4,
        labelBgStyle: { fill: "#1e293b", fillOpacity: 0.8 },
      };
    });

    return { flowNodes: layoutedNodes, flowEdges: layoutedEdges };
  }, [nodes, edges, selectedNodeID, expandedNodeIDs]);

  const [rfNodes, setNodes, onNodesChange] = useNodesState(flowNodes);
  const [rfEdges, setEdges, onEdgesChange] = useEdgesState(flowEdges);

  React.useEffect(() => {
    setNodes(flowNodes);
    setEdges(flowEdges);
  }, [flowNodes, flowEdges, setNodes, setEdges]);

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      onSelectNode(node.id);
    },
    [onSelectNode]
  );

  const handlePaneClick = useCallback(() => {
    onSelectNode(null);
  }, [onSelectNode]);

  return (
    <div className="w-full h-full bg-[#0b0f19]">
      <ReactFlow
        nodes={rfNodes}
        edges={rfEdges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={handleNodeClick}
        onPaneClick={handlePaneClick}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        minZoom={0.2}
        maxZoom={2.0}
        proOptions={{ hideAttribution: true }}
      >
        <Background color="#1e293b" gap={20} size={1} />
        <Controls className="!bg-surface !border-border !text-gray-300" />
        <MiniMap
          nodeColor={(n) => {
            const data = n.data as any;
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
    </div>
  );
};
