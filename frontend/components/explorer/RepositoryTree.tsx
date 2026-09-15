"use client";

import React, { useMemo, useState, useEffect } from "react";
import { FileManifestItem, TreeNode as TreeNodeType } from "@/lib/types";
import { TreeNode } from "./TreeNode";
import { FolderTree, ChevronsUpDown, FolderClosed } from "lucide-react";

interface RepositoryTreeProps {
  manifest: FileManifestItem[];
  searchQuery: string;
  expandedPaths: Set<string>;
  selectedPath: string | null;
  onToggleExpand: (path: string) => void;
  onSetExpandedPaths: (paths: Set<string>) => void;
  onSelectFile: (path: string) => void;
}

export const RepositoryTree: React.FC<RepositoryTreeProps> = ({
  manifest,
  searchQuery,
  expandedPaths,
  selectedPath,
  onToggleExpand,
  onSetExpandedPaths,
  onSelectFile,
}) => {
  const [focusedPath, setFocusedPath] = useState<string | null>(null);

  // Build hierarchical tree deterministically (folders first, then files)
  const treeNodes = useMemo(() => {
    const rootNodes: Map<string, TreeNodeType> = new Map();

    const query = searchQuery.trim().toLowerCase();
    const filteredManifest = query
      ? manifest.filter(
          (item) =>
            item.relative_path.toLowerCase().includes(query) ||
            item.relative_path.split("/").pop()?.toLowerCase().includes(query)
        )
      : manifest;

    for (const item of filteredManifest) {
      const parts = item.relative_path.split("/").filter(Boolean);
      let currentPath = "";
      let currentMap = rootNodes;
      let parentNode: TreeNodeType | null = null;

      for (let i = 0; i < parts.length; i++) {
        const part = parts[i];
        const isLast = i === parts.length - 1;
        currentPath = currentPath ? `${currentPath}/${part}` : part;

        let existing = currentMap.get(part);

        if (!existing) {
          existing = {
            name: part,
            path: currentPath,
            isFolder: !isLast,
            children: !isLast ? [] : undefined,
            fileItem: isLast ? item : undefined,
          };
          currentMap.set(part, existing);
          if (parentNode && parentNode.children) {
            parentNode.children.push(existing);
          }
        }

        if (!isLast) {
          if (!existing.children) existing.children = [];
          parentNode = existing;
          const childMap = new Map<string, TreeNodeType>();
          for (const child of existing.children) {
            childMap.set(child.name, child);
          }
          currentMap = childMap;
        }
      }
    }

    const sortTree = (nodes: TreeNodeType[]): TreeNodeType[] => {
      const folders: TreeNodeType[] = [];
      const files: TreeNodeType[] = [];

      for (const node of nodes) {
        if (node.isFolder) {
          if (node.children) {
            node.children = sortTree(node.children);
          }
          folders.push(node);
        } else {
          files.push(node);
        }
      }

      folders.sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" }));
      files.sort((a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" }));

      return [...folders, ...files];
    };

    return sortTree(Array.from(rootNodes.values()));
  }, [manifest, searchQuery]);

  // Expand parent folders automatically when filtering
  useEffect(() => {
    if (searchQuery.trim()) {
      const allFolderPaths = new Set<string>();
      const collectFolders = (nodes: TreeNodeType[]) => {
        for (const n of nodes) {
          if (n.isFolder) {
            allFolderPaths.add(n.path);
            if (n.children) collectFolders(n.children);
          }
        }
      };
      collectFolders(treeNodes);
      onSetExpandedPaths(allFolderPaths);
    }
  }, [searchQuery, treeNodes, onSetExpandedPaths]);

  // Flatten visible nodes for keyboard navigation
  const visibleNodes = useMemo(() => {
    const result: TreeNodeType[] = [];
    const traverse = (nodes: TreeNodeType[]) => {
      for (const node of nodes) {
        result.push(node);
        if (node.isFolder && expandedPaths.has(node.path) && node.children) {
          traverse(node.children);
        }
      }
    };
    traverse(treeNodes);
    return result;
  }, [treeNodes, expandedPaths]);

  const handleExpandAll = () => {
    const allFolderPaths = new Set<string>();
    const collectFolders = (nodes: TreeNodeType[]) => {
      for (const n of nodes) {
        if (n.isFolder) {
          allFolderPaths.add(n.path);
          if (n.children) collectFolders(n.children);
        }
      }
    };
    const gatherRootFolders = (nodes: TreeNodeType[]) => {
      for (const n of nodes) {
        if (n.isFolder) {
          allFolderPaths.add(n.path);
          if (n.children) collectFolders(n.children);
        }
      }
    };
    gatherRootFolders(treeNodes);
    onSetExpandedPaths(allFolderPaths);
  };

  const handleCollapseAll = () => {
    onSetExpandedPaths(new Set());
  };

  const handleKeyDownNav = (e: React.KeyboardEvent, currentNode: TreeNodeType) => {
    const currentIndex = visibleNodes.findIndex((n) => n.path === currentNode.path);
    if (currentIndex === -1) return;

    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (currentIndex < visibleNodes.length - 1) {
        setFocusedPath(visibleNodes[currentIndex + 1].path);
      }
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (currentIndex > 0) {
        setFocusedPath(visibleNodes[currentIndex - 1].path);
      }
    } else if (e.key === "ArrowRight") {
      e.preventDefault();
      if (currentNode.isFolder) {
        if (!expandedPaths.has(currentNode.path)) {
          onToggleExpand(currentNode.path);
        } else if (currentNode.children && currentNode.children.length > 0) {
          setFocusedPath(currentNode.children[0].path);
        }
      }
    } else if (e.key === "ArrowLeft") {
      e.preventDefault();
      if (currentNode.isFolder && expandedPaths.has(currentNode.path)) {
        onToggleExpand(currentNode.path);
      } else {
        // Move focus to parent folder
        const parts = currentNode.path.split("/");
        if (parts.length > 1) {
          parts.pop();
          setFocusedPath(parts.join("/"));
        }
      }
    }
  };

  if (manifest.length === 0) {
    return (
      <div className="p-6 text-center text-xs text-gray-400 border border-dashed border-border rounded-md my-4">
        <FolderClosed className="w-6 h-6 mx-auto mb-2 text-gray-500" />
        No files discovered in this repository yet.
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full select-none">
      {/* Controls Header */}
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-border bg-background text-[11px] text-gray-400">
        <span className="font-semibold text-gray-300 flex items-center gap-1.5">
          <FolderTree className="w-3.5 h-3.5 text-accent" />
          Files ({manifest.length})
        </span>
        <div className="flex items-center space-x-1">
          <button
            onClick={handleExpandAll}
            className="px-2 py-0.5 rounded border border-border hover:bg-surface-hover hover:text-white transition"
            title="Expand All Directories"
          >
            Expand
          </button>
          <button
            onClick={handleCollapseAll}
            className="px-2 py-0.5 rounded border border-border hover:bg-surface-hover hover:text-white transition"
            title="Collapse All Directories"
          >
            Collapse
          </button>
        </div>
      </div>

      {/* Tree Content Area */}
      <div role="tree" aria-label="Repository Tree" className="flex-1 overflow-y-auto p-2 space-y-0.5">
        {treeNodes.length === 0 ? (
          <div className="py-6 text-center text-xs text-gray-400">
            No matching files found for &quot;{searchQuery}&quot;.
          </div>
        ) : (
          treeNodes.map((node) => (
            <TreeNode
              key={node.path}
              node={node}
              depth={0}
              expandedPaths={expandedPaths}
              selectedPath={selectedPath}
              focusedPath={focusedPath}
              onToggleExpand={onToggleExpand}
              onSelectFile={onSelectFile}
              onFocusNode={setFocusedPath}
              onKeyDownNav={handleKeyDownNav}
            />
          ))
        )}
      </div>
    </div>
  );
};
