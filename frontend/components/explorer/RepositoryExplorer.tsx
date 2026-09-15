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
} from "lucide-react";

export const RepositoryExplorer: React.FC = () => {
  const {
    activeRepo,
    fileManifest,
    expandedPaths,
    selectedPath,
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
    <div className="flex flex-col h-full bg-background overflow-hidden">
      {/* Top Repository Summary & Filter Bar */}
      <div className="border-b border-border bg-surface px-4 py-3 shrink-0 space-y-3 select-none">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className="w-8 h-8 rounded-lg bg-accent/10 border border-accent/30 flex items-center justify-center text-accent">
              <HardDrive className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center space-x-2">
                <h2 className="text-sm font-bold text-gray-100">
                  {activeRepo ? activeRepo.name : "No Repository Selected"}
                </h2>
                {activeRepo && (
                  <span className="text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 font-semibold font-mono">
                    {activeRepo.status}
                  </span>
                )}
              </div>
              {activeRepo && (
                <p className="text-[11px] text-gray-400 font-mono truncate max-w-md">
                  ID: {activeRepo.id} {activeRepo.local_path ? `| Path: ${activeRepo.local_path}` : ""}
                </p>
              )}
            </div>
          </div>

          {activeRepo && (
            <div className="flex items-center space-x-3 text-xs">
              <div className="flex items-center space-x-1.5 px-2.5 py-1 rounded bg-background border border-border text-gray-300">
                <FileText className="w-3.5 h-3.5 text-accent" />
                <span>
                  Files: <strong className="text-white">{fileManifest.length}</strong>
                </span>
              </div>
              <div className="flex items-center space-x-1.5 px-2.5 py-1 rounded bg-background border border-border text-gray-300">
                <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
                <span>
                  Indexed: <strong className="text-white">{indexedCount}</strong>
                </span>
              </div>
              {secretCount > 0 && (
                <div className="flex items-center space-x-1.5 px-2.5 py-1 rounded bg-background border border-border text-red-400">
                  <Lock className="w-3.5 h-3.5" />
                  <span>
                    Secret: <strong>{secretCount}</strong>
                  </span>
                </div>
              )}
              {binaryCount > 0 && (
                <div className="flex items-center space-x-1.5 px-2.5 py-1 rounded bg-background border border-border text-amber-400">
                  <Binary className="w-3.5 h-3.5" />
                  <span>
                    Binary: <strong>{binaryCount}</strong>
                  </span>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Filter Input Bar */}
        <div className="flex items-center space-x-3">
          <div className="relative flex-1 max-w-md">
            <Search className="w-3.5 h-3.5 text-gray-400 absolute left-3 top-2.5" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Filter repository tree by file or folder path..."
              className="w-full bg-background border border-border rounded-md pl-9 pr-3 py-1.5 text-xs text-gray-200 placeholder-gray-500 focus:outline-none focus:border-accent"
            />
            {searchQuery && (
              <button
                onClick={() => setSearchQuery("")}
                className="absolute right-2.5 top-2 text-xs text-gray-400 hover:text-white"
              >
                ×
              </button>
            )}
          </div>
        </div>
      </div>

      {/* Manifest Loading Error Alert */}
      {manifestError && (
        <div className="mx-4 mt-3 border border-red-500/40 bg-red-500/10 text-red-300 rounded-lg p-3.5 flex items-center justify-between shrink-0">
          <div className="flex items-center space-x-2.5">
            <AlertTriangle className="w-4 h-4 text-red-400 shrink-0" />
            <span className="text-xs">{manifestError}</span>
          </div>
          {activeRepoID && (
            <button
              onClick={() => fetchFileManifest(activeRepoID)}
              className="px-2.5 py-1 rounded text-xs bg-red-500/20 border border-red-500/40 hover:bg-red-500/30 text-red-200 font-medium transition flex items-center gap-1"
            >
              <RefreshCw className="w-3 h-3" /> Retry
            </button>
          )}
        </div>
      )}

      {/* Main Split Layout View */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left Pane: Repository Tree Navigation */}
        <div className="w-80 border-r border-border bg-surface flex flex-col shrink-0 overflow-hidden">
          {isLoadingManifest ? (
            <div className="flex-1 flex flex-col items-center justify-center p-6 text-center text-xs text-gray-400 space-y-2">
              <div className="w-5 h-5 border-2 border-accent border-t-transparent rounded-full animate-spin" />
              <span>Loading file manifest...</span>
            </div>
          ) : !activeRepo ? (
            <div className="flex-1 flex flex-col items-center justify-center p-6 text-center text-xs text-gray-400">
              <FolderTree className="w-8 h-8 text-gray-500 mb-2" />
              <span>Select a repository to explore files.</span>
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

        {/* Right Pane: Source Code Viewer */}
        <div className="flex-1 bg-background overflow-hidden">
          <SourceViewer
            source={sourceContent}
            isLoading={isLoadingSource}
            error={sourceError}
            selectedPath={selectedPath}
            manifestItem={selectedManifestItem}
          />
        </div>
      </div>
    </div>
  );
};
