"use client";

import React, { useState, useEffect } from "react";
import { useAppStore } from "@/store/useAppStore";
import {
  Database,
  CheckCircle2,
  AlertCircle,
  RefreshCw,
  Github,
  Search,
  Terminal,
  Activity,
  Cpu,
  Layers,
} from "lucide-react";
import { ImportRepoModal } from "./ImportRepoModal";

export const Header: React.FC = () => {
  const [isImportModalOpen, setIsImportModalOpen] = useState(false);

  const {
    repositories,
    activeRepoID,
    activeRepo,
    activeRepoStats,
    selectRepository,
    fetchRepositories,
    isLoadingRepos,
    error,
    searchQuery,
    setSearchQuery,
    explanationResult,
    explanationError,
  } = useAppStore();

  // Keyboard shortcut for search input focus
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        const searchEl = document.getElementById("global-search-input");
        searchEl?.focus();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  const repoStatus = activeRepo?.status || "";
  const repoStatusStyle =
    repoStatus === "INDEXED"
      ? "text-emerald-400 border-emerald-500/30 bg-emerald-950/30"
      : repoStatus === "INDEXING"
      ? "text-amber-400 border-amber-500/30 bg-amber-950/30"
      : repoStatus === "FAILED"
      ? "text-red-400 border-red-500/30 bg-red-950/30"
      : "text-cyan-400 border-cyan-500/30 bg-cyan-950/30";

  const repoStatusText =
    repoStatus === "INDEXED"
      ? "INDEXED"
      : repoStatus === "INDEXING"
      ? "ANALYZING"
      : repoStatus === "REGISTERED"
      ? "REGISTERED"
      : repoStatus || "READY";

  return (
    <>
      <header className="h-11 border-b border-border bg-surface px-3.5 flex items-center justify-between select-none shrink-0 text-xs font-mono relative z-20">
        {/* Left: Branding & Repository Dropdown */}
        <div className="flex items-center space-x-3.5">
          <div className="flex items-center space-x-2">
            <div className="w-6 h-6 rounded-sm bg-cyan-950/90 border border-cyan-500/50 flex items-center justify-center text-cyan-400 shadow-sm">
              <Cpu className="w-3.5 h-3.5" />
            </div>
            <div className="flex items-center space-x-1.5">
              <span className="font-mono font-extrabold tracking-wider text-gray-100 text-xs uppercase">
                CODE<span className="text-cyan-400">GRAPH</span>
              </span>
              <span className="text-[9px] font-mono tracking-widest px-1 py-0.2 rounded bg-cyan-950/50 border border-cyan-500/30 text-cyan-300">
                GRAPHITE
              </span>
            </div>
          </div>

          <div className="h-4 w-[1px] bg-border" />

          {/* Repository Selector */}
          <div className="flex items-center space-x-2">
            <select
              value={activeRepoID || ""}
              onChange={(e) => selectRepository(e.target.value)}
              disabled={repositories.length === 0}
              className="bg-background border border-border text-xs rounded-sm px-2.5 py-1 text-gray-200 font-mono focus:outline-none focus:border-cyan-500 hover:border-border/80 transition cursor-pointer"
            >
              {repositories.length === 0 ? (
                <option value="">No Repositories</option>
              ) : (
                repositories.map((repo) => (
                  <option key={repo.id} value={repo.id}>
                    {repo.name}
                  </option>
                ))
              )}
            </select>

            {activeRepo && (
              <div className="flex items-center space-x-1.5">
                <span
                  className={`text-[9px] font-mono font-bold tracking-wider px-2 py-0.5 rounded-sm border flex items-center gap-1.5 ${repoStatusStyle}`}
                >
                  <span
                    className={`w-1.5 h-1.5 rounded-full ${
                      repoStatus === "INDEXED"
                        ? "bg-emerald-400"
                        : repoStatus === "INDEXING"
                        ? "bg-amber-400 animate-pulse"
                        : "bg-cyan-400"
                    }`}
                  />
                  <span>● {repoStatusText}</span>
                </span>
                {activeRepoStats && (
                  <span className="text-[9px] font-mono text-gray-300 px-2 py-0.5 rounded-sm bg-background border border-border">
                    {activeRepoStats.files_indexed} files · {activeRepoStats.folders_discovered} folders
                  </span>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Center: Global Search Bar */}
        <div className="flex-1 max-w-md mx-6">
          <div className="relative flex items-center">
            <Search className="w-3.5 h-3.5 absolute left-2.5 text-gray-500 pointer-events-none" />
            <input
              id="global-search-input"
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search symbols, paths, or relationships... (Ctrl+K)"
              className="w-full bg-background border border-border rounded-sm pl-8 pr-12 py-1 text-xs text-gray-200 placeholder-gray-500 font-mono focus:outline-none focus:border-cyan-500/80 transition"
            />
            <span className="absolute right-2 text-[9px] font-mono text-gray-500 bg-surface px-1.5 py-0.5 rounded-sm border border-border pointer-events-none">
              ⌘K
            </span>
          </div>
        </div>

        {/* Right: Actions & System Status */}
        <div className="flex items-center space-x-2.5">
          <button
            onClick={() => setIsImportModalOpen(true)}
            className="flex items-center gap-1.5 px-2.5 py-1 rounded-sm border border-cyan-500/40 bg-cyan-950/30 hover:bg-cyan-900/40 text-cyan-300 text-xs font-mono transition"
          >
            <Github className="w-3 h-3 text-cyan-400" />
            <span>Import Repo</span>
          </button>

          <button
            onClick={() => fetchRepositories()}
            disabled={isLoadingRepos}
            className="p-1 rounded-sm border border-border hover:bg-surface-hover text-gray-400 hover:text-white transition"
            title="Refresh Repositories"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoadingRepos ? "animate-spin text-cyan-400" : ""}`} />
          </button>

          <div className="h-4 w-[1px] bg-border" />

          {/* Engine Connection Badge */}
          <div className={`flex items-center space-x-1.5 text-[10px] font-mono px-2 py-0.5 rounded-sm border ${
            error || explanationError || explanationResult?.provider_mode === "LLM_PROVIDER_ERROR"
              ? "border-red-500/30 bg-red-950/20 text-red-400"
              : explanationResult?.is_insufficient_evidence
              ? "border-amber-500/30 bg-amber-950/20 text-amber-400"
              : explanationResult?.provider_mode === "DETERMINISTIC_SUMMARY"
              ? "border-amber-500/30 bg-amber-950/20 text-amber-300"
              : "border-emerald-500/30 bg-emerald-950/20 text-emerald-400"
          }`}>
            {error ? (
              <>
                <AlertCircle className="w-3 h-3 text-red-400" />
                <span className="text-red-400 font-bold">OFFLINE</span>
              </>
            ) : explanationError || explanationResult?.provider_mode === "LLM_PROVIDER_ERROR" ? (
              <>
                <AlertCircle className="w-3 h-3 text-red-400" />
                <span className="text-red-400 font-bold">PROVIDER ERROR</span>
              </>
            ) : explanationResult?.is_insufficient_evidence ? (
              <>
                <span className="w-1.5 h-1.5 rounded-full bg-amber-400" />
                <span className="text-amber-400 font-bold">RETRIEVAL INSUFFICIENT</span>
              </>
            ) : (
              <>
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400" />
                <span className="text-emerald-400 font-bold">
                  {explanationResult?.provider
                    ? `${explanationResult.provider.toUpperCase()} GROUNDED`
                    : "GEMINI GROUNDED"}
                </span>
              </>
            )}
          </div>
        </div>
      </header>

      {/* Import Repository Modal */}
      <ImportRepoModal
        isOpen={isImportModalOpen}
        onClose={() => setIsImportModalOpen(false)}
      />
    </>
  );
};
