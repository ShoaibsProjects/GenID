"use client"

import { useState, useEffect, useCallback, useRef } from "react"

// ─── Types ──────────────────────────────────────────────────

interface Identity {
  id: string
  tenant_id: string
  type: string
  status: string
  email: string
  display_name: string
  department?: string
  employee_id?: string
  manager_id?: string
  source: string
  risk_score: number
  risk_factors: string[]
  assurance_level: string
  attributes: string
  created_at: string
  updated_at: string
  last_accessed_at?: string
  last_reviewed_at?: string
}

// ─── Constants ──────────────────────────────────────────────

const STATUS_COLORS: Record<string, string> = {
  active: "text-emerald-400 bg-emerald-500/10 border-emerald-500/30",
  inactive: "text-gray-400 bg-gray-500/10 border-gray-500/30",
  suspended: "text-amber-400 bg-amber-500/10 border-amber-500/30",
  terminated: "text-red-400 bg-red-500/10 border-red-500/30",
  revoked: "text-red-600 bg-red-700/20 border-red-700/30",
  pending_review: "text-purple-400 bg-purple-500/10 border-purple-500/30",
}

const TYPE_COLORS: Record<string, string> = {
  human: "text-blue-400 bg-blue-500/10 border-blue-500/30",
  service_account: "text-teal-400 bg-teal-500/10 border-teal-500/30",
  ai_agent: "text-violet-400 bg-violet-500/10 border-violet-500/30",
  robot: "text-orange-400 bg-orange-500/10 border-orange-500/30",
  iot_device: "text-cyan-400 bg-cyan-500/10 border-cyan-500/30",
  rpa_bot: "text-pink-400 bg-pink-500/10 border-pink-500/30",
  api_key: "text-yellow-400 bg-yellow-500/10 border-yellow-500/30",
}

const TYPE_ICONS: Record<string, string> = {
  human: "\u{1F464}",
  service_account: "\u{2699}",
  ai_agent: "\u{1F916}",
  robot: "\u{1F916}",
  iot_device: "\u{1F4E1}",
  rpa_bot: "\u{1F4E6}",
  api_key: "\u{1F511}",
}

const RISK_CLASSES = [
  { max: 0.2, color: "text-emerald-400", bg: "bg-emerald-500/10", label: "Low" },
  { max: 0.5, color: "text-amber-400", bg: "bg-amber-500/10", label: "Medium" },
  { max: 1.0, color: "text-red-400", bg: "bg-red-500/10", label: "High" },
]

function getRiskStyle(score: number) {
  for (const r of RISK_CLASSES) {
    if (score <= r.max) return r
  }
  return RISK_CLASSES[RISK_CLASSES.length - 1]
}

const PAGE_SIZES = [10, 25, 50, 100]
const STATUSES = ["active", "inactive", "suspended", "terminated", "revoked", "pending_review"]
const TYPES = ["human", "service_account", "ai_agent", "robot", "iot_device", "rpa_bot", "api_key"]
const SOURCES = ["manual", "hris", "scim", "ldap", "agent_registration", "discovery", "saml"]

// ─── Main Page ──────────────────────────────────────────────

