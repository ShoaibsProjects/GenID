"use client"

import { useState, useEffect, useCallback } from "react"

interface LogEntry {
  id: string
  timestamp: string
  level: "debug" | "info" | "warn" | "error" | "fatal"
  service: string
  method?: string
  path?: string
  status?: number
  latency?: string
  message: string
  detail?: string
  source_ip?: string
  tags?: string[]
}

interface LogStats {
  total: number
  by_level: Record<string, number>
  capacity: number
  usage_pct: number
}

const LEVEL_COLORS: Record<string, string> = {
  debug: "text-gray-500",
  info: "text-blue-400",
  warn: "text-amber-400",
  error: "text-red-400",
  fatal: "text-red-600",
}

const LEVEL_BG: Record<string, string> = {
  debug: "bg-gray-900/50",
  info: "bg-blue-900/20",
  warn: "bg-amber-900/20",
  error: "bg-red-900/20",
  fatal: "bg-red-900/40",
}

const LEVEL_BADGE: Record<string, string> = {
  info: "badge-info",
  warn: "badge-warning",
  error: "badge-error",
  debug: "badge-neutral",
  fatal: "badge-error",
}

function StatusBadge({ status }: { status?: number }) {
  if (!status) return null
  let cls = "badge-neutral"
  if (status >= 500) cls = "badge-error"
  else if (status >= 400) cls = "badge-warning"
  else if (status >= 200 && status < 300) cls = "badge-success"
  return <span className={cls}>{status}</span>
}

function LevelBadge({ level }: { level: string }) {
  return <span className={LEVEL_BADGE[level] || "badge-neutral"}>{level}</span>
}

