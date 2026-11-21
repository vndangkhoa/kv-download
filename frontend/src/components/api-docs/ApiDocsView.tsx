'use client';

import React from 'react';
import { BookOpen, Terminal, Code, Cpu } from 'lucide-react';

export function ApiDocsView() {
  return (
    <div className="space-y-6 max-w-4xl mx-auto pb-12 select-none">
      {/* Intro Card */}
      <div className="glass-panel p-6 rounded-3xl space-y-3 shadow-xl">
        <div className="flex items-center gap-3">
          <div className="w-10 h-10 rounded-2xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
            <BookOpen className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-base font-bold text-[var(--text-color)]">REST API & Aria2 Integration</h3>
            <p className="text-[12px] text-[var(--text-muted)]">
              Interact with KV Download programmatically or connect browser extensions via Aria2 JSON-RPC.
            </p>
          </div>
        </div>
      </div>

      {/* REST API Endpoints */}
      <div className="glass-panel p-6 rounded-3xl space-y-4 shadow-xl">
        <h4 className="text-sm font-bold text-[var(--text-color)] flex items-center gap-2">
          <Terminal className="w-4 h-4 text-primary" /> Core REST Endpoints
        </h4>

        <div className="space-y-3 font-mono text-[12px]">
          <div className="p-3 rounded-2xl bg-[var(--card-bg)] border border-[var(--glass-border)] space-y-1">
            <div className="flex items-center gap-2">
              <span className="px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-400 font-bold text-[10px]">POST</span>
              <span className="text-[var(--text-color)]">/api/queue/add</span>
            </div>
            <p className="text-[11px] text-[var(--text-muted)] font-sans">Enqueue one or multiple URLs for background downloading.</p>
          </div>

          <div className="p-3 rounded-2xl bg-[var(--card-bg)] border border-[var(--glass-border)] space-y-1">
            <div className="flex items-center gap-2">
              <span className="px-2 py-0.5 rounded bg-sky-500/20 text-sky-400 font-bold text-[10px]">GET</span>
              <span className="text-[var(--text-color)]">/api/queue</span>
            </div>
            <p className="text-[11px] text-[var(--text-muted)] font-sans">Retrieve active, pending, and completed download task queue items.</p>
          </div>

          <div className="p-3 rounded-2xl bg-[var(--card-bg)] border border-[var(--glass-border)] space-y-1">
            <div className="flex items-center gap-2">
              <span className="px-2 py-0.5 rounded bg-sky-500/20 text-sky-400 font-bold text-[10px]">GET</span>
              <span className="text-[var(--text-color)]">/api/events</span>
            </div>
            <p className="text-[11px] text-[var(--text-muted)] font-sans">Server-Sent Events (SSE) stream for real-time download telemetry, speeds, and status.</p>
          </div>

          <div className="p-3 rounded-2xl bg-[var(--card-bg)] border border-[var(--glass-border)] space-y-1">
            <div className="flex items-center gap-2">
              <span className="px-2 py-0.5 rounded bg-sky-500/20 text-sky-400 font-bold text-[10px]">GET</span>
              <span className="text-[var(--text-color)]">/api/gallery</span>
            </div>
            <p className="text-[11px] text-[var(--text-muted)] font-sans">List all organized media items on disk grouped by category folders.</p>
          </div>
        </div>
      </div>

      {/* Aria2 JSON-RPC Card */}
      <div className="glass-panel p-6 rounded-3xl space-y-4 shadow-xl">
        <h4 className="text-sm font-bold text-[var(--text-color)] flex items-center gap-2">
          <Cpu className="w-4 h-4 text-cyan-400" /> Aria2 JSON-RPC Compatibility
        </h4>
        <p className="text-[12px] text-[var(--text-muted)]">
          KV Download implements standard Aria2 JSON-RPC emulation at <code className="text-primary font-mono font-bold">/jsonrpc</code> or <code className="text-primary font-mono font-bold">/rpc</code>. Point any Aria2 browser extension (e.g. Aria2 Explorer, Aria2 Download Manager) to port 9292.
        </p>
      </div>
    </div>
  );
}
