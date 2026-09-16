"use client";

import React, { useMemo } from "react";
import { useAppStore } from "@/store/useAppStore";
import { GraphCanvas } from "./GraphCanvas";
import { GraphToolbar } from "./GraphToolbar";
import { GraphInspector } from "./GraphInspector";
import { Network, AlertTriangle, RefreshCw } from "lucide-react";

export const GraphExplorer: React.FC = () => {
  const {
    activeRepo,
    graphNodes,
    graphEdges,
    graphScope,
    graphNodeLimit,
    graphNodeTypesFilter,
    graphEdgeTypesFilter,
    selectedNodeID,
    expandedNodeIDs,
    isLoadingGraph,
    graphError,
    fetchGraph,
    expandGraphNode,
    setGraphScope,
    setGraphNodeLimit,
    setGraphNodeTypesFilter,
    setGraphEdgeTypesFilter,
    resetGraph,
    selectGraphNode,
    navigateToSourceFromGraph,
    traceFlowFromSymbol,
  } = useAppStore();

  const activeRepoID = activeRepo?.id;

  // Filter nodes & edges for canvas presentation without mutating traversal state
  const { visibleNodes, visibleEdges } = useMemo(() => {
    const filteredNodes = graphNodes.filter((n) => graphNodeTypesFilter.has(n.kind));
    const validNodeIDs = new Set(filteredNodes.map((n) => n.id));
    const filteredEdges = graphEdges.filter(
      (e) =>
        graphEdgeTypesFilter.has(e.kind) &&
        validNodeIDs.has(e.source_id) &&
        validNodeIDs.has(e.target_id)
    );
    return { visibleNodes: filteredNodes, visibleEdges: filteredEdges };
  }, [graphNodes, graphEdges, graphNodeTypesFilter, graphEdgeTypesFilter]);

  const selectedNode = useMemo(() => {
    return graphNodes.find((n) => n.id === selectedNodeID) || null;
  }, [graphNodes, selectedNodeID]);

  const handleExpandNode = (nodeID: string) => {
    if (activeRepoID) {
      expandGraphNode(activeRepoID, nodeID);
    }
  };

  const handleViewInExplorer = (nodeID: string) => {
    navigateToSourceFromGraph(nodeID, "EXPLORER");
  };

  const handleResetGraph = () => {
    if (activeRepoID) {
      resetGraph(activeRepoID);
    }
  };

  if (!activeRepo) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
        <Network className="w-10 h-10 text-gray-500 mb-3" />
        <h3 className="text-sm font-bold text-gray-200">No Repository Selected</h3>
        <p className="text-xs text-gray-400 max-w-sm mt-1">
          Select a repository from the header dropdown to explore its architectural knowledge graph.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full bg-background overflow-hidden">
      {/* Graph Toolbar */}
      <GraphToolbar
        scope={graphScope}
        nodeLimit={graphNodeLimit}
        nodeTypesFilter={graphNodeTypesFilter}
        edgeTypesFilter={graphEdgeTypesFilter}
        onSetScope={setGraphScope}
        onSetNodeLimit={(limit) => {
          setGraphNodeLimit(limit);
          if (activeRepoID) {
            fetchGraph(activeRepoID, { scope: graphScope, node_limit: limit });
          }
        }}
        onSetNodeTypesFilter={setGraphNodeTypesFilter}
        onSetEdgeTypesFilter={setGraphEdgeTypesFilter}
        onResetGraph={handleResetGraph}
        isLoading={isLoadingGraph}
      />

      {/* Error Alert */}
      {graphError && (
        <div className="mx-4 mt-3 border border-red-500/40 bg-red-500/10 text-red-300 rounded-lg p-3.5 flex items-center justify-between shrink-0 select-none">
          <div className="flex items-center space-x-2.5">
            <AlertTriangle className="w-4 h-4 text-red-400 shrink-0" />
            <span className="text-xs">{graphError}</span>
          </div>
          {activeRepoID && (
            <button
              onClick={() => fetchGraph(activeRepoID, { scope: graphScope })}
              className="px-2.5 py-1 rounded text-xs bg-red-500/20 border border-red-500/40 hover:bg-red-500/30 text-red-200 font-medium transition flex items-center gap-1"
            >
              <RefreshCw className="w-3 h-3" /> Retry
            </button>
          )}
        </div>
      )}

      {/* Main Graph View & Inspector Panel */}
      <div className="flex-1 flex overflow-hidden">
        {/* Canvas Area */}
        <div className="flex-1 relative overflow-hidden">
          {isLoadingGraph && visibleNodes.length === 0 ? (
            <div className="h-full flex flex-col items-center justify-center p-8 bg-background">
              <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin mb-3" />
              <p className="text-xs text-gray-400">Loading graph for {activeRepo.name}...</p>
            </div>
          ) : visibleNodes.length === 0 ? (
            <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
              <Network className="w-8 h-8 text-gray-500 mb-2" />
              <h4 className="text-xs font-semibold text-gray-300">No Graph Nodes Match Active Filters</h4>
              <p className="text-[11px] text-gray-400 mt-1 max-w-sm">
                Try resetting filters or expanding node limit in the toolbar.
              </p>
            </div>
          ) : (
            <GraphCanvas
              nodes={visibleNodes}
              edges={visibleEdges}
              selectedNodeID={selectedNodeID}
              expandedNodeIDs={expandedNodeIDs}
              onSelectNode={(nodeID) => selectGraphNode(nodeID)}
            />
          )}

          {/* Floating Graph Stats Overlay */}
          <div className="absolute bottom-3 left-3 bg-surface/90 border border-border backdrop-blur-md px-3 py-1.5 rounded-md text-[11px] text-gray-300 flex items-center space-x-3 pointer-events-none select-none">
            <span>
              Visible Nodes: <strong className="text-white">{visibleNodes.length}</strong>
            </span>
            <span>
              Visible Edges: <strong className="text-white">{visibleEdges.length}</strong>
            </span>
          </div>
        </div>

        {/* Right Pane: Graph Inspector Panel */}
        <div className="w-80 border-l border-border bg-surface shrink-0 overflow-hidden">
          <GraphInspector
            selectedNode={selectedNode}
            nodes={graphNodes}
            edges={graphEdges}
            onExpandNode={handleExpandNode}
            onViewInExplorer={handleViewInExplorer}
            onTraceFlow={(symbolID) => traceFlowFromSymbol(symbolID)}
            isLoading={isLoadingGraph}
          />
        </div>
      </div>
    </div>
  );
};
