"use client";
import React, { useEffect } from "react";
import { useAppStore } from "@/store/useAppStore";
import {
  Compass,
  Play,
  RotateCcw,
  CheckCircle2,
  Circle,
  ArrowRight,
  HelpCircle,
  FileCode,
  Layers,
  Activity,
  ChevronRight,
  BookOpen,
  Sparkles,
} from "lucide-react";

export const GuideView: React.FC = () => {
  const activeRepoID = useAppStore((state) => state.activeRepoID);
  const activeRepo = useAppStore((state) => state.activeRepo);
  const investigation = useAppStore((state) => state.investigation);
  const isGuiding = useAppStore((state) => state.isGuiding);
  const guideError = useAppStore((state) => state.guideError);
  const fetchGuide = useAppStore((state) => state.fetchGuide);
  const selectGuideStep = useAppStore((state) => state.selectGuideStep);
  const fetchExplanation = useAppStore((state) => state.fetchExplanation);
  const setActiveTab = useAppStore((state) => state.setActiveTab);
  const traceFlowFromSymbol = useAppStore((state) => state.traceFlowFromSymbol);
  const selectFileAndHighlight = useAppStore((state) => state.selectFileAndHighlight);
  const selectGraphNode = useAppStore((state) => state.selectGraphNode);

  useEffect(() => {
    if (activeRepoID && !investigation && !isGuiding) {
      fetchGuide("INITIALIZE");
    }
  }, [activeRepoID, investigation, isGuiding, fetchGuide]);

  if (!activeRepoID) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-gray-400 font-mono select-none">
        <p>Please select a repository to launch Guided Reverse Engineering.</p>
      </div>
    );
  }

  const handleStartOrReset = () => {
    fetchGuide("INITIALIZE");
  };

  const handleNextStep = () => {
    fetchGuide("NEXT_STEP");
  };

  const handleQuestionClick = (question: string) => {
    const step = investigation?.current_step;
    fetchExplanation(question, {
      symbolId: step?.target_symbol || undefined,
      targetNode: step?.target_node || undefined,
    });
    setActiveTab("AI");
  };

  const summary = investigation?.architecture_summary;
  const currentStep = investigation?.current_step;
  const steps = investigation?.steps || [];

  return (
    <div className="flex flex-col h-full bg-background text-gray-200 overflow-y-auto p-5 space-y-5 font-mono select-text">
      {/* Header Banner */}
      <div className="flex items-center justify-between border-b border-border pb-3.5 select-none">
        <div>
          <div className="flex items-center space-x-2">
            <Compass className="h-5 w-5 text-cyan-400" />
            <h1 className="text-sm font-bold text-gray-100 font-mono uppercase tracking-wider">
              Guided Reverse Engineering
            </h1>
            <span className="text-[9px] bg-cyan-950/70 text-cyan-300 border border-cyan-500/40 px-2 py-0.2 rounded-sm font-mono font-bold">
              C7 GROUNDED GUIDE
            </span>
          </div>
          <p className="text-[11px] text-gray-500 mt-1 font-mono">
            Deterministic architectural discovery & step-by-step code investigation for{" "}
            <span className="font-mono text-gray-200 font-bold">{activeRepo?.name || activeRepoID}</span>
          </p>
        </div>

        <div className="flex items-center space-x-2">
          <button
            onClick={handleStartOrReset}
            disabled={isGuiding}
            className="flex items-center space-x-1.5 px-3 py-1 bg-surface hover:bg-surface-hover text-gray-300 text-xs font-mono font-bold rounded-sm border border-border disabled:opacity-50 transition"
          >
            <RotateCcw className="h-3.5 w-3.5" />
            <span>Reset Guide</span>
          </button>
          <button
            onClick={handleNextStep}
            disabled={isGuiding || investigation?.status === "COMPLETED"}
            className="flex items-center space-x-1.5 px-3.5 py-1 bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-mono font-bold rounded-sm disabled:opacity-50 transition shadow-sm"
          >
            <Play className="h-3.5 w-3.5 fill-current" />
            <span>{isGuiding ? "Analyzing..." : "Next Step"}</span>
          </button>
        </div>
      </div>

      {guideError && (
        <div className="bg-red-950/40 border border-red-500/40 text-red-200 p-3 rounded-sm text-xs font-mono">
          <strong>Error:</strong> {guideError}
        </div>
      )}

      {/* Main Content Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-5">
        {/* Left Column: Progress Timeline & Architecture Summary */}
        <div className="space-y-5 lg:col-span-1">
          {/* Progress Timeline */}
          <div className="bg-surface border border-border rounded-sm p-4 space-y-3">
            <h2 className="text-xs font-bold text-gray-300 uppercase tracking-wider flex items-center space-x-2 font-mono border-b border-border pb-2">
              <Layers className="h-3.5 w-3.5 text-cyan-400" />
              <span>Investigation Journey</span>
            </h2>

            {steps.length === 0 ? (
              <p className="text-xs text-gray-500 italic">No investigation steps initialized yet.</p>
            ) : (
              <div className="space-y-2">
                {steps.map((step, idx) => {
                  const isCurrent = currentStep?.id === step.id;
                  const isCompleted = step.status === "STEP_COMPLETED" || idx < (investigation?.current_step_index ?? 0);

                  return (
                    <div
                      key={step.id || idx}
                      onClick={() => selectGuideStep(idx)}
                      className={`flex items-start space-x-2.5 p-2.5 rounded-sm cursor-pointer transition border font-mono ${
                        isCurrent
                          ? "bg-cyan-950/40 border-cyan-500/60 text-cyan-200"
                          : isCompleted
                          ? "bg-background border-border text-gray-400 hover:border-gray-700"
                          : "bg-background/40 border-transparent text-gray-500 hover:border-border"
                      }`}
                    >
                      <div className="mt-0.5 shrink-0">
                        {isCompleted ? (
                          <CheckCircle2 className="h-4 w-4 text-emerald-400" />
                        ) : isCurrent ? (
                          <ArrowRight className="h-4 w-4 text-cyan-400 animate-pulse" />
                        ) : (
                          <Circle className="h-4 w-4 text-gray-600" />
                        )}
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between">
                          <span className="text-[9px] font-mono text-gray-500 uppercase font-bold">
                            Step {idx + 1}: {step.step_type}
                          </span>
                        </div>
                        <p className="text-xs font-bold truncate mt-0.5 text-gray-200">
                          {step.title}
                        </p>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* Architecture Summary Card */}
          {summary && (
            <div className="bg-surface border border-border rounded-sm p-4 space-y-3 font-mono">
              <h2 className="text-xs font-bold text-gray-300 uppercase tracking-wider flex items-center space-x-2 border-b border-border pb-2">
                <BookOpen className="h-3.5 w-3.5 text-cyan-400" />
                <span>Architecture Overview</span>
              </h2>

              <p className="text-xs text-gray-300 leading-relaxed bg-background p-2.5 rounded-sm border border-border">
                {summary.overview}
              </p>

              {/* Stats Grid */}
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="bg-background p-2 rounded-sm border border-border">
                  <span className="text-gray-500 block text-[9px] uppercase font-bold">Total Files</span>
                  <span className="font-mono font-bold text-gray-200">{summary.total_files}</span>
                </div>
                <div className="bg-background p-2 rounded-sm border border-border">
                  <span className="text-gray-500 block text-[9px] uppercase font-bold">Total Symbols</span>
                  <span className="font-mono font-bold text-gray-200">{summary.total_symbols}</span>
                </div>
              </div>

              {/* Major Modules */}
              {summary.major_modules && summary.major_modules.length > 0 && (
                <div className="space-y-1">
                  <span className="text-[10px] font-bold text-gray-400 uppercase block">
                    Major Modules
                  </span>
                  <div className="space-y-1">
                    {summary.major_modules.map((mod, i) => (
                      <div
                        key={i}
                        className="flex items-center justify-between text-xs bg-background px-2 py-1 rounded-sm border border-border"
                      >
                        <span className="font-mono text-cyan-300 truncate">{mod.name}</span>
                        <span className="text-gray-500 font-mono text-[9px]">
                          {mod.file_count} files / {mod.symbol_count} syms
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Entry Point Candidates */}
              {summary.entry_point_candidates && summary.entry_point_candidates.length > 0 && (
                <div className="space-y-1">
                  <span className="text-[10px] font-bold text-gray-400 uppercase block">
                    Entry Point Candidates
                  </span>
                  <div className="space-y-1">
                    {summary.entry_point_candidates.map((ep, i) => (
                      <div
                        key={i}
                        onClick={() => selectFileAndHighlight(ep, 1)}
                        className="flex items-center space-x-1.5 text-xs text-cyan-400 hover:text-cyan-300 cursor-pointer font-mono truncate"
                      >
                        <FileCode className="h-3 w-3 shrink-0" />
                        <span className="truncate">{ep}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Right Column: Current Finding, Evidence & Suggested Questions */}
        <div className="space-y-5 lg:col-span-2 font-mono">
          {/* Current Step Finding & Evidence */}
          <div className="bg-surface border border-border rounded-sm p-4 space-y-4">
            <div className="flex items-center justify-between border-b border-border pb-3">
              <div>
                <span className="text-[10px] font-mono text-cyan-400 uppercase font-bold tracking-wider">
                  CURRENT DETERMINISTIC FINDING
                </span>
                <h2 className="text-sm font-bold text-gray-100 mt-0.5">
                  {currentStep ? currentStep.title : "No Active Step"}
                </h2>
              </div>

              {currentStep?.target_file && (
                <button
                  onClick={() => {
                    selectFileAndHighlight(currentStep.target_file!, 1);
                    setActiveTab("EXPLORER");
                  }}
                  className="flex items-center space-x-1.5 px-2.5 py-1 bg-background hover:bg-surface-hover text-xs font-mono text-cyan-300 rounded-sm border border-border transition font-bold"
                >
                  <FileCode className="h-3.5 w-3.5 text-cyan-400" />
                  <span>Inspect Code</span>
                </button>
              )}
            </div>

            {currentStep ? (
              <div className="space-y-4">
                <div className="text-xs text-gray-300 leading-relaxed font-sans space-y-2">
                  {currentStep.description.split("\n\n").map((paragraph, pIdx) => (
                    <p key={pIdx}>
                      {paragraph.split(/(`[^`]+`)/g).map((part, idx) => {
                        if (part.startsWith("`") && part.endsWith("`")) {
                          return (
                            <code
                              key={idx}
                              className="bg-cyan-950/60 text-cyan-300 border border-cyan-500/30 px-1.5 py-0.5 mx-0.5 rounded text-[11px] font-mono font-bold inline-block"
                            >
                              {part.slice(1, -1)}
                            </code>
                          );
                        }
                        return <span key={idx}>{part}</span>;
                      })}
                    </p>
                  ))}
                </div>

                {/* Evidence Package Reference */}
                {currentStep.evidence && currentStep.evidence.items && currentStep.evidence.items.length > 0 && (
                  <div className="bg-background rounded-sm p-3 border border-border space-y-2">
                    <span className="text-[10px] font-bold text-gray-400 uppercase tracking-wider block">
                      DETERMINISTIC EVIDENCE PACKAGE ({currentStep.evidence.items.length} ITEMS)
                    </span>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
                      {currentStep.evidence.items.map((item, idx) => (
                        <div
                          key={item.id || idx}
                          className="bg-surface p-2.5 rounded-sm border border-border text-xs space-y-1"
                        >
                          <div className="flex items-center justify-between">
                            <span className="font-mono text-cyan-300 font-bold text-[11px]">
                              [{item.label}]
                            </span>
                            <span className="text-[9px] bg-background text-gray-400 px-1.5 py-0.2 rounded-sm border border-border font-mono">
                              {item.kind}
                            </span>
                          </div>
                          <p className="text-gray-300 text-[11px] line-clamp-2">{item.summary}</p>
                          <span className="text-[9px] text-gray-500 font-mono block truncate">
                            {item.provenance}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Navigation & Action Buttons */}
                <div className="pt-2 flex flex-wrap items-center gap-2.5">
                  {currentStep.target_file && (
                    <button
                      onClick={() => {
                        selectFileAndHighlight(currentStep.target_file!, 1);
                        setActiveTab("EXPLORER");
                      }}
                      className="flex items-center space-x-1.5 px-3 py-1.5 bg-surface hover:bg-surface-hover text-cyan-300 text-xs font-bold rounded-sm border border-border transition"
                    >
                      <FileCode className="h-3.5 w-3.5 text-cyan-400" />
                      <span>Inspect Code</span>
                    </button>
                  )}

                  {currentStep.target_symbol && (
                    <button
                      onClick={() => {
                        selectGraphNode(currentStep.target_symbol!);
                        setActiveTab("GRAPH");
                      }}
                      className="flex items-center space-x-1.5 px-3 py-1.5 bg-surface hover:bg-surface-hover text-gray-200 text-xs font-bold rounded-sm border border-border transition"
                    >
                      <Layers className="h-3.5 w-3.5 text-cyan-400" />
                      <span>Focus in Graph</span>
                    </button>
                  )}

                  {currentStep.target_symbol && (
                    <button
                      onClick={() => traceFlowFromSymbol(currentStep.target_symbol!)}
                      className="flex items-center space-x-1.5 px-3 py-1.5 bg-cyan-950/70 hover:bg-cyan-900/90 text-cyan-300 text-xs font-bold rounded-sm border border-cyan-500/50 transition"
                    >
                      <Activity className="h-3.5 w-3.5 text-cyan-400" />
                      <span>Trace Execution & Call Flow</span>
                    </button>
                  )}

                  {currentStep.step_type === "STEP_MODULE_STRUCTURE" && (
                    <button
                      onClick={() => setActiveTab("GRAPH")}
                      className="flex items-center space-x-1.5 px-3 py-1.5 bg-surface hover:bg-surface-hover text-amber-300 text-xs font-bold rounded-sm border border-border transition"
                    >
                      <BookOpen className="h-3.5 w-3.5 text-amber-400" />
                      <span>View Architecture Diagram</span>
                    </button>
                  )}
                </div>
              </div>
            ) : (
              <p className="text-xs text-gray-500 italic py-4">
                Click &quot;Next Step&quot; or select a step from the journey to view findings.
              </p>
            )}
          </div>

          {/* Contextual Suggested Questions */}
          <div className="bg-surface border border-border rounded-sm p-4 space-y-3">
            <h2 className="text-xs font-bold text-gray-300 uppercase tracking-wider flex items-center space-x-2 border-b border-border pb-2">
              <HelpCircle className="h-3.5 w-3.5 text-cyan-400" />
              <span>Suggested Next Questions</span>
            </h2>

            {!investigation?.suggested_questions || investigation.suggested_questions.length === 0 ? (
              <p className="text-xs text-gray-500 italic">No suggested questions generated for this step.</p>
            ) : (
              <div className="grid grid-cols-1 gap-2">
                {investigation.suggested_questions.map((q, idx) => (
                  <button
                    key={idx}
                    onClick={() => handleQuestionClick(q)}
                    className="flex items-center justify-between text-left p-2.5 bg-background hover:bg-surface-hover text-xs text-gray-200 hover:text-cyan-300 rounded-sm border border-border hover:border-cyan-500/50 transition group font-mono"
                  >
                    <div className="flex items-center space-x-2">
                      <Sparkles className="h-3.5 w-3.5 text-cyan-400 shrink-0 group-hover:scale-110 transition-transform" />
                      <span>{q}</span>
                    </div>
                    <ChevronRight className="h-3.5 w-3.5 text-gray-500 group-hover:text-cyan-400 shrink-0" />
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

