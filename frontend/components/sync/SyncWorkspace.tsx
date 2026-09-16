"use client";

import React, { useMemo } from "react";
import { useAppStore } from "@/store/useAppStore";
import { RepositoryTree } from "../explorer/RepositoryTree";
import { SourceViewer } from "../explorer/SourceViewer";
import { GraphCanvas } from "../graph/GraphCanvas";
import { GraphToolbar } from "../graph/GraphToolbar";
import { GraphInspector } from "../graph/GraphInspector";
import {
  HardDrive,
  FileCode,
  Code2,
  ListFilter,
  Network,
  AlertTriangle,
  RefreshCw,
  FolderTree,
} from "lucide-react";

export const SyncWorkspace: React.FC = () => {
  const {
    activeRepo,
    fileManifest,
    expandedPaths,
    selectedPath,
    selectedSymbolID,
    selectedNodeID,
    highlightLineRange,
    sourceContent,
    isLoadingSource,
    sourceError,
    graphNodes,
    graphEdges,
    graphScope,
    graphNodeLimit,
    graphNodeTypesFilter,
    graphEdgeTypesFilter,
    expandedNodeIDs,
    isLoadingGraph,
    graphError,
    toggleExpandPath,
    setExpandedPaths,
    selectSourceFileAndSyncGraph,
    selectGraphNode,
    syncSourceFromGraphNode,
    expandGraphNode,
    setGraphScope,
    setGraphNodeLimit,
    setGraphNodeTypesFilter,
    setGraphEdgeTypesFilter,
    resetGraph,
    fetchGraph,
    traceFlowFromSymbol,
  } = useAppStore();

  const activeRepoID = activeRepo?.id;

  const [treeSearchQuery, setTreeSearchQuery] = React.useState("");

  // Visible graph nodes & edges
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

  const selectedSymbolNode = useMemo(() => {
    if (!selectedSymbolID) return null;
    return graphNodes.find((n) => n.id === selectedSymbolID) || null;
  }, [graphNodes, selectedSymbolID]);

  const selectedManifestItem = fileManifest.find((item) => item.relative_path === selectedPath);

  const handleSelectTreeFile = (path: string) => {
    selectSourceFileAndSyncGraph(path);
  };

  const handleSelectGraphNode = (nodeID: string | null) => {
    selectGraphNode(nodeID);
  };

  const handleExpandGraphNode = (nodeID: string) => {
    if (activeRepoID) {
      expandGraphNode(activeRepoID, nodeID);
    }
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
          Select a repository from the header dropdown to launch the Synchronized Workspace.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full bg-background overflow-hidden">
      {/* Synchronized Context Indicator Bar */}
      <div className="h-10 border-b border-border bg-surface px-4 flex items-center justify-between shrink-0 select-none text-xs">
        <div className="flex items-center space-x-3 truncate">
          <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded bg-accent/20 border border-accent/40 text-accent flex items-center gap-1.5 shrink-0">
            <RefreshCw className="w-3 h-3 animate-spin-slow" /> SYNC WORKSPACE
          </span>

          <div className="flex items-center space-x-2 text-gray-300 font-mono text-[11px] truncate">
            <span className="flex items-center gap-1 text-gray-400 shrink-0">
              <HardDrive className="w-3 h-3 text-emerald-400" />
              <strong className="text-gray-200">{activeRepo.name}</strong>
            </span>

            {selectedPath && (
              <>
                <span className="text-gray-600">/</span>
                <span className="flex items-center gap-1 text-gray-200 truncate">
                  <FileCode className="w-3 h-3 text-blue-400 shrink-0" />
                  <span className="truncate">{selectedPath}</span>
                </span>
              </>
            )}

            {selectedSymbolNode && (
              <>
                <span className="text-gray-600">/</span>
                <span className="flex items-center gap-1 text-purple-300 truncate">
                  <Code2 className="w-3 h-3 text-purple-400 shrink-0" />
                  <span className="truncate">{selectedSymbolNode.label}</span>
                </span>
              </>
            )}

            {highlightLineRange && (
              <span className="text-accent font-semibold px-1.5 py-0.5 rounded bg-accent/10 border border-accent/20 text-[10px] shrink-0">
                L{highlightLineRange[0]}–L{highlightLineRange[1]}
              </span>
            )}
          </div>
        </div>

        <div className="flex items-center space-x-2 text-[11px] text-gray-400 shrink-0">
          <span>
            Graph: <strong className="text-gray-200">{visibleNodes.length}</strong> nodes
          </span>
        </div>
      </div>

      {/* Main 50/50 Split View */}
      <div className="flex-1 flex overflow-hidden">
        {/* LEFT PANE (50%): Repository Tree + Source Viewer */}
        <div className="w-1/2 border-r border-border flex flex-col overflow-hidden bg-background">
          <div className="flex-1 flex overflow-hidden">
            {/* Tree Sub-pane */}
            <div className="w-64 border-r border-border bg-surface flex flex-col shrink-0 overflow-hidden">
              <div className="p-2 border-b border-border bg-surface/50">
                <input
                  type="text"
                  value={treeSearchQuery}
                  onChange={(e) => setTreeSearchQuery(e.target.value)}
                  placeholder="Filter tree..."
                  className="w-full bg-background border border-border rounded px-2 py-1 text-[11px] text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
                />
              </div>
              <RepositoryTree
                manifest={fileManifest}
                searchQuery={treeSearchQuery}
                expandedPaths={expandedPaths}
                selectedPath={selectedPath}
                onToggleExpand={toggleExpandPath}
                onSetExpandedPaths={setExpandedPaths}
                onSelectFile={handleSelectTreeFile}
              />
            </div>

            {/* Source Sub-pane */}
            <div className="flex-1 bg-background overflow-hidden">
              <SourceViewer
                source={sourceContent}
                isLoading={isLoadingSource}
                error={sourceError}
                selectedPath={selectedPath}
                manifestItem={selectedManifestItem}
                highlightLineRange={highlightLineRange}
              />
            </div>
          </div>
        </div>

        {/* RIGHT PANE (50%): Graph Toolbar + Graph Canvas + Graph Inspector */}
        <div className="w-1/2 flex flex-col overflow-hidden bg-background">
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

          {graphError && (
            <div className="mx-3 mt-2 border border-red-500/40 bg-red-500/10 text-red-300 rounded-lg p-2.5 flex items-center justify-between shrink-0 select-none">
              <div className="flex items-center space-x-2 text-xs">
                <AlertTriangle className="w-4 h-4 text-red-400 shrink-0" />
                <span>{graphError}</span>
              </div>
              {activeRepoID && (
                <button
                  onClick={() => fetchGraph(activeRepoID, { scope: graphScope })}
                  className="px-2 py-0.5 rounded text-xs bg-red-500/20 border border-red-500/40 text-red-200"
                >
                  Retry
                </button>
              )}
            </div>
          )}

          <div className="flex-1 flex overflow-hidden">
            {/* Graph Canvas */}
            <div className="flex-1 relative overflow-hidden">
              {isLoadingGraph && visibleNodes.length === 0 ? (
                <div className="h-full flex flex-col items-center justify-center p-6 bg-background">
                  <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin mb-2" />
                  <p className="text-xs text-gray-400">Loading graph...</p>
                </div>
              ) : visibleNodes.length === 0 ? (
                <div className="h-full flex flex-col items-center justify-center p-6 text-center text-xs text-gray-400 select-none">
                  <Network className="w-8 h-8 text-gray-500 mb-2" />
                  <p>No nodes match current filters.</p>
                </div>
              ) : (
                <GraphCanvas
                  nodes={visibleNodes}
                  edges={visibleEdges}
                  selectedNodeID={selectedNodeID}
                  expandedNodeIDs={expandedNodeIDs}
                  onSelectNode={handleSelectGraphNode}
                />
              )}

              {/* Stats overlay */}
              <div className="absolute bottom-2 left-2 bg-surface/90 border border-border backdrop-blur-md px-2.5 py-1 rounded text-[10px] text-gray-300 flex items-center space-x-2 pointer-events-none select-none">
                <span>Nodes: <strong className="text-white">{visibleNodes.length}</strong></span>
                <span>Edges: <strong className="text-white">{visibleEdges.length}</strong></span>
              </div>
            </div>

            {/* Inspector */}
            <div className="w-64 border-l border-border bg-surface shrink-0 overflow-hidden">
              <GraphInspector
                selectedNode={selectedNode}
                nodes={graphNodes}
                edges={graphEdges}
                onExpandNode={handleExpandGraphNode}
                onViewInExplorer={(nodeID) => syncSourceFromGraphNode(nodeID)}
                onTraceFlow={(symbolID) => traceFlowFromSymbol(symbolID)}
                isLoading={isLoadingGraph}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
