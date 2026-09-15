"use client";

import React from "react";
import { FileContentResponse, FileManifestItem } from "@/lib/types";
import { FileText, ShieldAlert, Binary, AlertTriangle, FileCode2, Copy, Check } from "lucide-react";

interface SourceViewerProps {
  source: FileContentResponse | null;
  isLoading: boolean;
  error: string | null;
  selectedPath: string | null;
  manifestItem?: FileManifestItem;
}

export const SourceViewer: React.FC<SourceViewerProps> = ({
  source,
  isLoading,
  error,
  selectedPath,
  manifestItem,
}) => {
  const [copied, setCopied] = React.useState(false);

  const handleCopy = () => {
    if (source?.content) {
      navigator.clipboard.writeText(source.content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const formatFileSize = (bytes?: number) => {
    if (bytes === undefined) return "";
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  if (!selectedPath) {
    return (
      <div className="h-full flex flex-col items-center justify-center text-center p-8 bg-background select-none">
        <div className="w-12 h-12 rounded-full bg-surface border border-border flex items-center justify-center text-gray-400 mb-3">
          <FileCode2 className="w-6 h-6 text-accent/80" />
        </div>
        <h3 className="text-sm font-semibold text-gray-200">No File Selected</h3>
        <p className="text-xs text-gray-400 max-w-sm mt-1">
          Select a file from the repository tree on the left to inspect its source code.
        </p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background">
        <div className="w-6 h-6 border-2 border-accent border-t-transparent rounded-full animate-spin mb-3" />
        <p className="text-xs text-gray-400">Loading source for {selectedPath}...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-full p-6 bg-background overflow-y-auto">
        <div className="border border-red-500/40 bg-red-500/10 text-red-300 rounded-lg p-5 flex items-start space-x-3">
          <AlertTriangle className="w-5 h-5 text-red-400 shrink-0 mt-0.5" />
          <div>
            <h3 className="text-sm font-semibold text-red-200">Error Reading Source File</h3>
            <p className="text-xs text-red-300/90 mt-1 font-mono">{selectedPath}</p>
            <p className="text-xs text-red-400/90 mt-2">{error}</p>
          </div>
        </div>
      </div>
    );
  }

  if (manifestItem?.status === "SECRET" || source?.language === "SECURITY") {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background select-none">
        <div className="w-12 h-12 rounded-full bg-red-500/10 border border-red-500/30 flex items-center justify-center text-red-400 mb-3">
          <ShieldAlert className="w-6 h-6" />
        </div>
        <h3 className="text-sm font-bold text-red-300">Security Restriction</h3>
        <p className="text-xs text-red-400/90 max-w-md mt-1">
          Source unavailable for security reasons.
        </p>
        <code className="mt-3 text-[11px] font-mono text-gray-400 bg-surface px-3 py-1 rounded border border-border">
          {selectedPath}
        </code>
      </div>
    );
  }

  if (manifestItem?.status === "BINARY" || source?.language === "BINARY") {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background select-none">
        <div className="w-12 h-12 rounded-full bg-amber-500/10 border border-amber-500/30 flex items-center justify-center text-amber-400 mb-3">
          <Binary className="w-6 h-6" />
        </div>
        <h3 className="text-sm font-bold text-amber-300">Binary File</h3>
        <p className="text-xs text-amber-400/90 max-w-md mt-1">
          Binary file — source preview unavailable.
        </p>
        <code className="mt-3 text-[11px] font-mono text-gray-400 bg-surface px-3 py-1 rounded border border-border">
          {selectedPath} ({formatFileSize(manifestItem?.size)})
        </code>
      </div>
    );
  }

  const lines = source?.content ? source.content.split("\n") : [];

  return (
    <div className="h-full flex flex-col bg-background overflow-hidden select-text">
      {/* File Header Bar */}
      <div className="h-11 border-b border-border bg-surface px-4 flex items-center justify-between shrink-0 select-none">
        <div className="flex items-center space-x-2 min-w-0">
          <FileText className="w-4 h-4 text-accent shrink-0" />
          <span className="text-xs font-mono text-gray-200 truncate font-semibold" title={selectedPath}>
            {selectedPath}
          </span>
        </div>

        <div className="flex items-center space-x-3 text-xs shrink-0">
          {source?.language && (
            <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded bg-accent/15 border border-accent/30 text-accent">
              {source.language}
            </span>
          )}

          {source?.total_lines !== undefined && (
            <span className="text-[11px] text-gray-400">
              <strong className="text-gray-200">{source.total_lines}</strong> lines
            </span>
          )}

          {manifestItem?.size !== undefined && (
            <span className="text-[11px] text-gray-400">
              {formatFileSize(manifestItem.size)}
            </span>
          )}

          <button
            onClick={handleCopy}
            disabled={!source?.content}
            className="p-1.5 rounded border border-border hover:bg-surface-hover text-gray-300 hover:text-white transition"
            title="Copy Source Code"
          >
            {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
          </button>
        </div>
      </div>

      {/* Code Text Grid */}
      <div className="flex-1 overflow-auto font-mono text-xs leading-5 p-0 bg-[#0d1117]">
        {lines.length === 0 ? (
          <div className="p-4 text-gray-500 italic">Empty file</div>
        ) : (
          <table className="w-full border-collapse">
            <tbody>
              {lines.map((lineText, idx) => {
                const lineNumber = idx + 1;
                return (
                  <tr key={lineNumber} className="hover:bg-surface-hover/50 group">
                    <td className="w-12 text-right pr-4 py-0.5 text-gray-500 select-none bg-surface/30 border-r border-border/50 text-[11px] group-hover:text-gray-300">
                      {lineNumber}
                    </td>
                    <td className="pl-4 pr-4 py-0.5 text-gray-200 whitespace-pre">
                      {lineText}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
};
