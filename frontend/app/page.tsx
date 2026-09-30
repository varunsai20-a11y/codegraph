"use client";

import React, { useEffect } from "react";
import { Header } from "@/components/layout/Header";
import { SidebarNav } from "@/components/layout/SidebarNav";
import { StatusBar } from "@/components/layout/StatusBar";
import { PersistentAIBanner } from "@/components/ai/PersistentAIBanner";
import { RepositoryExplorer } from "@/components/explorer/RepositoryExplorer";
import { GraphExplorer } from "@/components/graph/GraphExplorer";
import { useAppStore } from "@/store/useAppStore";
import { AlertTriangle, Cpu } from "lucide-react";

import { SyncWorkspace } from "@/components/sync/SyncWorkspace";
import { FlowExplorer } from "@/components/flow/FlowExplorer";
import { ExplanationPanel } from "@/components/ai/ExplanationPanel";
import { GuideView } from "@/components/guide/GuideView";

export default function Home() {
  const [mounted, setMounted] = React.useState(false);
  const { activeTab, fetchRepositories, error } = useAppStore();

  React.useEffect(() => {
    setMounted(true);
    fetchRepositories();
  }, [fetchRepositories]);

  if (!mounted) {
    return (
      <div className="flex items-center justify-center h-screen bg-background text-gray-400 text-xs font-mono">
        <div className="w-5 h-5 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin mr-2.5" />
        <span>Initializing Graphite Intelligence Workspace...</span>
      </div>
    );
  }

  const renderTabContent = () => {
    switch (activeTab) {
      case "EXPLORER":
        return <RepositoryExplorer />;
      case "GRAPH":
        return <GraphExplorer />;
      case "SYNC":
        return <SyncWorkspace />;
      case "FLOW":
        return <FlowExplorer />;
      case "AI":
        return <ExplanationPanel />;
      case "GUIDE":
        return <GuideView />;
      default:
        return <RepositoryExplorer />;
    }
  };

  return (
    <div className="flex flex-col h-screen overflow-hidden bg-background text-foreground selection:bg-cyan-900 selection:text-cyan-100">
      {/* Persistent Technical Top Header */}
      <Header />

      {/* Main Workspace Frame */}
      <div className="flex flex-1 overflow-hidden relative">
        {/* Persistent Left Navigation Sidebar */}
        <SidebarNav />

        {/* Center Main Viewport */}
        <main className="flex-1 bg-background flex flex-col overflow-hidden relative">
          {/* Global Backend Error Alert */}
          {error && (
            <div className="m-3 border border-red-500/40 bg-red-950/30 text-red-300 rounded-sm p-3 flex items-center justify-between text-xs font-mono shrink-0 select-none z-30">
              <div className="flex items-center space-x-2">
                <AlertTriangle className="w-4 h-4 text-red-400 shrink-0" />
                <span>Backend Error: {error} (Ensure main.go server is running on port 8080)</span>
              </div>
              <button
                onClick={() => fetchRepositories()}
                className="px-2.5 py-1 rounded-sm bg-red-950 border border-red-500/50 hover:bg-red-900 text-red-200 font-bold transition"
              >
                Retry
              </button>
            </div>
          )}

          {/* Active View Content */}
          <div className="flex-1 overflow-hidden">{renderTabContent()}</div>

          {/* Persistent AI Interaction Banner */}
          <PersistentAIBanner />
        </main>
      </div>

      {/* Bottom Technical Status Bar */}
      <StatusBar />
    </div>
  );
}
