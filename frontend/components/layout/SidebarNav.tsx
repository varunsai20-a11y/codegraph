"use client";

import React from "react";
import { useAppStore, NavigationTab } from "@/store/useAppStore";
import { FolderTree, Network, SplitSquareVertical, GitFork, Bot, Compass } from "lucide-react";

interface NavItem {
  id: NavigationTab;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  checkpoint: string;
}

const navItems: NavItem[] = [
  { id: "explorer", label: "Explorer", icon: FolderTree, checkpoint: "C2" },
  { id: "graph", label: "Graph", icon: Network, checkpoint: "C3" },
  { id: "sync", label: "Code+Graph", icon: SplitSquareVertical, checkpoint: "C4" },
  { id: "flow", label: "Call Flow", icon: GitFork, checkpoint: "C5" },
  { id: "ai", label: "AI Workspace", icon: Bot, checkpoint: "C6" },
  { id: "guide", label: "Guided Tour", icon: Compass, checkpoint: "C7" },
];

export const SidebarNav: React.FC = () => {
  const { activeTab, setActiveTab } = useAppStore();

  return (
    <aside className="w-48 border-r border-border bg-surface flex flex-col justify-between select-none">
      <div className="p-2 space-y-1">
        <div className="px-2 py-1.5 text-[10px] font-semibold text-gray-400 uppercase tracking-wider">
          Views
        </div>
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = activeTab === item.id;
          return (
            <button
              key={item.id}
              onClick={() => setActiveTab(item.id)}
              className={`w-full flex items-center justify-between px-2.5 py-2 rounded-md text-xs font-medium transition ${
                isActive
                  ? "bg-accent/15 text-accent border border-accent/30"
                  : "text-gray-300 hover:bg-surface-hover hover:text-white"
              }`}
            >
              <div className="flex items-center space-x-2">
                <Icon className="w-4 h-4" />
                <span>{item.label}</span>
              </div>
              <span className="text-[9px] px-1 rounded bg-background text-gray-400 border border-border">
                {item.checkpoint}
              </span>
            </button>
          );
        })}
      </div>

      <div className="p-3 border-t border-border text-[11px] text-gray-400">
        <p className="font-semibold text-gray-300">Phase 5 Status</p>
        <p className="mt-0.5 text-accent font-mono">C1 Foundation Active</p>
      </div>
    </aside>
  );
};
