"use client";

import React, { useEffect } from "react";
import { Header } from "@/components/layout/Header";
import { SidebarNav } from "@/components/layout/SidebarNav";
import { StatusBar } from "@/components/layout/StatusBar";
import { RepositoryExplorer } from "@/components/explorer/RepositoryExplorer";
import { GraphExplorer } from "@/components/graph/GraphExplorer";
import { useAppStore } from "@/store/useAppStore";
import { AlertTriangle, SplitSquareVertical, GitFork, Bot, Compass } from "lucide-react";

export default function Home() {
  const { activeTab, fetchRepositories, error } = useAppStore();

  useEffect(() => {
    fetchRepositories();
  }, [fetchRepositories]);

  const renderTabContent = () => {
    switch (activeTab) {
      case "EXPLORER":
        return <RepositoryExplorer />;
      case "GRAPH":
        return <GraphExplorer />;
      case "SYNC":
        return (
          <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
            <SplitSquareVertical className="w-10 h-10 text-gray-500 mb-3" />
            <h3 className="text-sm font-bold text-gray-200">Synchronized Code & Graph Split Workspace</h3>
            <p className="text-xs text-gray-400 max-w-sm mt-1">
              Bi-directional source selection and graph node synchronization will be implemented in Checkpoint 4.
            </p>
          </div>
        );
      case "FLOW":
        return (
          <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
            <GitFork className="w-10 h-10 text-gray-500 mb-3" />
            <h3 className="text-sm font-bold text-gray-200">Call Flow & Execution Analysis</h3>
            <p className="text-xs text-gray-400 max-w-sm mt-1">
              Interactive caller/callee flow tracing will be implemented in Checkpoint 5.
            </p>
          </div>
        );
      case "AI":
        return (
          <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
            <Bot className="w-10 h-10 text-gray-500 mb-3" />
            <h3 className="text-sm font-bold text-gray-200">AI Code Assistant & Hybrid Retrieval</h3>
            <p className="text-xs text-gray-400 max-w-sm mt-1">
              AI explanation workspace and code intelligence chat will be implemented in Checkpoint 6.
            </p>
          </div>
        );
      case "GUIDE":
        return (
          <div className="h-full flex flex-col items-center justify-center p-8 bg-background text-center select-none">
            <Compass className="w-10 h-10 text-gray-500 mb-3" />
            <h3 className="text-sm font-bold text-gray-200">Guided Architectural Onboarding Tour</h3>
            <p className="text-xs text-gray-400 max-w-sm mt-1">
              Automated repository walkthrough and architectural tour will be implemented in Checkpoint 7.
            </p>
          </div>
        );
      default:
        return <RepositoryExplorer />;
    }
  };

  return (
    <div className="flex flex-col h-screen overflow-hidden">
      <Header />
      <div className="flex flex-1 overflow-hidden">
        <SidebarNav />

        <main className="flex-1 bg-background flex flex-col overflow-hidden relative">
          {/* Global Backend Connection Error Toast/Banner */}
          {error && (
            <div className="m-3 border border-red-500/40 bg-red-500/10 text-red-300 rounded-lg p-3 flex items-center justify-between text-xs shrink-0 select-none">
              <div className="flex items-center space-x-2">
                <AlertTriangle className="w-4 h-4 text-red-400 shrink-0" />
                <span>Backend Error: {error} (Ensure backend main.go is running on port 8080)</span>
              </div>
              <button
                onClick={() => fetchRepositories()}
                className="px-2 py-0.5 rounded bg-red-500/20 border border-red-500/40 hover:bg-red-500/30 text-red-200 font-medium transition"
              >
                Retry
              </button>
            </div>
          )}

          <div className="flex-1 overflow-hidden">{renderTabContent()}</div>
        </main>
      </div>
      <StatusBar />
    </div>
  );
}
