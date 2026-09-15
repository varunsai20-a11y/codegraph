"use client";

import React, { useEffect } from "react";
import { Header } from "@/components/layout/Header";
import { SidebarNav } from "@/components/layout/SidebarNav";
import { StatusBar } from "@/components/layout/StatusBar";
import { useAppStore } from "@/store/useAppStore";
import { ShieldCheck, HardDrive, CheckCircle2, AlertTriangle, Layers } from "lucide-react";

export default function Home() {
  const {
    repositories,
    activeRepo,
    activeTab,
    fetchRepositories,
    isLoading,
    error,
  } = useAppStore();

  useEffect(() => {
    fetchRepositories();
  }, [fetchRepositories]);

  return (
    <div className="flex flex-col h-screen overflow-hidden">
      <Header />
      <div className="flex flex-1 overflow-hidden">
        <SidebarNav />

        {/* Main Content Workspace Area */}
        <main className="flex-1 bg-background p-6 overflow-y-auto space-y-6">
          {/* Top Info Banner */}
          <div className="border border-border bg-surface rounded-lg p-5 flex items-start justify-between">
            <div className="space-y-1">
              <div className="flex items-center space-x-2">
                <span className="h-2 w-2 rounded-full bg-accent animate-pulse" />
                <h2 className="text-base font-bold text-gray-100">
                  Phase 5 — C1 Frontend Architecture & API Contract
                </h2>
              </div>
              <p className="text-xs text-gray-400">
                Production-quality Next.js foundation, strongly typed API client, Zustand state manager, and secure Go backend integration.
              </p>
            </div>
            <div className="flex items-center space-x-2 bg-accent/10 border border-accent/30 text-accent px-3 py-1.5 rounded-md text-xs font-semibold">
              <ShieldCheck className="w-4 h-4" />
              <span>C1 Active</span>
            </div>
          </div>

          {/* Backend Connection Error Alert */}
          {error && (
            <div className="border border-red-500/40 bg-red-500/10 text-red-300 rounded-lg p-4 flex items-start space-x-3">
              <AlertTriangle className="w-5 h-5 text-red-400 shrink-0 mt-0.5" />
              <div>
                <h3 className="text-sm font-semibold text-red-200">Backend Connection Error</h3>
                <p className="text-xs text-red-300/90 mt-0.5">{error}</p>
                <p className="text-[11px] text-red-400/80 mt-1">
                  Ensure the Go backend server is running on <code>http://localhost:8080</code> (`go run cmd/codegraph/main.go`).
                </p>
              </div>
            </div>
          )}

          {/* Active Repository Card or Empty State */}
          <div className="border border-border bg-surface rounded-lg p-5">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-400 mb-4 flex items-center gap-2">
              <HardDrive className="w-4 h-4 text-accent" />
              Active Repository Status
            </h3>

            {isLoading ? (
              <div className="py-8 text-center text-xs text-gray-400">
                Fetching repository manifest from backend...
              </div>
            ) : activeRepo ? (
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <div className="bg-background border border-border p-3.5 rounded-md space-y-1">
                  <span className="text-[10px] text-gray-400 uppercase font-semibold">Name</span>
                  <p className="text-sm font-bold text-gray-100">{activeRepo.name}</p>
                </div>
                <div className="bg-background border border-border p-3.5 rounded-md space-y-1">
                  <span className="text-[10px] text-gray-400 uppercase font-semibold">Repository ID</span>
                  <p className="text-xs font-mono text-gray-300 truncate">{activeRepo.id}</p>
                </div>
                <div className="bg-background border border-border p-3.5 rounded-md space-y-1">
                  <span className="text-[10px] text-gray-400 uppercase font-semibold">Source Type & Status</span>
                  <div className="flex items-center space-x-2">
                    <span className="text-xs font-semibold text-gray-200">{activeRepo.source_type}</span>
                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 font-medium">
                      {activeRepo.status}
                    </span>
                  </div>
                </div>
              </div>
            ) : (
              <div className="py-8 text-center text-xs text-gray-400 border border-dashed border-border rounded-md">
                No registered repositories found in backend. Register a repository via `POST /api/repositories`.
              </div>
            )}
          </div>

          {/* Verified API Contract Grid */}
          <div className="border border-border bg-surface rounded-lg p-5">
            <h3 className="text-xs font-semibold uppercase tracking-wider text-gray-400 mb-4 flex items-center gap-2">
              <Layers className="w-4 h-4 text-accent" />
              Verified C1 Backend API Contracts
            </h3>

            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs border-collapse">
                <thead>
                  <tr className="border-b border-border text-gray-400 font-semibold">
                    <th className="py-2 px-3">Method</th>
                    <th className="py-2 px-3">Endpoint Path</th>
                    <th className="py-2 px-3">Purpose</th>
                    <th className="py-2 px-3">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border text-gray-300">
                  <tr>
                    <td className="py-2.5 px-3 font-mono text-emerald-400 font-bold">GET</td>
                    <td className="py-2.5 px-3 font-mono text-gray-200">/api/repositories</td>
                    <td className="py-2.5 px-3">List all registered repositories</td>
                    <td className="py-2.5 px-3">
                      <span className="inline-flex items-center gap-1 text-emerald-400 font-medium">
                        <CheckCircle2 className="w-3.5 h-3.5" /> Verified
                      </span>
                    </td>
                  </tr>
                  <tr>
                    <td className="py-2.5 px-3 font-mono text-emerald-400 font-bold">GET</td>
                    <td className="py-2.5 px-3 font-mono text-gray-200">/api/repositories/&#123;id&#125;</td>
                    <td className="py-2.5 px-3">Retrieve repository details</td>
                    <td className="py-2.5 px-3">
                      <span className="inline-flex items-center gap-1 text-emerald-400 font-medium">
                        <CheckCircle2 className="w-3.5 h-3.5" /> Verified
                      </span>
                    </td>
                  </tr>
                  <tr>
                    <td className="py-2.5 px-3 font-mono text-emerald-400 font-bold">GET</td>
                    <td className="py-2.5 px-3 font-mono text-gray-200">/api/repositories/&#123;id&#125;/file?path=...</td>
                    <td className="py-2.5 px-3">Secure, repository-scoped source code file reader</td>
                    <td className="py-2.5 px-3">
                      <span className="inline-flex items-center gap-1 text-emerald-400 font-medium">
                        <CheckCircle2 className="w-3.5 h-3.5" /> Secured (No Traversal / Secrets)
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </main>
      </div>
      <StatusBar />
    </div>
  );
}
