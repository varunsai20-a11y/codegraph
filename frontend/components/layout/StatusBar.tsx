"use client";

import React from "react";
import { useAppStore } from "@/store/useAppStore";
import { ShieldCheck, HardDrive } from "lucide-react";

export const StatusBar: React.FC = () => {
  const { activeRepo, selectedPath, error } = useAppStore();

  return (
    <footer className="h-6 border-t border-border bg-surface px-3 flex items-center justify-between text-[11px] text-gray-400 select-none">
      <div className="flex items-center space-x-4">
        <div className="flex items-center space-x-1.5">
          <HardDrive className="w-3 h-3 text-accent" />
          <span>
            Repo:{" "}
            <strong className="text-gray-200">
              {activeRepo ? activeRepo.name : "None selected"}
            </strong>
          </span>
        </div>

        {selectedPath && (
          <div className="flex items-center space-x-1.5 border-l border-border pl-3">
            <span>Path:</span>
            <code className="text-gray-200 font-mono">{selectedPath}</code>
          </div>
        )}
      </div>

      <div className="flex items-center space-x-3">
        {error ? (
          <span className="text-red-400 truncate max-w-xs">{error}</span>
        ) : (
          <div className="flex items-center space-x-1 text-emerald-400">
            <ShieldCheck className="w-3 h-3" />
            <span>Strict Backend Grounding Enforced</span>
          </div>
        )}
      </div>
    </footer>
  );
};
