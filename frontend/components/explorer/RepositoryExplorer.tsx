"use client";

import React from "react";
import { useAppStore } from "@/store/useAppStore";
import { RepositoryTree } from "./RepositoryTree";
import { SourceViewer } from "./SourceViewer";
import {
  HardDrive,
  Search,
  AlertTriangle,
  RefreshCw,
  FileText,
  Lock,
  Binary,
  CheckCircle2,
  FolderTree,
  Terminal,
  Cpu,
} from "lucide-react";

export const RepositoryExplorer: React.FC = () => {
  const {
    activeRepo,
    fileManifest,
    expandedPaths,
    selectedPath,
    highlightLineRange,
    sourceContent,
    searchQuery,
    isLoadingManifest,
    isLoadingSource,
    manifestError,
    sourceError,
    toggleExpandPath,
    setExpandedPaths,
    setSearchQuery,
    fetchSourceFile,
    fetchFileManifest,
    navigateToGraphFromSource,
  } = useAppStore();

  const activeRepoID = activeRepo?.id;

  const handleSelectFile = (path: string) => {
    if (activeRepoID) {
      fetchSourceFile(activeRepoID, path);
    }
  };

  const selectedManifestItem = fileManifest.find((item) => item.relative_path === selectedPath);

  const indexedCount = fileManifest.filter((f) => f.status === "INDEXED").length;
  const secretCount = fileManifest.filter((f) => f.status === "SECRET").length;
  const binaryCount = fileManifest.filter((f) => f.status === "BINARY").length;

  return (
    <div className="flex flex-col h-full bg-background overflow-hidden font-mono">
      {/* Top Repository Bar */}
      <div className="border-b border-border bg-surface px-3.5 py-2 shrink-0 space-y-2 select-none text-xs">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-6 h-6 rounded-sm bg-cyan-950/80 border border-cyan-500/40 flex items-center justify-center text-cyan-400">
              <Terminal className="w-3.5 h-3.5" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <h2 className="text-xs font-bold text-gray-100 font-mono">
                  {activeRepo ? activeRepo.name : "No Repository Selected"}
                </h2>
                {activeRepo && (
                  <span className="text-[9px] px-1.5 py-0.2 rounded-sm bg-emerald-950/60 border border-emerald-500/40 text-emerald-400 font-bold font-mono">
                    {activeRepo.status === "INDEXED" ? "READY" : activeRepo.status}
                  </span>
                )}
              </div>
              {activeRepo && (
                <p className="text-[10px] text-gray-500 font-mono truncate max-w-md">
                  {activeRepo.local_path || activeRepo.id}
                </p>
              )}
            </div>
          </div>

          {activeRepo && (
            <div className="flex items-center space-x-2 text-[10px] font-mono">
              <div className="flex items-center space-x-1 px-2 py-0.5 rounded-sm bg-background border border-border text-gray-300">
                <FileText className="w-3 h-3 text-cyan-400" />
                <span>
                  FILES: <strong className="text-white">{fileManifest.length}</strong>
                </span>
              </div>
              <div className="flex items-center space-x-1 px-2 py-0.5 rounded-sm bg-background border border-border text-gray-300">
                <CheckCircle2 className="w-3 h-3 text-emerald-400" />
                <span>
                  INDEXED: <strong className="text-white">{indexedCount}</strong>
                </span>
              </div>
              {secretCount > 0 && (
                <div className="flex items-center space-x-1 px-2 py-0.5 rounded-sm bg-background border border-border text-red-400">
                  <Lock className="w-3 h-3" />
                  <span>
                    SECRET: <strong>{secretCount}</strong>
                  </span>
                </div>
              )}
              {binaryCount > 0 && (
                <div className="flex items-center space-x-1 px-2 py-0.5 rounded-sm bg-background border border-border text-amber-400">
                  <Binary className="w-3 h-3" />
                  <span>
                    BINARY: <strong>{binaryCount}</strong>
                  </span>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Filter Input Bar */}
        <div className="flex items-center space-x-2">
          <div className="relative flex-1 max-w-sm">
            <Search className="w-3 h-3 text-gray-500 absolute left-2.5 top-2 pointer-events-none" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Filter tree by file path..."
              className="w-full bg-background border border-border rounded-sm pl-8 pr-3 py-1 text-xs text-gray-200 placeholder-gray-500 focus:outline-none focus:border-cyan-500/80 font-mono"
            />
            {searchQuery && (
              <button
                onClick={() => setSearchQuery("")}
                className="absolute right-2 top-1 text-xs text-gray-400 hover:text-white"
              >
                ×
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Manifest Error Alert */}
      {manifestError && (
        <div className="mx-3 mt-2 border border-red-500/40 bg-red-950/20 text-red-300 rounded-sm p-2.5 flex items-center justify-between shrink-0 text-xs font-mono">
          <div className="flex items-center space-x-2">
            <AlertTriangle className="w-3.5 h-3.5 text-red-400 shrink-0" />
            <span>{manifestError}</span>
          </div>
          {activeRepoID && (
            <button
              onClick={() => fetchFileManifest(activeRepoID)}
              className="px-2 py-0.5 rounded-sm text-[10px] bg-red-950 border border-red-500/40 hover:bg-red-900 text-red-200 font-bold transition flex items-center gap-1"
            >
              <RefreshCw className="w-3 h-3" /> Retry
            </button>
          )}
        </div>
      )}

      {/* Main Split View */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left Tree Pane */}
        <div className="w-72 border-r border-border bg-surface flex flex-col shrink-0 overflow-hidden">
          {isLoadingManifest ? (
            <div className="flex-1 flex flex-col items-center justify-center p-4 text-center text-xs text-gray-400 space-y-2 font-mono">
              <div className="w-4 h-4 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin" />
              <span>Loading file manifest...</span>
            </div>
          ) : !activeRepo ? (
            <div className="flex-1 flex flex-col items-center justify-center p-4 text-center text-xs text-gray-500 font-mono">
              <FolderTree className="w-6 h-6 text-gray-600 mb-2" />
              <span>Select a repository.</span>
            </div>
          ) : (
            <RepositoryTree
              manifest={fileManifest}
              searchQuery={searchQuery}
              expandedPaths={expandedPaths}
              selectedPath={selectedPath}
              onToggleExpand={toggleExpandPath}
              onSetExpandedPaths={setExpandedPaths}
              onSelectFile={handleSelectFile}
            />
          )}
        </div>

        {/* Right Source Viewer Pane */}
        <div className="flex-1 bg-background overflow-hidden">
          <SourceViewer
            source={sourceContent}
            isLoading={isLoadingSource}
            error={sourceError}
            selectedPath={selectedPath}
            manifestItem={selectedManifestItem}
            highlightLineRange={highlightLineRange}
            onViewInGraph={navigateToGraphFromSource}
          />
        </div>
      </div>
    </div>
  );
};
