"use client";

import React from "react";
import { useAppStore } from "@/store/useAppStore";
import { Database, CheckCircle2, AlertCircle, RefreshCw } from "lucide-react";

export const Header: React.FC = () => {
  const {
    repositories,
    activeRepoID,
    selectRepository,
    fetchRepositories,
    isLoading,
    error,
  } = useAppStore();

  return (
    <header className="h-14 border-b border-border bg-surface px-4 flex items-center justify-between select-none">
      <div className="flex items-center space-x-3">
        <div className="w-8 h-8 rounded-lg bg-accent/20 border border-accent/40 flex items-center justify-center text-accent">
          <Database className="w-4 h-4" />
        </div>
        <div>
          <h1 className="text-sm font-bold tracking-wide text-gray-100 flex items-center gap-2">
            CodeGraph
            <span className="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-accent/10 border border-accent/30 text-accent font-semibold">
              Phase 5 C1
            </span>
          </h1>
          <p className="text-[11px] text-gray-400">Visual Reverse Engineering</p>
        </div>
      </div>

      <div className="flex items-center space-x-4">
        {/* Repository Selector */}
        <div className="flex items-center space-x-2">
          <span className="text-xs text-gray-400 font-medium">Repository:</span>
          <select
            value={activeRepoID || ""}
            onChange={(e) => selectRepository(e.target.value)}
            disabled={repositories.length === 0}
            className="bg-background border border-border text-xs rounded-md px-2.5 py-1.5 text-gray-200 focus:outline-none focus:border-accent"
          >
            {repositories.length === 0 ? (
              <option value="">No repositories found</option>
            ) : (
              repositories.map((repo) => (
                <option key={repo.id} value={repo.id}>
                  {repo.name} ({repo.status})
                </option>
              ))
            )}
          </select>
        </div>

        {/* Refresh Button */}
        <button
          onClick={() => fetchRepositories()}
          disabled={isLoading}
          className="p-1.5 rounded-md border border-border hover:bg-surface-hover text-gray-300 hover:text-white transition"
          title="Refresh Repositories"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${isLoading ? "animate-spin text-accent" : ""}`} />
        </button>

        {/* Backend Connection Indicator */}
        <div className="flex items-center space-x-1.5 text-xs px-2.5 py-1 rounded-full border border-border bg-background">
          {error ? (
            <>
              <AlertCircle className="w-3.5 h-3.5 text-red-400" />
              <span className="text-red-400 font-medium">Backend Offline</span>
            </>
          ) : (
            <>
              <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
              <span className="text-emerald-400 font-medium">Connected</span>
            </>
          )}
        </div>
      </div>
    </header>
  );
};
