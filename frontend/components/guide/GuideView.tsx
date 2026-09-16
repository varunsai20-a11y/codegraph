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

  useEffect(() => {
    if (activeRepoID && !investigation && !isGuiding) {
      fetchGuide("INITIALIZE");
    }
  }, [activeRepoID, investigation, isGuiding, fetchGuide]);

  if (!activeRepoID) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-zinc-400">
        <p>Please select a repository to start guided reverse engineering.</p>
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
    <div className="flex flex-col h-full bg-zinc-950 text-zinc-100 overflow-y-auto p-6 space-y-6">
      {/* Header Banner */}
      <div className="flex items-center justify-between border-b border-zinc-800 pb-4">
        <div>
          <div className="flex items-center space-x-2">
            <Compass className="h-6 w-6 text-sky-400" />
            <h1 className="text-xl font-semibold text-zinc-100">
              Guided Reverse Engineering
            </h1>
            <span className="text-xs bg-sky-950 text-sky-300 border border-sky-800 px-2 py-0.5 rounded-full font-mono">
              C7 Grounded Guide
            </span>
          </div>
          <p className="text-sm text-zinc-400 mt-1">
            Deterministic architectural discovery & step-by-step code investigation for{" "}
            <span className="font-mono text-zinc-200">{activeRepo?.name || activeRepoID}</span>
          </p>
        </div>

        <div className="flex items-center space-x-3">
          <button
            onClick={handleStartOrReset}
            disabled={isGuiding}
            className="flex items-center space-x-2 px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 text-sm font-medium rounded border border-zinc-700 disabled:opacity-50 transition"
          >
            <RotateCcw className="h-4 w-4" />
            <span>Reset Guide</span>
          </button>
          <button
            onClick={handleNextStep}
            disabled={isGuiding || investigation?.status === "COMPLETED"}
            className="flex items-center space-x-2 px-4 py-1.5 bg-sky-600 hover:bg-sky-500 text-white text-sm font-medium rounded shadow disabled:opacity-50 transition"
          >
            <Play className="h-4 w-4 fill-current" />
            <span>{isGuiding ? "Analyzing..." : "Next Step"}</span>
          </button>
        </div>
      </div>

      {guideError && (
        <div className="bg-red-950/60 border border-red-800 text-red-200 p-4 rounded-md text-sm">
          <strong>Error:</strong> {guideError}
        </div>
      )}

      {/* Main Content Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column: Progress Timeline & Architecture Summary */}
        <div className="space-y-6 lg:col-span-1">
          {/* Progress Timeline */}
          <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4">
            <h2 className="text-sm font-semibold text-zinc-300 uppercase tracking-wider mb-4 flex items-center space-x-2">
              <Layers className="h-4 w-4 text-sky-400" />
              <span>Investigation Journey</span>
            </h2>

            {steps.length === 0 ? (
              <p className="text-sm text-zinc-500 italic">No investigation steps initialized yet.</p>
            ) : (
              <div className="space-y-3">
                {steps.map((step, idx) => {
                  const isCurrent = currentStep?.id === step.id;
                  const isCompleted = step.status === "STEP_COMPLETED" || idx < (investigation?.current_step_index ?? 0);

                  return (
                    <div
                      key={step.id || idx}
                      onClick={() => selectGuideStep(idx)}
                      className={`flex items-start space-x-3 p-2.5 rounded-md cursor-pointer transition border ${
                        isCurrent
                          ? "bg-sky-950/40 border-sky-700/60 text-sky-200"
                          : isCompleted
                          ? "bg-zinc-900/50 border-zinc-800 text-zinc-400 hover:border-zinc-700"
                          : "bg-zinc-950/30 border-transparent text-zinc-500 hover:border-zinc-800"
                      }`}
                    >
                      <div className="mt-0.5 flex-shrink-0">
                        {isCompleted ? (
                          <CheckCircle2 className="h-4 w-4 text-emerald-400" />
                        ) : isCurrent ? (
                          <ArrowRight className="h-4 w-4 text-sky-400 animate-pulse" />
                        ) : (
                          <Circle className="h-4 w-4 text-zinc-600" />
                        )}
                      </div>
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between">
                          <span className="text-xs font-mono text-zinc-400 uppercase">
                            Step {idx + 1}: {step.step_type}
                          </span>
                        </div>
                        <p className="text-sm font-medium truncate mt-0.5">
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
            <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-4 space-y-4">
              <h2 className="text-sm font-semibold text-zinc-300 uppercase tracking-wider flex items-center space-x-2">
                <BookOpen className="h-4 w-4 text-sky-400" />
                <span>Architecture Overview</span>
              </h2>

              <p className="text-xs text-zinc-300 leading-relaxed bg-zinc-950 p-2.5 rounded border border-zinc-800/80">
                {summary.overview}
              </p>

              {/* Stats Grid */}
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="bg-zinc-950 p-2 rounded border border-zinc-800">
                  <span className="text-zinc-500 block">Total Files</span>
                  <span className="font-mono font-semibold text-zinc-200">{summary.total_files}</span>
                </div>
                <div className="bg-zinc-950 p-2 rounded border border-zinc-800">
                  <span className="text-zinc-500 block">Total Symbols</span>
                  <span className="font-mono font-semibold text-zinc-200">{summary.total_symbols}</span>
                </div>
              </div>

              {/* Major Modules */}
              {summary.major_modules && summary.major_modules.length > 0 && (
                <div>
                  <span className="text-xs font-semibold text-zinc-400 block mb-1">
                    Major Modules
                  </span>
                  <div className="space-y-1">
                    {summary.major_modules.map((mod, i) => (
                      <div
                        key={i}
                        className="flex items-center justify-between text-xs bg-zinc-950/60 px-2 py-1 rounded border border-zinc-800/50"
                      >
                        <span className="font-mono text-zinc-300 truncate">{mod.name}</span>
                        <span className="text-zinc-500 font-mono text-[10px]">
                          {mod.file_count} files / {mod.symbol_count} syms
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* Entry Point Candidates */}
              {summary.entry_point_candidates && summary.entry_point_candidates.length > 0 && (
                <div>
                  <span className="text-xs font-semibold text-zinc-400 block mb-1">
                    Entry Point Candidates
                  </span>
                  <div className="space-y-1">
                    {summary.entry_point_candidates.map((ep, i) => (
                      <div
                        key={i}
                        onClick={() => selectFileAndHighlight(ep, 1)}
                        className="flex items-center space-x-1.5 text-xs text-sky-400 hover:text-sky-300 cursor-pointer font-mono truncate"
                      >
                        <FileCode className="h-3 w-3 flex-shrink-0" />
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
        <div className="space-y-6 lg:col-span-2">
          {/* Current Step Finding & Evidence */}
          <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-5 space-y-4">
            <div className="flex items-center justify-between border-b border-zinc-800 pb-3">
              <div>
                <span className="text-xs font-mono text-sky-400 uppercase tracking-wide">
                  Current Finding
                </span>
                <h2 className="text-lg font-semibold text-zinc-100 mt-0.5">
                  {currentStep ? currentStep.title : "No Active Step"}
                </h2>
              </div>

              {currentStep?.target_file && (
                <button
                  onClick={() => {
                    selectFileAndHighlight(currentStep.target_file!, 1);
                    setActiveTab("EXPLORER");
                  }}
                  className="flex items-center space-x-1.5 px-3 py-1 bg-zinc-800 hover:bg-zinc-700 text-xs font-mono text-zinc-200 rounded border border-zinc-700 transition"
                >
                  <FileCode className="h-3.5 w-3.5 text-sky-400" />
                  <span>Inspect Code</span>
                </button>
              )}
            </div>

            {currentStep ? (
              <div className="space-y-4">
                <p className="text-sm text-zinc-300 leading-relaxed">
                  {currentStep.description}
                </p>

                {/* Evidence Package Reference */}
                {currentStep.evidence && currentStep.evidence.items && currentStep.evidence.items.length > 0 && (
                  <div className="bg-zinc-950 rounded-lg p-3 border border-zinc-800 space-y-2">
                    <span className="text-xs font-semibold text-zinc-400 uppercase tracking-wider block">
                      Deterministic Evidence ({currentStep.evidence.items.length} items)
                    </span>
                    <div className="grid grid-cols-1 md:grid-cols-2 gap-2">
                      {currentStep.evidence.items.map((item, idx) => (
                        <div
                          key={item.id || idx}
                          className="bg-zinc-900 p-2.5 rounded border border-zinc-800/80 text-xs space-y-1"
                        >
                          <div className="flex items-center justify-between">
                            <span className="font-mono text-sky-300 font-semibold">
                              [{item.label}]
                            </span>
                            <span className="text-[10px] bg-zinc-800 text-zinc-400 px-1.5 py-0.5 rounded font-mono">
                              {item.kind}
                            </span>
                          </div>
                          <p className="text-zinc-300 line-clamp-2">{item.summary}</p>
                          <span className="text-[10px] text-zinc-500 font-mono block truncate">
                            {item.provenance}
                          </span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Static Flow Trigger Button if step has target symbol */}
                {currentStep.target_symbol && (
                  <div className="pt-2 flex items-center space-x-3">
                    <button
                      onClick={() => traceFlowFromSymbol(currentStep.target_symbol!)}
                      className="flex items-center space-x-2 px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 text-sky-400 text-xs font-medium rounded border border-zinc-700 transition"
                    >
                      <Activity className="h-3.5 w-3.5" />
                      <span>Trace Static Call Flow</span>
                    </button>
                  </div>
                )}
              </div>
            ) : (
              <p className="text-sm text-zinc-500 italic py-4">
                Click &quot;Next Step&quot; or select a step from the journey to view deterministic findings.
              </p>
            )}
          </div>

          {/* Contextual Suggested Questions */}
          <div className="bg-zinc-900 border border-zinc-800 rounded-lg p-5 space-y-4">
            <h2 className="text-sm font-semibold text-zinc-300 uppercase tracking-wider flex items-center space-x-2">
              <HelpCircle className="h-4 w-4 text-sky-400" />
              <span>Suggested Next Questions</span>
            </h2>

            {!investigation?.suggested_questions || investigation.suggested_questions.length === 0 ? (
              <p className="text-xs text-zinc-500 italic">No suggested questions generated for this step.</p>
            ) : (
              <div className="grid grid-cols-1 gap-2">
                {investigation.suggested_questions.map((q, idx) => (
                  <button
                    key={idx}
                    onClick={() => handleQuestionClick(q)}
                    className="flex items-center justify-between text-left p-3 bg-zinc-950 hover:bg-zinc-900 text-sm text-zinc-200 hover:text-sky-300 rounded border border-zinc-800 hover:border-sky-800/80 transition group"
                  >
                    <div className="flex items-center space-x-2">
                      <Sparkles className="h-4 w-4 text-sky-400 flex-shrink-0 group-hover:scale-110 transition-transform" />
                      <span>{q}</span>
                    </div>
                    <ChevronRight className="h-4 w-4 text-zinc-600 group-hover:text-sky-400 flex-shrink-0" />
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
