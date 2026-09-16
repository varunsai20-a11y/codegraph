"use client";

import React, { useState } from "react";
import { useAppStore } from "@/store/useAppStore";
import { ExplanationEvidence, Citation } from "@/lib/types";

export const ExplanationPanel: React.FC = () => {
  const {
    activeRepo,
    explanationResult,
    isExplaining,
    explanationError,
    fetchExplanation,
    selectedSymbolID,
    selectedNodeID,
    flowResult,
    selectFileAndHighlight,
    selectGraphNode,
    setActiveTab,
  } = useAppStore();

  const [inputQuery, setInputQuery] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!inputQuery.trim()) return;

    fetchExplanation(inputQuery.trim(), {
      symbolId: selectedSymbolID || selectedNodeID || undefined,
      rootSymbol: flowResult?.root_node_id,
      targetNode: flowResult?.target_node_id,
      flow: !!flowResult,
    });
  };

  const handleQuickExplainSymbol = () => {
    const symId = selectedSymbolID || selectedNodeID;
    if (!symId) return;
    const q = `Explain the purpose and relationships of symbol ID ${symId}.`;
    setInputQuery(q);
    fetchExplanation(q, { symbolId: symId });
  };

  const handleQuickExplainFlow = () => {
    if (!flowResult?.root_node_id) return;
    const q = `Explain the static call flow from ${flowResult.root_node_id}${
      flowResult.target_node_id ? ` to ${flowResult.target_node_id}` : ""
    }.`;
    setInputQuery(q);
    fetchExplanation(q, {
      rootSymbol: flowResult.root_node_id,
      targetNode: flowResult.target_node_id,
      flow: true,
    });
  };

  const handleEvidenceClick = (ev: ExplanationEvidence) => {
    if (ev.relative_path) {
      const line = ev.location?.start_line || 1;
      selectFileAndHighlight(ev.relative_path, line);
      setActiveTab("SYNC");
    } else if (ev.symbol_id) {
      selectGraphNode(ev.symbol_id);
    }
  };

  const handleCitationClick = (cit: Citation) => {
    if (cit.relative_path) {
      const line = cit.location?.start_line || 1;
      selectFileAndHighlight(cit.relative_path, line);
      setActiveTab("SYNC");
    }
  };

  const renderAnswerWithCitations = (answer: string) => {
    if (!answer) return null;
    const parts = answer.split(/(\[E\d+\])/g);
    return parts.map((part, idx) => {
      const match = part.match(/^\[(E\d+)\]$/);
      if (match) {
        const eLabel = match[1];
        const cit = explanationResult?.citations?.find(
          (c) => c.evidence_id === eLabel
        );
        return (
          <button
            key={idx}
            onClick={() => cit && handleCitationClick(cit)}
            className="inline-flex items-center px-1.5 py-0.5 mx-0.5 text-xs font-semibold rounded bg-sky-500/20 text-sky-300 hover:bg-sky-500/30 border border-sky-500/30 transition cursor-pointer"
            title={
              cit
                ? `Jump to ${cit.relative_path}:${cit.location?.start_line || 1}`
                : eLabel
            }
          >
            {part}
          </button>
        );
      }
      return <span key={idx}>{part}</span>;
    });
  };

  if (!activeRepo) {
    return (
      <div className="p-6 text-slate-400 text-sm">
        Select a repository to use AI Explanation.
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full bg-slate-900 text-slate-100 p-4 border-l border-slate-800 overflow-y-auto">
      {/* Header */}
      <div className="flex items-center justify-between pb-3 border-b border-slate-800 mb-4">
        <div className="flex items-center space-x-2">
          <div className="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse" />
          <h2 className="text-sm font-semibold tracking-wide text-slate-200 uppercase">
            Grounded AI Explanation
          </h2>
        </div>
        <span className="text-xs text-slate-400 bg-slate-800 px-2 py-0.5 rounded">
          Source of Truth: CodeGraph
        </span>
      </div>

      {/* Query Form */}
      <form onSubmit={handleSubmit} className="mb-4 space-y-2">
        <div className="relative">
          <textarea
            value={inputQuery}
            onChange={(e) => setInputQuery(e.target.value)}
            placeholder="Ask a question about this repository codebase..."
            rows={3}
            className="w-full bg-slate-950 border border-slate-700 rounded-lg p-3 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 transition resize-none"
          />
        </div>
        <div className="flex items-center justify-between">
          <div className="flex space-x-2">
            {(selectedSymbolID || selectedNodeID) && (
              <button
                type="button"
                onClick={handleQuickExplainSymbol}
                className="text-xs bg-slate-800 hover:bg-slate-700 text-slate-300 px-2.5 py-1 rounded border border-slate-700 transition"
              >
                Explain Selected Symbol
              </button>
            )}
            {flowResult?.root_node_id && (
              <button
                type="button"
                onClick={handleQuickExplainFlow}
                className="text-xs bg-slate-800 hover:bg-slate-700 text-slate-300 px-2.5 py-1 rounded border border-slate-700 transition"
              >
                Explain Static Flow
              </button>
            )}
          </div>
          <button
            type="submit"
            disabled={isExplaining || !inputQuery.trim()}
            className="px-4 py-1.5 text-xs font-semibold rounded-md bg-sky-600 hover:bg-sky-500 disabled:opacity-50 text-white transition shadow-sm"
          >
            {isExplaining ? "Analyzing..." : "Ask AI"}
          </button>
        </div>
      </form>

      {/* Error state */}
      {explanationError && (
        <div className="p-3 mb-4 text-xs bg-rose-950/40 border border-rose-800/60 rounded text-rose-300">
          {explanationError}
        </div>
      )}

      {/* Results */}
      {explanationResult && (
        <div className="space-y-4 flex-1">
          {/* Grounding & Sufficiency Status */}
          <div className="flex items-center justify-between p-2.5 bg-slate-950/60 border border-slate-800 rounded-lg">
            <div className="flex items-center space-x-2">
              <span
                className={`text-xs font-bold px-2 py-0.5 rounded ${
                  explanationResult.is_insufficient_evidence
                    ? "bg-amber-500/20 text-amber-400 border border-amber-500/30"
                    : explanationResult.grounding?.status === "GROUNDING_PASSED"
                    ? "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30"
                    : "bg-sky-500/20 text-sky-400 border border-sky-500/30"
                }`}
              >
                {explanationResult.is_insufficient_evidence
                  ? "INSUFFICIENT EVIDENCE"
                  : explanationResult.grounding?.status || "GROUNDED"}
              </span>
            </div>
            <div className="text-[11px] text-slate-400 space-x-2">
              <span>
                Evidence: {explanationResult.grounding?.evidence_count || 0}
              </span>
              <span>•</span>
              <span>
                Citations: {explanationResult.grounding?.cited_evidence_count || 0}
              </span>
              {explanationResult.latency_ms !== undefined && (
                <>
                  <span>•</span>
                  <span>{explanationResult.latency_ms}ms</span>
                </>
              )}
            </div>
          </div>

          {/* Answer Content */}
          <div className="p-3 bg-slate-950/80 border border-slate-800 rounded-lg">
            <h3 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
              Explanation Answer
            </h3>
            <div className="text-xs text-slate-200 leading-relaxed whitespace-pre-wrap">
              {renderAnswerWithCitations(explanationResult.answer)}
            </div>
          </div>

          {/* Evidence List */}
          {explanationResult.evidence && explanationResult.evidence.length > 0 && (
            <div className="p-3 bg-slate-950/80 border border-slate-800 rounded-lg">
              <h3 className="text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
                Grounding Evidence ({explanationResult.evidence.length})
              </h3>
              <div className="space-y-2">
                {explanationResult.evidence.map((item, idx) => (
                  <div
                    key={idx}
                    onClick={() => handleEvidenceClick(item)}
                    className="p-2 bg-slate-900 hover:bg-slate-850 border border-slate-800 rounded cursor-pointer transition group"
                  >
                    <div className="flex items-center justify-between mb-1">
                      <span className="text-xs font-bold text-sky-400 group-hover:underline">
                        {item.label || `E${idx + 1}`} {item.relative_path}
                        {item.location?.start_line
                          ? `:${item.location.start_line}`
                          : ""}
                      </span>
                      <span className="text-[10px] text-slate-500 uppercase px-1.5 py-0.2 bg-slate-800 rounded">
                        {item.retriever_type || item.type}
                      </span>
                    </div>
                    <pre className="text-[11px] text-slate-300 font-mono bg-slate-950 p-1.5 rounded overflow-x-auto whitespace-pre-wrap">
                      {item.content}
                    </pre>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
};
