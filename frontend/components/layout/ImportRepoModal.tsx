"use client";

import React, { useState, useEffect, useRef } from "react";
import { CodeGraphAPIClient } from "../../lib/api-client";
import { Repository, IndexJob } from "../../lib/types";
import { useAppStore } from "../../store/useAppStore";
import { Github, Loader2, CheckCircle2, AlertCircle, X, ArrowRight } from "lucide-react";

interface ImportRepoModalProps {
  isOpen: boolean;
  onClose: () => void;
  onImportSuccess?: (repo: Repository) => void;
}

export const ImportRepoModal: React.FC<ImportRepoModalProps> = ({
  isOpen,
  onClose,
  onImportSuccess,
}) => {
  const [url, setUrl] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [activeJob, setActiveJob] = useState<IndexJob | null>(null);
  const [activeRepo, setActiveRepo] = useState<Repository | null>(null);
  const [statusMessage, setStatusMessage] = useState<string>("");

  const isFetchingRef = useRef(false);
  const isUnmountedRef = useRef(false);
  const retryCountRef = useRef(0);
  const timerRef = useRef<NodeJS.Timeout | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  const { fetchRepositories, selectRepository } = useAppStore();
  const apiClient = useRef(new CodeGraphAPIClient()).current;

  useEffect(() => {
    isUnmountedRef.current = false;
    return () => {
      isUnmountedRef.current = true;
      if (timerRef.current) clearTimeout(timerRef.current);
      if (abortControllerRef.current) abortControllerRef.current.abort();
    };
  }, []);

  if (!isOpen) return null;

  const validateInput = (inputUrl: string): string | null => {
    const trimmed = inputUrl.trim();
    if (!trimmed) return "Please enter a GitHub repository URL.";
    if (trimmed.includes("?") || trimmed.includes("#")) {
      return "URL query parameters and fragments are not supported.";
    }
    if (!trimmed.startsWith("https://github.com/")) {
      return "URL must begin with https://github.com/";
    }
    return null;
  };

  const startPolling = (jobId: string, repo: Repository) => {
    if (timerRef.current) clearTimeout(timerRef.current);
    retryCountRef.current = 0;

    const pollTick = async () => {
      if (isUnmountedRef.current) return;
      if (isFetchingRef.current) return;

      isFetchingRef.current = true;
      abortControllerRef.current = new AbortController();

      try {
        const job = await apiClient.getIndexJob(jobId, abortControllerRef.current.signal);
        if (isUnmountedRef.current) return;

        setActiveJob(job);
        retryCountRef.current = 0;

        if (job.status === "PENDING") {
          setStatusMessage("Queued for indexing...");
          timerRef.current = setTimeout(pollTick, 1000);
        } else if (job.status === "RUNNING") {
          const processed = job.files_indexed + job.files_skipped + job.files_failed;
          if (job.files_discovered > 0) {
            setStatusMessage(`Indexing files: ${processed} / ${job.files_discovered} processed (${job.files_indexed} indexed, ${job.files_skipped} skipped, ${job.files_failed} failed)`);
          } else {
            setStatusMessage("Indexing repository files...");
          }
          timerRef.current = setTimeout(pollTick, 1000);
        } else if (job.status === "COMPLETED") {
          setStatusMessage("Indexing completed successfully!");
          setIsSubmitting(false);
          await fetchRepositories();
          await selectRepository(repo.id);
          if (onImportSuccess) onImportSuccess(repo);
        } else if (job.status === "FAILED") {
          setError(job.error || "Repository indexing failed.");
          setIsSubmitting(false);
        }
      } catch (err: any) {
        if (isUnmountedRef.current || err.message === "Request cancelled") return;

        if (retryCountRef.current < 3) {
          retryCountRef.current += 1;
          const backoff = Math.pow(2, retryCountRef.current - 1) * 1000;
          setStatusMessage(`Connection lost. Retrying status check (${retryCountRef.current}/3)...`);
          timerRef.current = setTimeout(pollTick, backoff);
        } else {
          setError("Lost connection to backend server status endpoint.");
          setIsSubmitting(false);
        }
      } finally {
        isFetchingRef.current = false;
      }
    };

    pollTick();
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);

    const validationErr = validateInput(url);
    if (validationErr) {
      setError(validationErr);
      return;
    }

    setIsSubmitting(true);
    setStatusMessage("Registering repository with CodeGraph...");

    try {
      // 1. Register Repository
      const repo = await apiClient.registerRepository({
        source_type: "GIT",
        source_url: url.trim(),
      });
      setActiveRepo(repo);

      // 2. Start Index Job
      setStatusMessage("Starting background indexing job...");
      const job = await apiClient.startIndexJob(repo.id);
      setActiveJob(job);

      // 3. Begin status polling
      startPolling(job.id, repo);
    } catch (err: any) {
      setError(err.message || "Failed to import GitHub repository.");
      setIsSubmitting(false);
    }
  };

  const handleClose = () => {
    if (timerRef.current) clearTimeout(timerRef.current);
    if (abortControllerRef.current) abortControllerRef.current.abort();
    onClose();
  };

  const processedCount = activeJob
    ? activeJob.files_indexed + activeJob.files_skipped + activeJob.files_failed
    : 0;
  const progressPercent =
    activeJob && activeJob.files_discovered > 0
      ? Math.min(100, Math.round((processedCount / activeJob.files_discovered) * 100))
      : 0;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
      <div className="relative w-full max-w-lg rounded-xl border border-slate-800 bg-slate-900 p-6 shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 pb-4 mb-4">
          <div className="flex items-center gap-3">
            <div className="rounded-lg bg-blue-500/10 p-2 text-blue-400">
              <Github className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-lg font-semibold text-slate-100">Import GitHub Repository</h3>
              <p className="text-xs text-slate-400">Analyze any public GitHub repository directly</p>
            </div>
          </div>
          <button
            onClick={handleClose}
            className="rounded-lg p-1 text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Body */}
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-xs font-medium text-slate-300 mb-1.5">
              GitHub Repository URL
            </label>
            <input
              type="text"
              value={url}
              onChange={(e) => {
                setUrl(e.target.value);
                if (error) setError(null);
              }}
              disabled={isSubmitting}
              placeholder="https://github.com/gin-gonic/gin"
              className="w-full rounded-lg border border-slate-700 bg-slate-950 px-3.5 py-2.5 text-sm text-slate-100 placeholder-slate-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          {/* Validation & API Errors */}
          {error && (
            <div className="flex items-start gap-2.5 rounded-lg bg-red-500/10 border border-red-500/20 p-3 text-xs text-red-400">
              <AlertCircle className="h-4 w-4 shrink-0 mt-0.5" />
              <span>{error}</span>
            </div>
          )}

          {/* Active Job Progress Display */}
          {activeJob && (
            <div className="space-y-3 rounded-lg border border-slate-800 bg-slate-950/50 p-4">
              <div className="flex items-center justify-between text-xs text-slate-300">
                <span className="font-medium">{statusMessage}</span>
                <span className="font-mono">{progressPercent}%</span>
              </div>

              {/* Progress Bar */}
              <div className="h-2 w-full overflow-hidden rounded-full bg-slate-800">
                <div
                  className="h-full bg-gradient-to-r from-blue-500 to-cyan-400 transition-all duration-300"
                  style={{ width: `${progressPercent}%` }}
                />
              </div>

              {/* Observable Metric Stats */}
              <div className="grid grid-cols-4 gap-2 pt-1 text-center text-[10px]">
                <div className="rounded bg-slate-900 p-1.5 border border-slate-800">
                  <div className="text-slate-400">Discovered</div>
                  <div className="font-semibold text-slate-200">{activeJob.files_discovered}</div>
                </div>
                <div className="rounded bg-slate-900 p-1.5 border border-slate-800">
                  <div className="text-slate-400">Indexed</div>
                  <div className="font-semibold text-emerald-400">{activeJob.files_indexed}</div>
                </div>
                <div className="rounded bg-slate-900 p-1.5 border border-slate-800">
                  <div className="text-slate-400">Skipped</div>
                  <div className="font-semibold text-amber-400">{activeJob.files_skipped}</div>
                </div>
                <div className="rounded bg-slate-900 p-1.5 border border-slate-800">
                  <div className="text-slate-400">Failed</div>
                  <div className="font-semibold text-red-400">{activeJob.files_failed}</div>
                </div>
              </div>
            </div>
          )}

          {/* Success State */}
          {activeJob?.status === "COMPLETED" && (
            <div className="flex items-center gap-2 rounded-lg bg-emerald-500/10 border border-emerald-500/20 p-3 text-xs text-emerald-400">
              <CheckCircle2 className="h-4 w-4 shrink-0" />
              <span>Repository imported and ready for exploration!</span>
            </div>
          )}

          {/* Footer Actions */}
          <div className="flex items-center justify-end gap-3 pt-2 border-t border-slate-800">
            <button
              type="button"
              onClick={handleClose}
              className="rounded-lg px-4 py-2 text-xs font-medium text-slate-400 hover:bg-slate-800 hover:text-slate-200 transition-colors"
            >
              {activeJob?.status === "COMPLETED" ? "Close" : "Cancel"}
            </button>

            {activeJob?.status === "COMPLETED" ? (
              <button
                type="button"
                onClick={handleClose}
                className="flex items-center gap-1.5 rounded-lg bg-blue-600 px-4 py-2 text-xs font-medium text-white hover:bg-blue-500 transition-colors"
              >
                <span>Explore Graph</span>
                <ArrowRight className="h-3.5 w-3.5" />
              </button>
            ) : (
              <button
                type="submit"
                disabled={isSubmitting}
                className="flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2 text-xs font-medium text-white hover:bg-blue-500 disabled:opacity-50 transition-colors"
              >
                {isSubmitting ? (
                  <>
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    <span>Importing...</span>
                  </>
                ) : (
                  <>
                    <Github className="h-3.5 w-3.5" />
                    <span>Import Repository</span>
                  </>
                )}
              </button>
            )}
          </div>
        </form>
      </div>
    </div>
  );
};
