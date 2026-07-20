"use client"

import { useState, useEffect, useCallback } from "react"

// ─── Types ──────────────────────────────────────────────

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

// ─── Constants ─────────────────────────────────────────

const METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "QUERY", "HEAD", "OPTIONS"]
const STATUS_GROUPS = [
  { label: "2xx", values: [200, 201, 202, 204] },
  { label: "3xx", values: [301, 302, 304] },
  { label: "4xx", values: [400, 401, 403, 404, 405, 409, 415, 422, 429] },
  { label: "5xx", values: [500, 501, 502, 503, 504] },
]
const TIME_PRESETS = [
  { label: "Last 5 min", seconds: 300 },
  { label: "Last 15 min", seconds: 900 },
  { label: "Last 1 hour", seconds: 3600 },
  { label: "Last 6 hours", seconds: 21600 },
  { label: "Last 24 hours", seconds: 86400 },
  { label: "All time", seconds: 0 },
]
const PAGE_SIZES = [25, 50, 100, 200]

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

const METHOD_COLORS: Record<string, string> = {
  GET: "text-emerald-400",
  POST: "text-blue-400",
  PUT: "text-amber-400",
  PATCH: "text-purple-400",
  DELETE: "text-red-400",
  QUERY: "text-cyan-400",
  HEAD: "text-gray-400",
  OPTIONS: "text-gray-400",
}

// ─── Sub-components ─────────────────────────────────────

function StatusBadge({ status }: { status?: number }) {
  if (!status) return null
  let cls = "badge-neutral"
  if (status >= 500) cls = "badge-error"
  else if (status >= 400) cls = "badge-warning"
  else if (status >= 300 && status < 400) cls = "badge-info"
  else if (status >= 200 && status < 300) cls = "badge-success"
  return <span className={`text-xs ${cls}`}>{status}</span>
}

function MethodBadge({ method }: { method?: string }) {
  if (!method) return null
  return (
    <span className={`text-xs font-mono font-medium ${METHOD_COLORS[method] || "text-gray-400"}`}>
      {method}
    </span>
  )
}

