"use client";

import React from "react";
import { useAppStore, NavigationTab } from "@/store/useAppStore";
import {
  FolderTree,
  Network,
  SplitSquareVertical,
  GitFork,
  Bot,
  Compass,
  Cpu,
  Layers,
  Terminal,
} from "lucide-react";

interface NavItem {
  id: NavigationTab;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  badge?: string;
  badgeStyle?: string;
}

const navItems: NavItem[] = [
  { id: "EXPLORER", label: "Repository", icon: FolderTree },
  { id: "GRAPH", label: "Graph", icon: Network },
  { id: "SYNC", label: "Source", icon: SplitSquareVertical },
  { id: "FLOW", label: "Call Flow", icon: GitFork },
  { id: "AI", label: "AI Explain", icon: Bot, badge: "AI", badgeStyle: "bg-violet-950/60 text-violet-300 border-violet-500/40" },
  { id: "GUIDE", label: "Guided Tour", icon: Compass, badge: "C7", badgeStyle: "bg-cyan-950/60 text-cyan-300 border-cyan-500/40" },
];

export const SidebarNav: React.FC = () => {
  const { activeTab, setActiveTab, activeRepo } = useAppStore();

  return (
    <aside className="w-44 border-r border-border bg-surface flex flex-col justify-between select-none shrink-0 font-mono">
      <div className="p-2 space-y-1">
        <div className="px-2 py-1.5 text-[9px] font-mono font-bold text-gray-500 uppercase tracking-widest flex items-center justify-between">
          <span>NAVIGATION</span>
          <span className="w-1.5 h-1.5 rounded-full bg-cyan-400" />
        </div>
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = activeTab === item.id;
          return (
            <button
              key={item.id}
              onClick={() => setActiveTab(item.id)}
              className={`w-full flex items-center justify-between px-2.5 py-1.5 rounded-sm text-xs font-mono transition ${
                isActive
                  ? "bg-cyan-950/50 text-cyan-300 border border-cyan-500/50 font-bold shadow-sm"
                  : "text-gray-400 hover:bg-surface-hover hover:text-gray-200 border border-transparent"
              }`}
            >
              <div className="flex items-center space-x-2 truncate">
                <Icon className={`w-3.5 h-3.5 shrink-0 ${isActive ? "text-cyan-400" : "text-gray-500"}`} />
                <span className="truncate">{item.label}</span>
              </div>
              {item.badge && (
                <span className={`text-[8px] font-mono px-1 py-0.2 rounded border ${item.badgeStyle}`}>
                  {item.badge}
                </span>
              )}
            </button>
          );
        })}
      </div>

      {/* System Indicator Footer */}
      <div className="p-2.5 border-t border-border bg-background/60 text-[10px] font-mono text-gray-400 space-y-1">
        <div className="flex items-center justify-between text-gray-400">
          <span className="flex items-center gap-1.5">
            <Cpu className="w-3 h-3 text-cyan-400" />
            <span className="tracking-wider">GRAPH ENGINE</span>
          </span>
          <span className="text-emerald-400 font-bold text-[9px] px-1 bg-emerald-950/40 border border-emerald-500/30 rounded-sm">
            READY
          </span>
        </div>
        {activeRepo && (
          <p className="text-[9px] text-gray-500 truncate pt-0.5" title={activeRepo.name}>
            Repo: <strong className="text-gray-300">{activeRepo.name}</strong>
          </p>
        )}
      </div>
    </aside>
  );
};
