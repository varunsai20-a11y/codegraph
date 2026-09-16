"use client";

import React, { useMemo } from "react";
import { useAppStore } from "@/store/useAppStore";
import { FlowToolbar } from "./FlowToolbar";
import { FlowCanvas } from "./FlowCanvas";
import { FlowInspector } from "./FlowInspector";
import { GitFork } from "lucide-react";

export const FlowExplorer: React.FC = () => {
  const {
    activeRepo,
    flowResult,
    flowRootNodeID,
    flowTargetNodeID,
    flowMaxDepth,
    selectedFlowStepIndex,
    isLoadingFlow,
    graphNodes,
    fetchStaticFlow,
    setFlowMaxDepth,
    selectFlowStep,
    navigateToSourceFromGraph,
    setActiveTab,
  } = useAppStore();

  const activeRepoID = activeRepo?.id;

  const rootSymbolNode = useMemo(() => {
    if (!flowRootNodeID) return null;
    return graphNodes.find((n) => n.id === flowRootNodeID) || null;
  }, [graphNodes, flowRootNodeID]);

  const targetSymbolNode = useMemo(() => {
    if (!flowTargetNodeID) return null;
    return graphNodes.find((n) => n.id === flowTargetNodeID) || null;
  }, [graphNodes, flowTargetNodeID]);

  const handleRefresh = () => {
    if (activeRepoID && flowRootNodeID) {
      fetchStaticFlow(activeRepoID, flowRootNodeID, flowTargetNodeID || undefined, flowMaxDepth);
    }
  };

  const selectedStep = useMemo(() => {
    if (!flowResult?.path?.steps || selectedFlowStepIndex === null) return null;
    return flowResult.path.steps[selectedFlowStepIndex] || null;
  }, [flowResult, selectedFlowStepIndex]);

  const handleSelectStep = (index: number) => {
    selectFlowStep(index);
  };

  const handleViewSource = (nodeID: string) => {
    navigateToSourceFromGraph(nodeID, "EXPLORER");
  };

  const handleViewInGraph = (nodeID: string) => {
    setActiveTab("GRAPH");
  };

  if (!activeRepo) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
        <GitFork className="w-10 h-10 text-gray-500 mb-3" />
        <h3 className="text-sm font-bold text-gray-200">No Repository Selected</h3>
        <p className="text-xs text-gray-400 max-w-sm mt-1">
          Select a repository from the header dropdown to launch the Static Call Flow workspace.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full bg-background overflow-hidden">
      {/* Flow Toolbar */}
      <FlowToolbar
        rootSymbolLabel={rootSymbolNode?.label || flowRootNodeID || undefined}
        targetSymbolLabel={targetSymbolNode?.label || flowTargetNodeID || undefined}
        maxDepth={flowMaxDepth}
        onSetMaxDepth={(depth) => {
          setFlowMaxDepth(depth);
          if (activeRepoID && flowRootNodeID) {
            fetchStaticFlow(activeRepoID, flowRootNodeID, flowTargetNodeID || undefined, depth);
          }
        }}
        onRefresh={handleRefresh}
        isLoading={isLoadingFlow}
      />

      {/* Main Flow Canvas & Inspector Split View */}
      <div className="flex-1 flex overflow-hidden">
        {/* Call Pipeline Canvas */}
        <div className="flex-1 bg-background overflow-hidden relative">
          <FlowCanvas
            flowResult={flowResult}
            selectedStepIndex={selectedFlowStepIndex}
            onSelectStep={handleSelectStep}
            isLoading={isLoadingFlow}
          />
        </div>

        {/* Right Inspector Panel */}
        <div className="w-80 border-l border-border bg-surface shrink-0 overflow-hidden">
          <FlowInspector
            selectedStep={selectedStep}
            onViewSource={handleViewSource}
            onViewInGraph={handleViewInGraph}
          />
        </div>
      </div>
    </div>
  );
};
