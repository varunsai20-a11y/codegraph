"use client";

import React from "react";
import { useAppStore } from "@/store/useAppStore";
import { ShieldCheck, HardDrive, Terminal } from "lucide-react";

export const StatusBar: React.FC = () => {
  const { activeRepo, selectedPath, error } = useAppStore();

  return (
    <footer className="h-6 border-t border-border bg-surface px-3 flex items-center justify-between text-[10px] font-mono text-gray-400 select-none shrink-0">
      <div className="flex items-center space-x-4 min-w-0">
        <div className="flex items-center space-x-1.5 truncate">
          <HardDrive className="w-3 h-3 text-cyan-400 shrink-0" />
          <span>
            TARGET:{" "}
            <strong className="text-gray-200">
              {activeRepo ? activeRepo.name : "NONE"}
            </strong>
          </span>
        </div>

        {selectedPath && (
          <div className="flex items-center space-x-1.5 border-l border-border pl-3 truncate">
            <span className="text-gray-500">FILE:</span>
            <code className="text-cyan-300 font-mono truncate">{selectedPath}</code>
          </div>
        )}
      </div>

      <div className="flex items-center space-x-3 shrink-0">
        {error ? (
          <span className="text-red-400 truncate max-w-xs">{error}</span>
        ) : (
          <div className="flex items-center space-x-1 text-emerald-400">
            <ShieldCheck className="w-3 h-3" />
            <span>STRICT GROUNDING ACTIVE</span>
          </div>
        )}
      </div>
    </footer>
  );
};
