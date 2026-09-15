"use client";

import React from "react";
import { TreeNode as TreeNodeType } from "@/lib/types";
import {
  ChevronRight,
  ChevronDown,
  Folder,
  FolderOpen,
  FileCode,
  FileText,
  Lock,
  Binary,
  AlertCircle,
  FileX,
} from "lucide-react";

interface TreeNodeProps {
  node: TreeNodeType;
  depth: number;
  expandedPaths: Set<string>;
  selectedPath: string | null;
  focusedPath: string | null;
  onToggleExpand: (path: string) => void;
  onSelectFile: (path: string) => void;
  onFocusNode: (path: string) => void;
  onKeyDownNav: (e: React.KeyboardEvent, node: TreeNodeType) => void;
}

export const TreeNode: React.FC<TreeNodeProps> = ({
  node,
  depth,
  expandedPaths,
  selectedPath,
  focusedPath,
  onToggleExpand,
  onSelectFile,
  onFocusNode,
  onKeyDownNav,
}) => {
  const isExpanded = expandedPaths.has(node.path);
  const isSelected = selectedPath === node.path;
  const isFocused = focusedPath === node.path;

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    onFocusNode(node.path);
    if (node.isFolder) {
      onToggleExpand(node.path);
    } else {
      onSelectFile(node.path);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      e.stopPropagation();
      if (node.isFolder) {
        onToggleExpand(node.path);
      } else {
        onSelectFile(node.path);
      }
      return;
    }
    onKeyDownNav(e, node);
  };

  const status = node.fileItem?.status;

  const renderStatusBadge = () => {
    if (!status || status === "INDEXED") return null;

    switch (status) {
      case "SECRET":
        return (
          <span className="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-red-500/15 border border-red-500/30 text-red-400 font-mono shrink-0 ml-auto">
            <Lock className="w-2.5 h-2.5" /> SECRET
          </span>
        );
      case "BINARY":
        return (
          <span className="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-amber-500/15 border border-amber-500/30 text-amber-400 font-mono shrink-0 ml-auto">
            <Binary className="w-2.5 h-2.5" /> BINARY
          </span>
        );
      case "IGNORED":
        return (
          <span className="inline-flex items-center text-[10px] px-1.5 py-0.5 rounded bg-gray-500/10 border border-gray-500/20 text-gray-400 font-mono shrink-0 ml-auto">
            IGNORED
          </span>
        );
      case "FAILED":
        return (
          <span className="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-red-500/15 border border-red-500/30 text-red-400 font-mono shrink-0 ml-auto">
            <AlertCircle className="w-2.5 h-2.5" /> FAILED
          </span>
        );
      case "OVERSIZED":
      case "UNSUPPORTED":
        return (
          <span className="inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-gray-500/15 border border-gray-500/30 text-gray-300 font-mono shrink-0 ml-auto">
            <FileX className="w-2.5 h-2.5" /> {status}
          </span>
        );
      default:
        return null;
    }
  };

  const getFileIcon = () => {
    if (status === "SECRET") return <Lock className="w-3.5 h-3.5 text-red-400 shrink-0" />;
    if (status === "BINARY") return <Binary className="w-3.5 h-3.5 text-amber-400 shrink-0" />;

    const ext = node.name.slice(node.name.lastIndexOf(".")).toLowerCase();
    switch (ext) {
      case ".go":
      case ".ts":
      case ".tsx":
      case ".js":
      case ".jsx":
      case ".py":
      case ".java":
        return <FileCode className="w-3.5 h-3.5 text-accent shrink-0" />;
      default:
        return <FileText className="w-3.5 h-3.5 text-gray-400 shrink-0" />;
    }
  };

  return (
    <div role="treeitem" aria-expanded={node.isFolder ? isExpanded : undefined}>
      <div
        tabIndex={isFocused ? 0 : -1}
        onClick={handleClick}
        onKeyDown={handleKeyDown}
        onFocus={() => onFocusNode(node.path)}
        style={{ paddingLeft: `${depth * 12 + 8}px` }}
        className={`group flex items-center gap-1.5 py-1 pr-2 rounded-md text-xs font-medium cursor-pointer transition select-none outline-none ${
          isSelected
            ? "bg-accent/20 text-white font-semibold border-l-2 border-accent"
            : isFocused
            ? "bg-surface-hover text-gray-100 ring-1 ring-accent/40"
            : "text-gray-300 hover:bg-surface-hover hover:text-white"
        }`}
      >
        {node.isFolder ? (
          <>
            <span className="text-gray-400 group-hover:text-gray-200 transition shrink-0">
              {isExpanded ? (
                <ChevronDown className="w-3.5 h-3.5" />
              ) : (
                <ChevronRight className="w-3.5 h-3.5" />
              )}
            </span>
            <span className="text-amber-400/90 shrink-0">
              {isExpanded ? (
                <FolderOpen className="w-3.5 h-3.5" />
              ) : (
                <Folder className="w-3.5 h-3.5" />
              )}
            </span>
          </>
        ) : (
          <>
            <span className="w-3.5 shrink-0" />
            {getFileIcon()}
          </>
        )}

        <span className="truncate flex-1">{node.name}</span>

        {renderStatusBadge()}
      </div>

      {node.isFolder && isExpanded && node.children && node.children.length > 0 && (
        <div role="group" className="flex flex-col">
          {node.children.map((child) => (
            <TreeNode
              key={child.path}
              node={child}
              depth={depth + 1}
              expandedPaths={expandedPaths}
              selectedPath={selectedPath}
              focusedPath={focusedPath}
              onToggleExpand={onToggleExpand}
              onSelectFile={onSelectFile}
              onFocusNode={onFocusNode}
              onKeyDownNav={onKeyDownNav}
            />
          ))}
        </div>
      )}
    </div>
  );
};
