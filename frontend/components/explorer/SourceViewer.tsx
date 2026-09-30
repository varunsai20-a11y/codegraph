"use client";

import React from "react";
import { FileContentResponse, FileManifestItem } from "@/lib/types";
import { FileText, ShieldAlert, Binary, AlertTriangle, FileCode2, Copy, Check, Network, Bot, MapPin } from "lucide-react";
import { useAppStore } from "@/store/useAppStore";

interface SourceViewerProps {
  source: FileContentResponse | null;
  isLoading: boolean;
  error: string | null;
  selectedPath: string | null;
  manifestItem?: FileManifestItem;
  highlightLineRange?: [number, number] | null;
  onViewInGraph?: () => void;
}

export const SourceViewer: React.FC<SourceViewerProps> = ({
  source,
  isLoading,
  error,
  selectedPath,
  manifestItem,
  highlightLineRange,
  onViewInGraph,
}) => {
  const [copied, setCopied] = React.useState(false);
  const startRowRef = React.useRef<HTMLTableRowElement | null>(null);
  const { fetchExplanation, setActiveTab, selectedSymbolID } = useAppStore();

  const handleCopy = () => {
    if (source?.content) {
      navigator.clipboard.writeText(source.content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleAskAIAboutCode = () => {
    if (!selectedPath) return;
    const lineStr = highlightLineRange ? ` (Lines ${highlightLineRange[0]}–${highlightLineRange[1]})` : "";
    const q = `Explain the logic, dependencies, and security implications of file ${selectedPath}${lineStr}.`;
    fetchExplanation(q, {
      symbolId: selectedSymbolID || undefined,
    });
    setActiveTab("AI");
  };

  const formatFileSize = (bytes?: number) => {
    if (bytes === undefined) return "";
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const lines = source?.content ? source.content.split("\n") : [];
  const totalLines = lines.length;

  const validRange = React.useMemo(() => {
    if (!highlightLineRange) return null;
    const [start, end] = highlightLineRange;
    if (start < 1 || end < start || start > totalLines) return null;
    return [start, Math.min(end, totalLines)] as [number, number];
  }, [highlightLineRange, totalLines]);

  React.useEffect(() => {
    if (validRange && startRowRef.current) {
      startRowRef.current.scrollIntoView({
        behavior: "smooth",
        block: "center",
      });
    }
  }, [validRange, selectedPath]);

  if (!selectedPath) {
    return (
      <div className="h-full flex flex-col items-center justify-center text-center p-8 bg-background font-mono select-none">
        <div className="w-10 h-10 rounded-sm bg-surface border border-border flex items-center justify-center text-gray-500 mb-3">
          <FileCode2 className="w-5 h-5 text-cyan-400" />
        </div>
        <h3 className="text-xs font-bold text-gray-300 uppercase tracking-wider">NO FILE SELECTED</h3>
        <p className="text-[11px] text-gray-500 max-w-sm mt-1 leading-relaxed">
          Select a file from the repository tree to view AST source code and symbol definitions.
        </p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background font-mono">
        <div className="w-5 h-5 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin mb-3" />
        <p className="text-xs text-gray-400">Loading source for {selectedPath}...</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-full p-6 bg-background overflow-y-auto font-mono">
        <div className="border border-red-500/40 bg-red-950/20 text-red-300 rounded-sm p-4 flex items-start space-x-3">
          <AlertTriangle className="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
          <div>
            <h3 className="text-xs font-bold text-red-200 uppercase">Error Reading Source File</h3>
            <p className="text-[11px] text-red-400 mt-1">{selectedPath}</p>
            <p className="text-[11px] text-red-300/80 mt-2">{error}</p>
          </div>
        </div>
      </div>
    );
  }

  if (manifestItem?.status === "SECRET" || source?.language === "SECURITY") {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background font-mono select-none">
        <div className="w-10 h-10 rounded-sm bg-red-950/40 border border-red-500/30 flex items-center justify-center text-red-400 mb-3">
          <ShieldAlert className="w-5 h-5" />
        </div>
        <h3 className="text-xs font-bold text-red-300 uppercase">Security Restriction</h3>
        <p className="text-[11px] text-red-400/80 max-w-md mt-1">
          Source code protected or suppressed for security compliance.
        </p>
        <code className="mt-3 text-[10px] text-gray-400 bg-surface px-2.5 py-1 rounded-sm border border-border">
          {selectedPath}
        </code>
      </div>
    );
  }

  if (manifestItem?.status === "BINARY" || source?.language === "BINARY") {
    return (
      <div className="h-full flex flex-col items-center justify-center p-8 bg-background font-mono select-none">
        <div className="w-10 h-10 rounded-sm bg-amber-950/40 border border-amber-500/30 flex items-center justify-center text-amber-400 mb-3">
          <Binary className="w-5 h-5" />
        </div>
        <h3 className="text-xs font-bold text-amber-300 uppercase">Binary File</h3>
        <p className="text-[11px] text-amber-400/80 max-w-md mt-1">
          Binary file contents cannot be inspected in plain text.
        </p>
        <code className="mt-3 text-[10px] text-gray-400 bg-surface px-2.5 py-1 rounded-sm border border-border">
          {selectedPath} ({formatFileSize(manifestItem?.size)})
        </code>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-background overflow-hidden select-text font-mono">
      {/* Code Header Bar */}
      <div className="h-10 border-b border-border bg-surface px-3 flex items-center justify-between shrink-0 select-none text-xs">
        <div className="flex items-center space-x-2 min-w-0">
          <FileText className="w-3.5 h-3.5 text-cyan-400 shrink-0" />
          <span className="text-xs font-mono text-gray-100 truncate font-bold" title={selectedPath}>
            {selectedPath}
          </span>
        </div>

        <div className="flex items-center space-x-2 text-[11px] shrink-0 font-mono">
          <button
            onClick={handleAskAIAboutCode}
            className="flex items-center space-x-1 px-2.5 py-0.5 rounded-sm border border-violet-500/40 bg-violet-950/40 hover:bg-violet-900/50 text-violet-200 font-bold transition"
            title="Ask AI to analyze this source file"
          >
            <Bot className="w-3 h-3 text-violet-400" />
            <span>Ask AI</span>
          </button>

          {onViewInGraph && (
            <button
              onClick={onViewInGraph}
              className="flex items-center space-x-1 px-2.5 py-0.5 rounded-sm border border-border bg-background hover:bg-surface-hover text-gray-300 font-bold transition"
              title="Return to Graph view"
            >
              <Network className="w-3 h-3 text-cyan-400" />
              <span>Show in Graph</span>
            </button>
          )}

          {validRange && (
            <span className="text-[10px] text-cyan-300 font-bold px-1.5 py-0.5 bg-cyan-950/60 border border-cyan-500/40 rounded-sm">
              L{validRange[0]}–L{validRange[1]}
            </span>
          )}

          {source?.language && (
            <span className="text-[9px] uppercase font-bold tracking-wider px-1.5 py-0.5 rounded-sm bg-white/5 border border-white/10 text-gray-400 font-mono">
              {source.language}
            </span>
          )}

          {source?.total_lines !== undefined && (
            <span className="text-[10px] text-gray-500 font-mono">
              <strong className="text-gray-300">{source.total_lines}</strong> lines
            </span>
          )}

          <button
            onClick={handleCopy}
            disabled={!source?.content}
            className="p-1 rounded-sm border border-border hover:bg-surface-hover text-gray-400 hover:text-white transition"
            title="Copy Source Code"
          >
            {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
          </button>
        </div>
      </div>

      {/* Code Lines Table */}
      <div className="flex-1 overflow-auto text-[11px] leading-5 bg-[#08090d]">
        {lines.length === 0 ? (
          <div className="p-4 text-gray-600 italic">Empty file</div>
        ) : (
          <table className="w-full border-collapse font-mono">
            <tbody>
              {lines.map((lineText, idx) => {
                const lineNumber = idx + 1;
                const isHighlighted = validRange
                  ? lineNumber >= validRange[0] && lineNumber <= validRange[1]
                  : false;

                const isStartLine = validRange && lineNumber === validRange[0];

                return (
                  <tr
                    key={lineNumber}
                    ref={isStartLine ? startRowRef : undefined}
                    className={
                      isHighlighted
                        ? "bg-cyan-950/40 border-l-2 border-cyan-400 text-cyan-100 font-semibold"
                        : "hover:bg-surface-hover/60 group"
                    }
                  >
                    <td
                      className={
                        isHighlighted
                          ? "w-12 text-right pr-3 py-0.5 text-cyan-400 select-none bg-cyan-950/60 border-r border-cyan-500/30 text-[10px] font-bold"
                          : "w-12 text-right pr-3 py-0.5 text-gray-600 select-none bg-surface/30 border-r border-border/40 text-[10px] group-hover:text-gray-400"
                      }
                    >
                      {lineNumber}
                    </td>
                    <td className="pl-3 pr-3 py-0.5 text-gray-300 whitespace-pre">
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