export default function IdentitiesPage() {
  const [identities, setIdentities] = useState<Identity[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  // Search & Filters
  const [search, setSearch] = useState("")
  const [searchInput, setSearchInput] = useState("")
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [statusFilter, setStatusFilter] = useState("")
  const [typeFilter, setTypeFilter] = useState("")
  const [deptFilter, setDeptFilter] = useState("")
  const [sourceFilter, setSourceFilter] = useState("")

  // Sorting
  const [sortBy, setSortBy] = useState("created_at")
  const [sortDir, setSortDir] = useState("desc")

  // Pagination
  const [pageSize, setPageSize] = useState(25)
  const [page, setPage] = useState(0)

  // Detail panel
  const [selected, setSelected] = useState<Identity | null>(null)
  const [detailData, setDetailData] = useState<any>(null)
  const [detailLoading, setDetailLoading] = useState(false)

  // Add identity modal
  const [showAdd, setShowAdd] = useState(false)
  const [addForm, setAddForm] = useState({
    email: "", display_name: "", first_name: "", last_name: "",
    phone: "", type: "human", department: "", title: "",
    employee_id: "", source: "manual",
  })

  const hasFilters = !!(statusFilter || typeFilter || deptFilter || sourceFilter || search)

  // ── Build params & fetch ──────────────────────────────
  function buildParams(overridePage?: number) {
    const p = new URLSearchParams()
    p.set("limit", String(pageSize))
    p.set("offset", String((overridePage ?? page) * pageSize))
    if (search) p.set("search", search)
    if (statusFilter) p.set("status", statusFilter)
    if (typeFilter) p.set("type", typeFilter)
    if (deptFilter) p.set("department", deptFilter)
    if (sourceFilter) p.set("source", sourceFilter)
    p.set("sort_by", sortBy)
    p.set("sort_dir", sortDir)
    return p.toString()
  }

  const fetchIdentities = useCallback(async () => {
    setLoading(true)
    setError("")
    try {
      const res = await fetch(`/api/v1/identities?${buildParams()}`)
      const data = await res.json()
      setIdentities(data.identities || [])
      setTotal(data.total || 0)
    } catch (err: any) {
      setError(err.message || "Failed to load identities")
    } finally {
      setLoading(false)
    }
  }, [search, statusFilter, typeFilter, deptFilter, sourceFilter, sortBy, sortDir, pageSize, page])

  useEffect(() => { fetchIdentities() }, [fetchIdentities])

  // Debounced search
  useEffect(() => {
    if (searchTimer.current) clearTimeout(searchTimer.current)
    searchTimer.current = setTimeout(() => {
      setSearch(searchInput)
      setPage(0)
    }, 300)
    return () => { if (searchTimer.current) clearTimeout(searchTimer.current) }
  }, [searchInput])

  // Reset page on filter changes
  useEffect(() => { setPage(0) }, [statusFilter, typeFilter, deptFilter, sourceFilter, sortBy, sortDir, pageSize, search])

  // ── Sort toggle ───────────────────────────────────────
  function toggleSort(column: string) {
    if (sortBy === column) {
      setSortDir(sortDir === "asc" ? "desc" : "asc")
    } else {
      setSortBy(column)
      setSortDir("asc")
    }
  }

  function sortIcon(column: string) {
    if (sortBy !== column) return <span className="text-gray-600 ml-1">\u2195</span>
    return sortDir === "asc"
      ? <span className="text-brand-400 ml-1">\u2191</span>
      : <span className="text-brand-400 ml-1">\u2193</span>
  }

  // ── Detail panel ─────────────────────────────────────
  async function openDetail(id: Identity) {
    setSelected(id)
    setDetailLoading(true)
    try {
      const [ident, ent, blast] = await Promise.all([
        fetch(`/api/v1/identities/${id.id}`).then(r => r.json()).catch(() => null),
        fetch(`/api/v1/identities/${id.id}/entitlements`).then(r => r.json()).catch(() => null),
        fetch(`/api/v1/identities/${id.id}/blast-radius`).then(r => r.json()).catch(() => null),
      ])
      setDetailData({ identity: ident, entitlements: ent, blast_radius: blast })
    } catch (_) {
      setDetailData(null)
    } finally {
      setDetailLoading(false)
    }
  }

  // ── Add identity ─────────────────────────────────────
  async function handleAdd() {
    try {
      const res = await fetch("/api/v1/identities", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(addForm),
      })
      if (!res.ok) throw new Error(await res.text())
      setShowAdd(false)
      setAddForm({ email: "", display_name: "", first_name: "", last_name: "", phone: "", type: "human", department: "", title: "", employee_id: "", source: "manual" })
      fetchIdentities()
    } catch (e: any) {
      alert("Create failed: " + e.message)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Terminate this identity?")) return
    try {
      await fetch(`/api/v1/identities/${id}`, { method: "DELETE" })
      fetchIdentities()
    } catch (e: any) {
      alert("Delete failed: " + e.message)
    }
  }

  // ── Render ────────────────────────────────────────────
  return (
    <div className="space-y-4">
      {/* ── Header ── */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Identities</h1>
          <p className="text-sm text-gray-400 mt-1">
            {total.toLocaleString()} total · {identities.length} shown
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button className="btn-secondary text-xs px-3 py-1.5" onClick={fetchIdentities}>
            Refresh
          </button>
          <button className="btn-primary text-xs px-3 py-1.5" onClick={() => setShowAdd(true)}>
            + Add Identity
          </button>
        </div>
      </div>

      {/* ── Search & Filter Bar ── */}
      <div className="glass-card p-3 space-y-3">
        {/* Search */}
        <div className="flex gap-3 items-center">
          <div className="relative flex-1 max-w-md">
            <input
              className="input text-sm py-1.5 pl-8 w-full"
              placeholder="Search by name or email..."
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
            />
            <svg className="absolute left-2.5 top-2 w-3.5 h-3.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            {searchInput && (
              <button
                className="absolute right-2 top-1.5 text-gray-500 hover:text-gray-300"
                onClick={() => { setSearchInput(""); setSearch("") }}
              >
                &times;
              </button>
            )}
          </div>

          {/* Status filter */}
          <div className="flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Status:</span>
            {STATUSES.map((s) => (
              <button
                key={s}
                onClick={() => setStatusFilter(statusFilter === s ? "" : s)}
                className={`px-2 py-0.5 rounded text-xs font-medium transition-all ${
                  statusFilter === s
                    ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50"
                    : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                }`}
              >
                {s.replace("_", " ")}
              </button>
            ))}
          </div>
        </div>

        {/* Row 2: Type + Source + Clear */}
        <div className="flex gap-3 items-center flex-wrap">
          <div className="flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Type:</span>
            {TYPES.map((t) => (
              <button
                key={t}
                onClick={() => setTypeFilter(typeFilter === t ? "" : t)}
                className={`px-2 py-0.5 rounded text-xs font-medium transition-all ${
                  typeFilter === t
                    ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50"
                    : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                }`}
              >
                {t.replace("_", " ")}
              </button>
            ))}
          </div>

          <div className="flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Source:</span>
            {SOURCES.map((s) => (
              <button
                key={s}
                onClick={() => setSourceFilter(sourceFilter === s ? "" : s)}
                className={`px-2 py-0.5 rounded text-xs font-medium transition-all ${
                  sourceFilter === s
                    ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50"
                    : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"
                }`}
              >
                {s.replace("_", " ")}
              </button>
            ))}
          </div>

          <div className="flex items-center gap-1">
            <span className="text-xs text-gray-500 mr-1">Dept:</span>
            <input
              className="input text-xs py-1 px-2 w-36"
              placeholder="Engineering"
              value={deptFilter}
              onChange={(e) => setDeptFilter(e.target.value)}
            />
          </div>

          {hasFilters && (
            <button
              className="text-xs text-brand-400 hover:text-brand-300 ml-auto"
              onClick={() => {
                setSearch(""); setSearchInput("");
                setStatusFilter(""); setTypeFilter(""); setDeptFilter(""); setSourceFilter("");
              }}
            >
              Clear All
            </button>
          )}
        </div>

        {/* Active filter chips */}
        {hasFilters && (
          <div className="flex gap-1.5 flex-wrap pt-1 border-t border-gray-800/50">
            {search && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                "{search}"
                <button className="hover:text-white ml-0.5" onClick={() => { setSearch(""); setSearchInput("") }}>&times;</button>
              </span>
            )}
            {statusFilter && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                Status: {statusFilter}
                <button className="hover:text-white ml-0.5" onClick={() => setStatusFilter("")}>&times;</button>
              </span>
            )}
            {typeFilter && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                Type: {typeFilter}
                <button className="hover:text-white ml-0.5" onClick={() => setTypeFilter("")}>&times;</button>
              </span>
            )}
            {sourceFilter && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                Source: {sourceFilter}
                <button className="hover:text-white ml-0.5" onClick={() => setSourceFilter("")}>&times;</button>
              </span>
            )}
            {deptFilter && (
              <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">
                Dept: {deptFilter}
                <button className="hover:text-white ml-0.5" onClick={() => setDeptFilter("")}>&times;</button>
              </span>
            )}
          </div>
        )}
      </div>

      {/* ── Pagination top ── */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500">Show:</span>
          {PAGE_SIZES.map((s) => (
            <button
              key={s}
              onClick={() => { setPageSize(s); setPage(0) }}
              className={`px-2 py-0.5 rounded text-xs transition-all ${
                pageSize === s ? "bg-brand-500/20 text-brand-400" : "bg-gray-800 text-gray-400 hover:text-gray-200"
              }`}
            >
              {s}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <button className="btn-secondary text-xs px-2 py-0.5" disabled={page === 0} onClick={() => setPage(Math.max(0, page - 1))}>
            Prev
          </button>
          <span className="text-xs text-gray-400">Page {page + 1} of {Math.max(1, Math.ceil(total / pageSize))}</span>
          <button className="btn-secondary text-xs px-2 py-0.5" disabled={(page + 1) * pageSize >= total} onClick={() => setPage(page + 1)}>
            Next
          </button>
        </div>
      </div>

      {/* ── Identity Table ── */}
      <div className="glass-card overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading identities...</div>
        ) : error ? (
          <div className="p-12 text-center text-red-400">{error}</div>
        ) : identities.length === 0 ? (
          <div className="p-12 text-center text-gray-500">
            <p className="mb-2">{hasFilters ? "No identities match the current filters" : "No identities found"}</p>
            <p className="text-xs text-gray-600">{hasFilters ? "Try adjusting or clearing your filters" : "Add identities via the + Add button or import a CSV"}</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-800">
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase cursor-pointer select-none hover:text-gray-300"
                    onClick={() => toggleSort("display_name")}>
                    Identity {sortIcon("display_name")}
                  </th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase cursor-pointer select-none hover:text-gray-300"
                    onClick={() => toggleSort("email")}>
                    Email {sortIcon("email")}
                  </th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase cursor-pointer select-none hover:text-gray-300"
                    onClick={() => toggleSort("department")}>
                    Department {sortIcon("department")}
                  </th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase cursor-pointer select-none hover:text-gray-300"
                    onClick={() => toggleSort("status")}>
                    Status {sortIcon("status")}
                  </th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase cursor-pointer select-none hover:text-gray-300"
                    onClick={() => toggleSort("type")}>
                    Type {sortIcon("type")}
                  </th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase cursor-pointer select-none hover:text-gray-300"
                    onClick={() => toggleSort("risk_score")}>
                    Risk {sortIcon("risk_score")}
                  </th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase cursor-pointer select-none hover:text-gray-300"
                    onClick={() => toggleSort("source")}>
                    Source {sortIcon("source")}
                  </th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase cursor-pointer select-none hover:text-gray-300"
                    onClick={() => toggleSort("created_at")}>
                    Created {sortIcon("created_at")}
                  </th>
                  <th className="text-right py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-20">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800/50">
                {identities.map((id) => {
                  const risk = getRiskStyle(id.risk_score)
                  const type = id.type || "human"
                  return (
                    <tr
                      key={id.id}
                      className="hover:bg-surface-100/50 cursor-pointer transition-colors"
                      onClick={() => openDetail(id)}
                    >
                      <td className="py-2 px-3">
                        <div className="flex items-center gap-2">
                          <span className="shrink-0 w-7 h-7 rounded-full bg-brand-500/15 flex items-center justify-center text-xs font-semibold text-brand-400">
                            {(id.display_name || id.email || "?").charAt(0).toUpperCase()}
                          </span>
                          <div>
                            <div className="text-sm text-gray-200 font-medium">{id.display_name || id.email}</div>
                            <div className="text-xs text-gray-500">{id.employee_id && id.employee_id !== "" ? id.employee_id : TYPE_ICONS[type] + " " + type.replace("_", " ")}</div>
                          </div>
                        </div>
                      </td>
                      <td className="py-2 px-3 text-sm text-gray-300 font-mono text-xs">{id.email}</td>
                      <td className="py-2 px-3 text-sm text-gray-300">{id.department || "-"}</td>
                      <td className="py-2 px-3">
                        <span className={`px-2 py-0.5 rounded-full text-xs border ${STATUS_COLORS[id.status] || ""}`}>
                          {id.status?.replace("_", " ")}
                        </span>
                      </td>
                      <td className="py-2 px-3">
                        <span className={`px-2 py-0.5 rounded-full text-xs border ${TYPE_COLORS[type] || ""}`}>
                          {type.replace("_", " ")}
                        </span>
                      </td>
                      <td className="py-2 px-3">
                        <span className={`text-sm font-mono font-medium ${risk.color}`}>
                          {id.risk_score > 0 ? id.risk_score.toFixed(2) : "0"}
                        </span>
                        {id.risk_factors?.length > 0 && (
                          <div className="text-xs text-gray-500 truncate max-w-[100px]">
                            {id.risk_factors.join(", ")}
                          </div>
                        )}
                      </td>
                      <td className="py-2 px-3">
                        <span className="text-xs text-gray-400 font-mono">{id.source || "manual"}</span>
                      </td>
                      <td className="py-2 px-3 text-xs text-gray-400 font-mono whitespace-nowrap">
                        {id.created_at ? new Date(id.created_at).toLocaleDateString() : "-"}
                      </td>
                      <td className="py-2 px-3 text-right" onClick={(e) => e.stopPropagation()}>
                        <button
                          className="text-xs text-red-400 hover:text-red-300 px-2 py-0.5"
                          onClick={() => handleDelete(id.id)}
                          title="Terminate identity"
                        >
                          Del
                        </button>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* ── Pagination bottom ── */}
      <div className="flex items-center justify-between">
        <span className="text-xs text-gray-500">
          Showing {page * pageSize + 1}\u2013{Math.min((page + 1) * pageSize, total)} of {total.toLocaleString()}
        </span>
        <div className="flex items-center gap-2">
          <button className="btn-secondary text-xs px-2 py-0.5" disabled={page === 0} onClick={() => setPage(Math.max(0, page - 1))}>
            Prev
          </button>
          <span className="text-xs text-gray-400">Page {page + 1}</span>
          <button className="btn-secondary text-xs px-2 py-0.5" disabled={(page + 1) * pageSize >= total} onClick={() => setPage(page + 1)}>
            Next
          </button>
        </div>
      </div>

      {/* ── Detail Panel ── */}
      {selected && (
        <div className="fixed inset-0 z-50 flex justify-end">
          <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={() => { setSelected(null); setDetailData(null) }} />
          <div
            className="relative z-10 w-full max-w-2xl h-full overflow-y-auto bg-surface border-l border-gray-800 shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Close button */}
            <button
              className="absolute top-3 right-3 text-gray-400 hover:text-white text-xl leading-none z-20"
              onClick={() => { setSelected(null); setDetailData(null) }}
            >
              &times;
            </button>

            {detailLoading ? (
              <div className="p-8 text-center text-gray-500">Loading details...</div>
            ) : (
              <div className="p-6 space-y-6 pt-12">
                {/* Header */}
                <div className="flex items-start gap-4">
                  <div className="w-14 h-14 rounded-xl bg-brand-500/15 flex items-center justify-center text-2xl font-bold text-brand-400 shrink-0">
                    {(selected.display_name || selected.email).charAt(0).toUpperCase()}
                  </div>
                  <div>
                    <h2 className="text-xl font-bold text-white">{selected.display_name}</h2>
                    <p className="text-sm text-gray-400">{selected.email}</p>
                    <div className="flex gap-2 mt-2">
                      <span className={`px-2 py-0.5 rounded-full text-xs border ${STATUS_COLORS[selected.status] || ""}`}>
                        {selected.status?.replace("_", " ")}
                      </span>
                      <span className={`px-2 py-0.5 rounded-full text-xs border ${TYPE_COLORS[selected.type] || ""}`}>
                        {selected.type?.replace("_", " ")}
                      </span>
                      {selected.assurance_level && (
                        <span className="px-2 py-0.5 rounded-full text-xs border bg-gray-800 border-gray-600 text-gray-400 font-mono">
                          {selected.assurance_level.toUpperCase()}
                        </span>
                      )}
                    </div>
                  </div>
                </div>

                {/* Profile fields */}
                <div className="space-y-3">
                  <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider">Profile</h3>
                  <div className="grid grid-cols-2 gap-3">
                    {[
                      ["ID", selected.id],
                      ["Tenant", selected.tenant_id],
                      ["Email", selected.email],
                      ["Department", selected.department || "-"],
                      ["Employee ID", selected.employee_id || "-"],
                      ["Source", selected.source || "manual"],
                      ["Risk Score", String(selected.risk_score)],
                      ["Assurance", selected.assurance_level || "aal1"],
                      ["Created", selected.created_at ? new Date(selected.created_at).toLocaleString() : "-"],
                      ["Updated", selected.updated_at ? new Date(selected.updated_at).toLocaleString() : "-"],
                      ["Last Accessed", selected.last_accessed_at ? new Date(selected.last_accessed_at).toLocaleString() : "Never"],
                      ["Last Reviewed", selected.last_reviewed_at ? new Date(selected.last_reviewed_at).toLocaleString() : "Never"],
                    ].map(([label, value]) => (
                      <div key={label} className="p-2 rounded bg-surface-100/30">
                        <span className="text-gray-500 block text-xs uppercase tracking-wider">{label}</span>
                        <span className="text-gray-200 text-sm font-mono break-all">{String(value).substring(0, 50)}</span>
                      </div>
                    ))}
                  </div>

                  {selected.risk_factors?.length > 0 && (
                    <div className="p-3 rounded bg-red-900/10 border border-red-900/30">
                      <span className="text-red-400 block text-xs uppercase tracking-wider mb-1">Risk Factors</span>
                      <div className="flex gap-1.5 flex-wrap">
                        {selected.risk_factors.map((f: string) => (
                          <span key={f} className="px-2 py-0.5 rounded-full text-xs bg-red-500/20 text-red-400">{f}</span>
                        ))}
                      </div>
                    </div>
                  )}
                </div>

                {/* Attributes */}
                {selected.attributes && selected.attributes !== "{}" && (
                  <div className="space-y-3">
                    <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider">Attributes</h3>
                    <div className="p-3 rounded bg-surface-100/30">
                      <pre className="text-xs text-gray-300 font-mono whitespace-pre-wrap break-all">{selected.attributes}</pre>
                    </div>
                  </div>
                )}

                {/* Entitlements */}
                {detailData?.entitlements && (
                  <div className="space-y-3">
                    <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider">
                      Entitlements ({detailData.entitlements.entitlements?.length || 0})
                    </h3>
                    {detailData.entitlements.entitlements?.length > 0 ? (
                      <div className="space-y-1">
                        {detailData.entitlements.entitlements.map((e: any, i: number) => (
                          <div key={i} className="p-2 rounded bg-surface-100/30 flex items-center justify-between text-sm">
                            <span className="text-gray-200">{e.app_name || e.name || e.resource || "?"}</span>
                            <span className="text-xs text-gray-500 font-mono">{e.permission_level || e.access_type || "?"}</span>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <p className="text-xs text-gray-500">No entitlements found</p>
                    )}
                  </div>
                )}

                {/* Blast Radius */}
                {detailData?.blast_radius && (
                  <div className="space-y-3">
                    <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider">
                      Blast Radius ({detailData.blast_radius.blast_radius?.length || 0})
                    </h3>
                    {detailData.blast_radius.blast_radius?.length > 0 ? (
                      <div className="space-y-1">
                        {detailData.blast_radius.blast_radius.map((b: any, i: number) => (
                          <div key={i} className="p-2 rounded bg-surface-100/30 flex items-center justify-between text-sm">
                            <span className="text-gray-200">{b.resource || b.name || "?"}</span>
                            <div className="flex gap-2">
                              <Badge color={b.criticality === "critical" ? "red" : b.criticality === "high" ? "amber" : "gray"}>
                                {b.criticality || "?"}
                              </Badge>
                              <span className="text-xs text-gray-500 font-mono">{b.permission_level || "?"}</span>
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <p className="text-xs text-gray-500">No blast radius data</p>
                    )}
                  </div>
                )}

                {/* Actions */}
                <div className="flex gap-2 pt-4 border-t border-gray-800">
                  <button className="btn-secondary text-xs px-3 py-1.5" onClick={() => {
                    setSelected(null); setDetailData(null)
                  }}>
                    Close
                  </button>
                  <button className="btn-danger text-xs px-3 py-1.5" onClick={() => {
                    handleDelete(selected.id)
                    setSelected(null); setDetailData(null)
                  }}>
                    Terminate
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* ── Add Identity Modal ── */}
      {showAdd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setShowAdd(false)}>
          <div className="w-full max-w-lg glass-card p-6 space-y-4" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-semibold text-white">Add Identity</h2>
              <button className="text-gray-400 hover:text-white text-xl" onClick={() => setShowAdd(false)}>&times;</button>
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="col-span-2">
                <label className="text-xs text-gray-400 block mb-0.5">Email *</label>
                <input className="input text-sm py-1.5" value={addForm.email} onChange={(e) => setAddForm({...addForm, email: e.target.value})} />
              </div>
              <div className="col-span-2">
                <label className="text-xs text-gray-400 block mb-0.5">Display Name *</label>
                <input className="input text-sm py-1.5" value={addForm.display_name} onChange={(e) => setAddForm({...addForm, display_name: e.target.value})} />
              </div>
              <div>
                <label className="text-xs text-gray-400 block mb-0.5">First Name</label>
                <input className="input text-sm py-1.5" value={addForm.first_name} onChange={(e) => setAddForm({...addForm, first_name: e.target.value})} />
              </div>
              <div>
                <label className="text-xs text-gray-400 block mb-0.5">Last Name</label>
                <input className="input text-sm py-1.5" value={addForm.last_name} onChange={(e) => setAddForm({...addForm, last_name: e.target.value})} />
              </div>
              <div>
                <label className="text-xs text-gray-400 block mb-0.5">Type</label>
                <select className="input text-sm py-1.5" value={addForm.type} onChange={(e) => setAddForm({...addForm, type: e.target.value})}>
                  {TYPES.map((t) => <option key={t} value={t}>{t.replace("_", " ")}</option>)}
                </select>
              </div>
              <div>
                <label className="text-xs text-gray-400 block mb-0.5">Source</label>
                <select className="input text-sm py-1.5" value={addForm.source} onChange={(e) => setAddForm({...addForm, source: e.target.value})}>
                  {SOURCES.map((s) => <option key={s} value={s}>{s.replace("_", " ")}</option>)}
                </select>
              </div>
              <div>
                <label className="text-xs text-gray-400 block mb-0.5">Department</label>
                <input className="input text-sm py-1.5" value={addForm.department} onChange={(e) => setAddForm({...addForm, department: e.target.value})} />
              </div>
              <div>
                <label className="text-xs text-gray-400 block mb-0.5">Title</label>
                <input className="input text-sm py-1.5" value={addForm.title} onChange={(e) => setAddForm({...addForm, title: e.target.value})} />
              </div>
              <div>
                <label className="text-xs text-gray-400 block mb-0.5">Employee ID</label>
                <input className="input text-sm py-1.5" value={addForm.employee_id} onChange={(e) => setAddForm({...addForm, employee_id: e.target.value})} />
              </div>
              <div>
                <label className="text-xs text-gray-400 block mb-0.5">Phone</label>
                <input className="input text-sm py-1.5" value={addForm.phone} onChange={(e) => setAddForm({...addForm, phone: e.target.value})} />
              </div>
            </div>

            <div className="flex gap-2 justify-end pt-2">
              <button className="btn-secondary text-xs px-4 py-2" onClick={() => setShowAdd(false)}>Cancel</button>
              <button className="btn-primary text-xs px-4 py-2" onClick={handleAdd}>Create</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// tiny badge helper
function Badge({ children, color }: { children: React.ReactNode; color: string }) {
  const colors: Record<string, string> = {
    red: "text-red-400 bg-red-500/10 border-red-500/30",
    amber: "text-amber-400 bg-amber-500/10 border-amber-500/30",
    gray: "text-gray-400 bg-gray-500/10 border-gray-500/30",
    green: "text-emerald-400 bg-emerald-500/10 border-emerald-500/30",
  }
  return <span className={`px-1.5 py-0.5 rounded text-xs border font-mono ${colors[color] || colors.gray}`}>{children}</span>
}