function timeAgo(ts: string): string {
  const diff = Date.now() - new Date(ts).getTime()
  const secs = Math.floor(diff / 1000)
  if (secs < 5) return "just now"
  if (secs < 60) return `${secs}s ago`
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

function DetailModal({ entry, onClose }: { entry: LogEntry; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={onClose}>
      <div className="w-full max-w-3xl max-h-[85vh] overflow-y-auto glass-card p-6 space-y-4" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-white">Log Detail</h2>
          <button className="text-gray-400 hover:text-white text-xl leading-none" onClick={onClose}>&times;</button>
        </div>

        <div className="grid grid-cols-2 gap-3 text-sm">
          <div className="p-2 rounded bg-surface-100/30">
            <span className="text-gray-500 block text-xs uppercase tracking-wider">ID</span>
            <span className="text-gray-200 font-mono">{entry.id}</span>
          </div>
          <div className="p-2 rounded bg-surface-100/30">
            <span className="text-gray-500 block text-xs uppercase tracking-wider">Timestamp</span>
            <span className="text-gray-200">{new Date(entry.timestamp).toLocaleString()}</span>
          </div>
          <div className="p-2 rounded bg-surface-100/30">
            <span className="text-gray-500 block text-xs uppercase tracking-wider">Level</span>
            <LevelBadge level={entry.level} />
          </div>
          <div className="p-2 rounded bg-surface-100/30">
            <span className="text-gray-500 block text-xs uppercase tracking-wider">Service</span>
            <span className="text-gray-200">{entry.service}</span>
          </div>
          {entry.method && (
            <div className="p-2 rounded bg-surface-100/30">
              <span className="text-gray-500 block text-xs uppercase tracking-wider">Method</span>
              <span className="text-brand-400 font-mono">{entry.method}</span>
            </div>
          )}
          {entry.path && (
            <div className="p-2 rounded bg-surface-100/30">
              <span className="text-gray-500 block text-xs uppercase tracking-wider">Path</span>
              <span className="text-gray-200 font-mono text-xs break-all">{entry.path}</span>
            </div>
          )}
          {entry.status && (
            <div className="p-2 rounded bg-surface-100/30">
              <span className="text-gray-500 block text-xs uppercase tracking-wider">Status</span>
              <StatusBadge status={entry.status} />
            </div>
          )}
          {entry.latency && (
            <div className="p-2 rounded bg-surface-100/30">
              <span className="text-gray-500 block text-xs uppercase tracking-wider">Latency</span>
              <span className="text-gray-200 font-mono">{entry.latency}</span>
            </div>
          )}
          {entry.source_ip && (
            <div className="p-2 rounded bg-surface-100/30">
              <span className="text-gray-500 block text-xs uppercase tracking-wider">Source IP</span>
              <span className="text-gray-200 font-mono">{entry.source_ip}</span>
            </div>
          )}
          {entry.tags && entry.tags.length > 0 && (
            <div className="p-2 rounded bg-surface-100/30 col-span-2">
              <span className="text-gray-500 block text-xs uppercase tracking-wider">Tags</span>
              <div className="flex gap-1.5 mt-1 flex-wrap">
                {entry.tags.map((t) => (
                  <span key={t} className="px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">{t}</span>
                ))}
              </div>
            </div>
          )}
        </div>

        <div className="p-3 rounded bg-surface-100/30">
          <span className="text-gray-500 block text-xs uppercase tracking-wider mb-1">Message</span>
          <p className="text-white text-sm whitespace-pre-wrap break-words">{entry.message}</p>
        </div>

        {entry.detail && (
          <div className="p-3 rounded bg-red-900/10 border border-red-900/30">
            <span className="text-red-400 block text-xs uppercase tracking-wider mb-1">Error Detail</span>
            <pre className="text-red-300 text-xs whitespace-pre-wrap break-words font-mono">{entry.detail}</pre>
          </div>
        )}
      </div>
    </div>
  )
}

export default function AuditPage() {
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [stats, setStats] = useState<LogStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [filterLevel, setFilterLevel] = useState("")
  const [filterPath, setFilterPath] = useState("")
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [selectedEntry, setSelectedEntry] = useState<LogEntry | null>(null)

  const fetchLogs = useCallback(async () => {
    try {
      let url = "/api/v1/audit/logs?limit=200"
      if (filterLevel) url += `&level=${filterLevel}`
      if (filterPath) url += `&path=${encodeURIComponent(filterPath)}`
      const res = await fetch(url)
      const data = await res.json()
      setEntries(data.entries || [])
      setStats(data.stats || null)
      setError("")
    } catch (err: any) {
      setError(err.message || "Failed to fetch logs")
    } finally {
      setLoading(false)
    }
  }, [filterLevel, filterPath])

  useEffect(() => {
    fetchLogs()
    if (!autoRefresh) return
    const interval = setInterval(fetchLogs, 3000)
    return () => clearInterval(interval)
  }, [fetchLogs, autoRefresh])

  const levelCounts = stats?.by_level || {}
  const totalEntries = stats?.total || 0

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Access Logs</h1>
          <p className="text-sm text-gray-400 mt-1">Detailed request/response log with error capture</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5">
            <div className={`w-2 h-2 rounded-full ${autoRefresh ? "bg-emerald-500 animate-pulse" : "bg-gray-600"}`} />
            <span className="text-xs text-gray-500">{autoRefresh ? "Live" : "Paused"}</span>
          </div>
          <button className="btn-primary text-xs px-3 py-1.5" onClick={() => setAutoRefresh(!autoRefresh)}>
            {autoRefresh ? "Pause" : "Live"}
          </button>
          <button className="btn-secondary text-xs px-3 py-1.5" onClick={fetchLogs}>
            Refresh
          </button>
        </div>
      </div>

      {/* Stats bar */}
      <div className="flex gap-3 flex-wrap">
        <div className="glass-card px-4 py-2 flex items-center gap-2">
          <span className="text-xs text-gray-500 uppercase tracking-wider">Total</span>
          <span className="text-lg font-semibold text-white">{totalEntries}</span>
        </div>
        {["error", "warn", "info", "debug"].map((lvl) => (
          <button
            key={lvl}
            onClick={() => setFilterLevel(filterLevel === lvl ? "" : lvl)}
            className={`glass-card px-3 py-2 flex items-center gap-2 cursor-pointer hover:opacity-80 transition-opacity ${filterLevel === lvl ? "ring-1 ring-brand-500" : ""}`}
          >
            <span className="text-xs text-gray-500 uppercase tracking-wider">{lvl}</span>
            <span className={`text-lg font-semibold ${LEVEL_COLORS[lvl]}`}>{levelCounts[lvl] || 0}</span>
          </button>
        ))}
      </div>

      {/* Filters */}
      <div className="flex gap-3 items-center">
        <input
          className="input max-w-xs text-sm"
          placeholder="Filter by path..."
          value={filterPath}
          onChange={(e) => setFilterPath(e.target.value)}
        />
        <button className="btn-secondary text-xs px-3 py-1.5" onClick={() => { setFilterLevel(""); setFilterPath(""); }}>
          Clear Filters
        </button>
      </div>

      {/* Log table */}
      <div className="glass-card overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading logs...</div>
        ) : error ? (
          <div className="p-12 text-center">
            <p className="text-red-400 mb-2">{error}</p>
            <p className="text-xs text-gray-500">Make sure the backend is running on port 8080</p>
          </div>
        ) : entries.length === 0 ? (
          <div className="p-12 text-center text-gray-500">
            <p className="mb-2">No log entries yet</p>
            <p className="text-xs text-gray-600">Logs will appear as you interact with the API</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-800">
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-16">Level</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-24">Time</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-16">Status</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">Message</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-20">Latency</th>
                  <th className="text-right py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-16">Detail</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800/50">
                {entries.map((e) => (
                  <tr
                    key={e.id}
                    className={`${LEVEL_BG[e.level]} hover:bg-surface-100/50 cursor-pointer transition-colors`}
                    onClick={() => setSelectedEntry(e)}
                  >
                    <td className="py-2 px-3">
                      <LevelBadge level={e.level} />
                    </td>
                    <td className="py-2 px-3 text-xs text-gray-400 font-mono whitespace-nowrap" title={new Date(e.timestamp).toLocaleString()}>
                      {timeAgo(e.timestamp)}
                    </td>
                    <td className="py-2 px-3">
                      <StatusBadge status={e.status} />
                    </td>
                    <td className="py-2 px-3">
                      <div className="text-sm text-gray-200 truncate max-w-md">{e.message}</div>
                    </td>
                    <td className="py-2 px-3 text-xs text-gray-400 font-mono">{e.latency || "-"}</td>
                    <td className="py-2 px-3 text-right">
                      {e.detail ? (
                        <span className="text-xs text-brand-400 cursor-pointer hover:underline" onClick={(ev) => { ev.stopPropagation(); setSelectedEntry(e); }}>
                          View
                        </span>
                      ) : (
                        <span className="text-xs text-gray-600">-</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="text-xs text-gray-600 text-center">
        Ring buffer: {stats ? `${Math.round(stats.usage_pct)}%` : "?"} full ({stats?.capacity || "?"} max entries)
      </div>

      {/* Detail modal */}
      {selectedEntry && <DetailModal entry={selectedEntry} onClose={() => setSelectedEntry(null)} />}
    </div>
  )
}
