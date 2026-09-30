"use client";

import React, { useState } from "react";
import { useAppStore } from "@/store/useAppStore";
import { Bot, Terminal, FileText, Code2, ArrowRight } from "lucide-react";

export const PersistentAIBanner: React.FC = () => {
  const {
    activeRepo,
    selectedPath,
    selectedSymbolID,
    selectedNodeID,
    graphNodes,
    isExplaining,
    fetchExplanation,
    setActiveTab,
    flowResult,
  } = useAppStore();

  const [promptInput, setPromptInput] = useState("");

  if (!activeRepo) return null;

  const currentSymbolNode = graphNodes.find(
    (n) => n.id === (selectedSymbolID || selectedNodeID)
  );

  const symbolLabel = currentSymbolNode?.label || (selectedSymbolID || selectedNodeID);

  const handleAsk = (e: React.FormEvent) => {
    e.preventDefault();
    if (!promptInput.trim() || isExplaining) return;

    const q = promptInput.trim();
    setPromptInput("");
    fetchExplanation(q, {
      symbolId: selectedSymbolID || selectedNodeID || undefined,
      rootSymbol: flowResult?.root_node_id,
      targetNode: flowResult?.target_node_id,
      flow: !!flowResult,
    });
    setActiveTab("AI");
  };

  return (
    <div className="border-t border-border bg-surface px-3 py-2 flex items-center justify-between text-xs font-mono shrink-0 select-none">
      {/* Context Badge Group */}
      <div className="flex items-center space-x-2 mr-3 shrink-0">
        <div className="flex items-center space-x-1.5 px-2 py-0.5 rounded-sm bg-violet-950/50 border border-violet-500/40 text-violet-300 font-bold text-[10px]">
          <Bot className="w-3 h-3 text-violet-400" />
          <span>AI CONTEXT</span>
        </div>

        <div className="hidden md:flex items-center space-x-1 text-[10px] text-gray-400">
          <span className="px-1.5 py-0.5 rounded-sm bg-background border border-border text-gray-300 flex items-center gap-1">
            <Terminal className="w-2.5 h-2.5 text-cyan-400" />
            <span className="truncate max-w-[100px]">{activeRepo.name}</span>
          </span>

          {selectedPath && (
            <>
              <span className="text-gray-600">/</span>
              <span className="px-1.5 py-0.5 rounded-sm bg-background border border-border text-cyan-300 flex items-center gap-1 truncate max-w-[140px]">
                <FileText className="w-2.5 h-2.5 text-cyan-400" />
                <span className="truncate">{selectedPath}</span>
              </span>
            </>
          )}

          {symbolLabel && (
            <>
              <span className="text-gray-600">/</span>
              <span className="px-1.5 py-0.5 rounded-sm bg-cyan-950/60 border border-cyan-500/40 text-cyan-300 flex items-center gap-1 truncate max-w-[140px]">
                <Code2 className="w-2.5 h-2.5 text-cyan-400" />
                <span className="truncate">{symbolLabel}</span>
              </span>
            </>
          )}
        </div>
      </div>

      {/* Input Form */}
      <form onSubmit={handleAsk} className="flex-1 flex items-center space-x-2">
        <input
          type="text"
          value={promptInput}
          onChange={(e) => setPromptInput(e.target.value)}
          placeholder={
            symbolLabel
              ? `Ask AI about symbol ${symbolLabel}...`
              : selectedPath
              ? `Ask AI about file ${selectedPath}...`
              : `Ask AI about ${activeRepo.name} repository...`
          }
          className="flex-1 bg-background border border-border rounded-sm px-3 py-1 text-xs text-gray-200 placeholder-gray-500 font-mono focus:outline-none focus:border-violet-500/80 transition"
        />
        <button
          type="submit"
          disabled={isExplaining || !promptInput.trim()}
          className="px-3 py-1 rounded-sm bg-violet-950/60 hover:bg-violet-900/80 border border-violet-500/40 text-violet-200 text-xs font-mono font-bold transition disabled:opacity-40 flex items-center space-x-1 shrink-0"
        >
          <span>{isExplaining ? "Analyzing..." : "Ask AI"}</span>
          <ArrowRight className="w-3 h-3 text-violet-400" />
        </button>
      </form>
    </div>
  );
};