function LevelBadge({ level }: { level: string }) {
  return <span className={`text-xs ${LEVEL_BADGE[level] || "badge-neutral"}`}>{level}</span>
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

function formatTimestamp(ts: string): string {
  return new Date(ts).toLocaleString()
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
              <MethodBadge method={entry.method} />
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

// ─── Main Page ──────────────────────────────────────────

export default function AuditPage() {
  // Data
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [stats, setStats] = useState<LogStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  // Filters
  const [filterLevel, setFilterLevel] = useState("")
  const [filterMethod, setFilterMethod] = useState("")
  const [filterStatus, setFilterStatus] = useState("")
  const [filterPath, setFilterPath] = useState("")
  const [filterSourceIP, setFilterSourceIP] = useState("")
  const [filterTimePreset, setFilterTimePreset] = useState("")

  // UI state
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [selectedEntry, setSelectedEntry] = useState<LogEntry | null>(null)
  const [pageSize, setPageSize] = useState(100)
  const [page, setPage] = useState(0)
  const [showMethodMenu, setShowMethodMenu] = useState(false)
  const [showStatusMenu, setShowStatusMenu] = useState(false)
  const [showTimeMenu, setShowTimeMenu] = useState(false)

  // Build query params
  function buildParams(pageOverride?: number, limitOverride?: number) {
    const params = new URLSearchParams()
    const limit = limitOverride ?? pageSize
    const offset = (pageOverride ?? page) * limit
    params.set("limit", String(limit))
    params.set("offset", String(offset))
    if (filterLevel) params.set("level", filterLevel)
    if (filterMethod) params.set("method", filterMethod)
    if (filterStatus) params.set("status", filterStatus)
    if (filterPath) params.set("path", filterPath)
    if (filterSourceIP) params.set("source_ip", filterSourceIP)

    // Time range from preset
    if (filterTimePreset) {
      const preset = TIME_PRESETS.find((t) => t.label === filterTimePreset)
      if (preset && preset.seconds > 0) {
        params.set("since", new Date(Date.now() - preset.seconds * 1000).toISOString())
      }
    }

    return params.toString()
  }

  // Fetch
  const fetchLogs = useCallback(async () => {
    try {
      const url = `/api/v1/audit/logs?${buildParams()}`
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
  }, [filterLevel, filterMethod, filterStatus, filterPath, filterSourceIP, filterTimePreset, pageSize, page])

  // Polling
  useEffect(() => {
    fetchLogs()
    if (!autoRefresh) return
    const interval = setInterval(fetchLogs, 3000)
    return () => clearInterval(interval)
  }, [fetchLogs, autoRefresh])

  // Reset page when filters change
  useEffect(() => {
    setPage(0)
  }, [filterLevel, filterMethod, filterStatus, filterPath, filterSourceIP, filterTimePreset, pageSize])

  // Derived
  const hasFilters = !!(filterLevel || filterMethod || filterStatus || filterPath || filterSourceIP || filterTimePreset)
  const levelCounts = stats?.by_level || {}
  const totalEntries = stats?.total || 0

  function clearFilters() {
    setFilterLevel("")
    setFilterMethod("")
    setFilterStatus("")
    setFilterPath("")
    setFilterSourceIP("")
    setFilterTimePreset("")
  }

  function toggleMultiSelect(setter: (v: string) => void, current: string, value: string) {
    setter(current === value ? "" : value)
  }

  // ─── Render ───────────────────────────────────────────

  return (
    <div className="space-y-4">
      {/* ── Header ── */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Access Logs</h1>
          <p className="text-sm text-gray-400 mt-1">Real-time request/response audit trail with full traceability</p>
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

      {/* ── Stats Bar ── */}
      <div className="flex gap-3 flex-wrap">
        <div className="glass-card px-4 py-2 flex items-center gap-2 cursor-pointer hover:opacity-80 transition-opacity"
          onClick={() => setFilterLevel("")}>
          <span className="text-xs text-gray-500 uppercase tracking-wider">Total</span>
          <span className="text-lg font-semibold text-white">{totalEntries}</span>
        </div>
        {["error", "warn", "info", "debug", "fatal"].map((lvl) => (
          <button
            key={lvl}
            onClick={() => setFilterLevel(filterLevel === lvl ? "" : lvl)}
            className={`glass-card px-3 py-2 flex items-center gap-2 cursor-pointer hover:opacity-80 transition-opacity ${
              filterLevel === lvl ? "ring-1 ring-brand-500" : ""
            }`}
          >
            <span className="text-xs text-gray-500 uppercase tracking-wider">{lvl}</span>
            <span className={`text-lg font-semibold ${LEVEL_COLORS[lvl]}`}>{levelCounts[lvl] || 0}</span>
          </button>
        ))}
      </div>

      {/* ── Filter Bar ── */}
      <div className="glass-card p-4 space-y-3">
        <div className="flex items-center justify-between">
          <span className="text-xs text-gray-500 uppercase tracking-wider font-medium">Filters</span>
          {hasFilters && (
            <button className="text-xs text-brand-400 hover:text-brand-300 transition-colors" onClick={clearFilters}>
              Clear All Filters
            </button>
          )}
        </div>

        {/* Row 1: Level + Method + Status */}
        <div className="flex gap-3 flex-wrap items-start">
          {/* Level filter */}
          <div className="flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Level:</span>
            {["debug", "info", "warn", "error", "fatal"].map((lvl) => (
              <button
                key={lvl}
                onClick={() => toggleMultiSelect(setFilterLevel, filterLevel, lvl)}
                className={`px-2 py-0.5 rounded text-xs font-medium transition-all ${
                  filterLevel === lvl
                    ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50"
                    : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                }`}
              >
                {lvl}
              </button>
            ))}
          </div>

          {/* Method filter */}
          <div className="relative flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Method:</span>
            {METHODS.slice(0, 5).map((m) => (
              <button
                key={m}
                onClick={() => toggleMultiSelect(setFilterMethod, filterMethod, m)}
                className={`px-2 py-0.5 rounded text-xs font-mono font-medium transition-all ${
                  filterMethod === m
                    ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50"
                    : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                }`}
              >
                {m}
              </button>
            ))}
            <div className="relative">
              <button
                onClick={() => setShowMethodMenu(!showMethodMenu)}
                className={`px-2 py-0.5 rounded text-xs font-mono font-medium transition-all ${
                  METHOD_COLORS[filterMethod] && !METHODS.slice(0, 5).includes(filterMethod)
                    ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50"
                    : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                }`}
              >
                ...
              </button>
              {showMethodMenu && (
                <div className="absolute top-full left-0 mt-1 z-30 glass-card p-2 min-w-[120px] space-y-1">
                  {METHODS.filter((m) => !METHODS.slice(0, 5).includes(m)).map((m) => (
                    <button
                      key={m}
                      onClick={() => { toggleMultiSelect(setFilterMethod, filterMethod, m); setShowMethodMenu(false) }}
                      className={`block w-full text-left px-2 py-1 rounded text-xs font-mono transition-colors ${
                        filterMethod === m ? "bg-brand-500/20 text-brand-400" : "text-gray-400 hover:bg-gray-700 hover:text-gray-200"
                      }`}
                    >
                      {m}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Row 2: Status + Path + Source IP */}
        <div className="flex gap-3 flex-wrap items-center">
          {/* Status filter */}
          <div className="relative flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Status:</span>
            {STATUS_GROUPS.map((grp) => (
              <button
                key={grp.label}
                onClick={() => {
                  if (filterStatus === grp.label) { setFilterStatus(""); return }
                  setFilterStatus(grp.label)
                }}
                className={`px-2 py-0.5 rounded text-xs font-mono font-medium transition-all ${
                  filterStatus === grp.label
                    ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50"
                    : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                }`}
              >
                {grp.label}
              </button>
            ))}
            <div className="relative">
              <button
                onClick={() => setShowStatusMenu(!showStatusMenu)}
                className="px-2 py-0.5 rounded text-xs font-mono font-medium transition-all bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
              >
                custom...
              </button>
              {showStatusMenu && (
                <div className="absolute top-full left-0 mt-1 z-30 glass-card p-2 min-w-[120px] max-h-[200px] overflow-y-auto space-y-1">
                  {STATUS_GROUPS.flatMap((g) => g.values).map((v) => (
                    <button
                      key={v}
                      onClick={() => { setFilterStatus(String(v)); setShowStatusMenu(false) }}
                      className={`block w-full text-left px-2 py-1 rounded text-xs font-mono transition-colors ${
                        filterStatus === String(v) ? "bg-brand-500/20 text-brand-400" : "text-gray-400 hover:bg-gray-700 hover:text-gray-200"
                      }`}
                    >
                      {v}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Path filter */}
          <div className="flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Path:</span>
            <input
              className="input text-xs py-1 px-2 w-48"
              placeholder="/api/v1/..."
              value={filterPath}
              onChange={(e) => setFilterPath(e.target.value)}
            />
          </div>

          {/* Source IP filter */}
          <div className="flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Source IP:</span>
            <input
              className="input text-xs py-1 px-2 w-36"
              placeholder="127.0.0.1"
              value={filterSourceIP}
              onChange={(e) => setFilterSourceIP(e.target.value)}
            />
          </div>
        </div>

        {/* Row 3: Time range */}
        <div className="flex gap-3 items-center">
          <div className="relative flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Time:</span>
            {TIME_PRESETS.slice(0, 4).map((tp) => (
              <button
                key={tp.label}
                onClick={() => setFilterTimePreset(filterTimePreset === tp.label ? "" : tp.label)}
                className={`px-2 py-0.5 rounded text-xs font-medium transition-all ${
                  filterTimePreset === tp.label
                    ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50"
                    : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                }`}
              >
                {tp.label}
              </button>
            ))}
            <div className="relative">
              <button
                onClick={() => setShowTimeMenu(!showTimeMenu)}
                className="px-2 py-0.5 rounded text-xs font-medium transition-all bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
              >
                more...
              </button>
              {showTimeMenu && (
                <div className="absolute top-full left-0 mt-1 z-30 glass-card p-2 min-w-[140px] space-y-1">
                  {TIME_PRESETS.filter((tp) => !TIME_PRESETS.slice(0, 4).includes(tp)).map((tp) => (
                    <button
                      key={tp.label}
                      onClick={() => { setFilterTimePreset(tp.label); setShowTimeMenu(false) }}
                      className={`block w-full text-left px-2 py-1 rounded text-xs transition-colors ${
                        filterTimePreset === tp.label ? "bg-brand-500/20 text-brand-400" : "text-gray-400 hover:bg-gray-700 hover:text-gray-200"
                      }`}
                    >
                      {tp.label}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>

        {/* Active filter chips */}
        {hasFilters && (
          <div className="flex gap-1.5 flex-wrap pt-1 border-t border-gray-800/50">
            <span className="text-xs text-gray-500 self-center mr-1">Active:</span>
            {filterLevel && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                Level: {filterLevel}
                <button className="text-gray-400 hover:text-white ml-0.5" onClick={() => setFilterLevel("")}>&times;</button>
              </span>
            )}
            {filterMethod && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                Method: {filterMethod}
                <button className="text-gray-400 hover:text-white ml-0.5" onClick={() => setFilterMethod("")}>&times;</button>
              </span>
            )}
            {filterStatus && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                Status: {filterStatus}
                <button className="text-gray-400 hover:text-white ml-0.5" onClick={() => setFilterStatus("")}>&times;</button>
              </span>
            )}
            {filterPath && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400 max-w-[250px] truncate">
                Path: {filterPath}
                <button className="text-gray-400 hover:text-white ml-0.5 shrink-0" onClick={() => setFilterPath("")}>&times;</button>
              </span>
            )}
            {filterSourceIP && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                IP: {filterSourceIP}
                <button className="text-gray-400 hover:text-white ml-0.5" onClick={() => setFilterSourceIP("")}>&times;</button>
              </span>
            )}
            {filterTimePreset && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                {filterTimePreset}
                <button className="text-gray-400 hover:text-white ml-0.5" onClick={() => setFilterTimePreset("")}>&times;</button>
              </span>
            )}
          </div>
        )}
      </div>

      {/* ── Pagination (top) ── */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500">Show:</span>
          {PAGE_SIZES.map((s) => (
            <button
              key={s}
              onClick={() => setPageSize(s)}
              className={`px-2 py-0.5 rounded text-xs transition-all ${
                pageSize === s
                  ? "bg-brand-500/20 text-brand-400"
                  : "bg-gray-800 text-gray-400 hover:text-gray-200"
              }`}
            >
              {s}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <button
            className="btn-secondary text-xs px-2 py-0.5"
            disabled={page === 0}
            onClick={() => setPage(Math.max(0, page - 1))}
          >
            Prev
          </button>
          <span className="text-xs text-gray-400">Page {page + 1}</span>
          <button
            className="btn-secondary text-xs px-2 py-0.5"
            disabled={entries.length < pageSize}
            onClick={() => setPage(page + 1)}
          >
            Next
          </button>
        </div>
      </div>

      {/* ── Log Table ── */}
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
            <p className="mb-2">
              {hasFilters ? "No log entries match the current filters" : "No log entries yet"}
            </p>
            <p className="text-xs text-gray-600">
              {hasFilters ? "Try adjusting or clearing your filters" : "Logs will appear as you interact with the API"}
            </p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-800">
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-16">Level</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-24">Time</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-20">Method</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-16">Status</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">Path / Message</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-36">Source IP</th>
                  <th className="text-right py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-20">Latency</th>
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
                    <td
                      className="py-2 px-3 text-xs text-gray-400 font-mono whitespace-nowrap"
                      title={formatTimestamp(e.timestamp)}
                    >
                      {timeAgo(e.timestamp)}
                    </td>
                    <td className="py-2 px-3">
                      <MethodBadge method={e.method} />
                    </td>
                    <td className="py-2 px-3">
                      <StatusBadge status={e.status} />
                    </td>
                    <td className="py-2 px-3">
                      <div className="text-sm text-gray-200 truncate max-w-md font-mono text-xs">{e.message}</div>
                    </td>
                    <td className="py-2 px-3 text-xs text-gray-500 font-mono truncate max-w-[140px]" title={e.source_ip}>
                      {e.source_ip || "-"}
                    </td>
                    <td className="py-2 px-3 text-xs text-gray-400 font-mono text-right">{e.latency || "-"}</td>
                    <td className="py-2 px-3 text-right">
                      {e.detail ? (
                        <span
                          className="text-xs text-brand-400 cursor-pointer hover:underline"
                          onClick={(ev) => { ev.stopPropagation(); setSelectedEntry(e) }}
                        >
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

      {/* ── Footer ── */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <button
            className="btn-secondary text-xs px-2 py-0.5"
            disabled={page === 0}
            onClick={() => setPage(Math.max(0, page - 1))}
          >
            Prev
          </button>
          <span className="text-xs text-gray-400">
            Page {page + 1}
            {entries.length > 0 && ` · ${entries.length} entries shown`}
          </span>
          <button
            className="btn-secondary text-xs px-2 py-0.5"
            disabled={entries.length < pageSize}
            onClick={() => setPage(page + 1)}
          >
            Next
          </button>
        </div>
        <div className="flex items-center gap-3">
          <div className="text-xs text-gray-600">
            Ring buffer: {stats ? `${Math.round(stats.usage_pct)}%` : "?"} full
          </div>
          <div className="text-xs text-gray-600">
            {stats ? `${stats.capacity}` : "?"} max entries
          </div>
        </div>
      </div>

      {/* ── Detail Modal ── */}
      {selectedEntry && <DetailModal entry={selectedEntry} onClose={() => setSelectedEntry(null)} />}

      {/* Click-outside handlers for dropdowns */}
      {(showMethodMenu || showStatusMenu || showTimeMenu) && (
        <div className="fixed inset-0 z-20" onClick={() => {
          setShowMethodMenu(false)
          setShowStatusMenu(false)
          setShowTimeMenu(false)
        }} />
      )}
    </div>
  )
}
