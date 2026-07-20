"use client"

import { useState, useEffect, useCallback, useRef } from "react"
import {
  fetchConnectors, createConnector, connectConnector, deleteConnector,
  fetchConnectorIdentities, fetchConnectorGroups, fetchConnectorEntitlements,
  fetchConnectorResources, fullSyncConnector,
} from "@/lib/api"

// ─── Types ──────────────────────────────────────────────────

interface SyncStats {
  users: number; groups: number; entitlements: number; resources: number
}

interface HealthReport {
  connector_id?: string; connector_name?: string; status: string
  delta_supported?: boolean; supports_schema?: boolean
  last_sync_duration?: string; consecutive_success?: number
  consecutive_errors?: number; last_error?: string
}

interface Connector {
  id: string; tenant_id: string; name: string; type: string
  status: string; last_sync_at?: string; last_error?: string
  created_at: string; updated_at: string
  health?: HealthReport; sync_stats?: SyncStats
}

interface ConnectorStats {
  total_connectors: number; connected_count: number
  disconnected_count: number; error_count: number; syncing_count: number
  total_identities: number; total_groups: number
  total_entitlements: number; total_resources: number
}

const CONNECTOR_TYPES: Record<string, string> = {
  entra_id: "Microsoft Entra ID", ldap: "LDAP", active_directory: "Active Directory",
  scim: "SCIM 2.0", okta: "Okta", aws_iam: "AWS IAM",
  gcp_iam: "GCP IAM", generic: "Generic", csv: "CSV Import",
}

const STATUS_COLORS: Record<string, string> = {
  connected: "text-emerald-400 bg-emerald-500/10 border-emerald-500/30",
  disconnected: "text-gray-400 bg-gray-500/10 border-gray-500/30",
  error: "text-red-400 bg-red-500/10 border-red-500/30",
  syncing: "text-amber-400 bg-amber-500/10 border-amber-500/30",
  degraded: "text-yellow-400 bg-yellow-500/10 border-yellow-500/30",
}

const CONNECTOR_TYPE_FIELDS: Record<string, {label: string; key: string; placeholder: string}[]> = {
  entra_id: [
    {label:"Tenant Name", key:"tenant_name", placeholder:"contoso.onmicrosoft.com"},
    {label:"Client ID", key:"client_id", placeholder:"Application client ID"},
    {label:"Client Secret", key:"client_secret", placeholder:"Application client secret"},
  ],
  active_directory: [
    {label:"Host:Port", key:"endpoint", placeholder:"dc01.contoso.com:389"},
    {label:"Base DN", key:"base_dn", placeholder:"DC=contoso,DC=com"},
    {label:"Username", key:"username", placeholder:"CN=Admin,..."},
    {label:"Password", key:"password", placeholder:"Password"},
    {label:"Domain", key:"domain", placeholder:"CONTOSO"},
  ],
  ldap: [
    {label:"Host:Port", key:"endpoint", placeholder:"ldap.example.com:389"},
    {label:"Base DN", key:"base_dn", placeholder:"dc=example,dc=com"},
    {label:"Username", key:"username", placeholder:"cn=admin,..."},
    {label:"Password", key:"password", placeholder:"Password"},
  ],
  scim: [
    {label:"Endpoint URL", key:"endpoint", placeholder:"https://api.example.com/scim/v2"},
    {label:"Bearer Token", key:"password", placeholder:"sk-..."},
  ],
  okta: [
    {label:"Okta Domain", key:"endpoint", placeholder:"https://your-domain.okta.com"},
    {label:"API Token", key:"password", placeholder:"SSWS token..."},
  ],
  aws_iam: [
    {label:"Access Key ID", key:"client_id", placeholder:"AKIA..."},
    {label:"Secret Access Key", key:"client_secret", placeholder:"Secret..."},
  ],
  gcp_iam: [
    {label:"Service Account Key (JSON)", key:"properties", placeholder:"Paste JSON key..."},
  ],
  csv: [
    {label:"CSV Data", key:"properties", placeholder:"Upload CSV file..."},
  ],
  generic: [
    {label:"Endpoint", key:"endpoint", placeholder:"https://api.example.com"},
    {label:"Auth Type", key:"auth_type", placeholder:"oauth2"},
    {label:"Username", key:"username", placeholder:"admin"},
    {label:"Password", key:"password", placeholder:"Password"},
    {label:"Client ID", key:"client_id", placeholder:"Client ID"},
    {label:"Client Secret", key:"client_secret", placeholder:"Client Secret"},
  ],
}

type TabKey = "accounts" | "groups" | "entitlements" | "resources" | "schema"

// ─── Main Page ──────────────────────────────────────────────

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [stats, setStats] = useState<ConnectorStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  // Filters
  const [search, setSearch] = useState("")
  const [searchInput, setSearchInput] = useState("")
  const searchTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [statusFilter, setStatusFilter] = useState("")
  const [typeFilter, setTypeFilter] = useState("")

  // UI state
  const [showAdd, setShowAdd] = useState(false)
  const [addType, setAddType] = useState("entra_id")
  const [addForm, setAddForm] = useState<Record<string, string>>({ name: "", auth_type: "oauth2" })
  const [testResult, setTestResult] = useState<any>(null)
  const [expanded, setExpanded] = useState<string | null>(null)
  const expandedRef = useRef<string | null>(null)
  const [activeTab, setActiveTab] = useState<TabKey>("accounts")
  const [tabData, setTabData] = useState<Record<string, any>>({})
  const [tabLoading, setTabLoading] = useState<Record<string, boolean>>({})
  const [busySync, setBusySync] = useState<string | null>(null)
  const [testMsg, setTestMsg] = useState<Record<string, { ok: boolean; msg: string }>>({})

  const hasFilters = !!(statusFilter || typeFilter || search)

  // ── Debounced search ──
  useEffect(() => {
    if (searchTimer.current) clearTimeout(searchTimer.current)
    searchTimer.current = setTimeout(() => setSearch(searchInput), 300)
    return () => { if (searchTimer.current) clearTimeout(searchTimer.current) }
  }, [searchInput])

  // ── Fetch ──
  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      if (search) params.set("search", search)
      if (statusFilter) params.set("status", statusFilter)
      if (typeFilter) params.set("type", typeFilter)

      const [connData, statsData] = await Promise.all([
        fetch("/api/v1/connectors?" + params.toString()).then(r => r.json()).catch(() => ({ connectors: [], total: 0 })),
        fetch("/api/v1/connectors/stats").then(r => r.json()).catch(() => null),
      ])
      setConnectors(connData.connectors || [])
      setStats(statsData)
      setError("")
    } catch (e: any) {
      setError(e.message)
    } finally { setLoading(false) }
  }, [search, statusFilter, typeFilter])

  useEffect(() => { load() }, [load])

  // ── Actions ──
  async function handleCreate() {
    try {
      const type = addType
      await createConnector({ name: addForm.name || type + "-" + Date.now(), type: type as any, ...addForm } as any)
      setShowAdd(false)
      setAddForm({ name: "", auth_type: "oauth2" })
      load()
    } catch (e: any) { alert("Create failed: " + e.message) }
  }

  async function handleTest() {
    try {
      const r = await fetch("/api/v1/connectors/test", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: addForm.name || "test", type: addType, ...addForm }),
      }).then(r => r.json())
      setTestResult(r)
    } catch (e: any) { setTestResult({ success: false, error: e.message }) }
  }

  async function handleTestConnector(id: string) {
    setTestMsg(t => ({ ...t, [id]: { ok: false, msg: "Testing..." } }))
    try {
      const r = await fetch(`/api/v1/connectors/${id}/test`, { method: "POST", headers: { "Content-Type": "application/json" } }).then(r => r.json())
      setTestMsg(t => ({ ...t, [id]: { ok: r.success, msg: r.success ? "OK" : (r.error || "Failed") } }))
      setTimeout(() => setTestMsg(t => { const n = { ...t }; delete n[id]; return n }), 3000)
    } catch (e: any) {
      setTestMsg(t => ({ ...t, [id]: { ok: false, msg: e.message } }))
      setTimeout(() => setTestMsg(t => { const n = { ...t }; delete n[id]; return n }), 3000)
    }
  }

  async function handleConnect(id: string) {
    try { await connectConnector(id); load() } catch (e: any) { alert("Connect failed: " + e.message) }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this connector and all synced data?")) return
    try { await deleteConnector(id); if (expanded === id) setExpanded(null); load() } catch (e: any) { alert("Delete failed: " + e.message) }
  }

  async function handleSync(id: string, delta?: boolean) {
    setBusySync(id)
    try {
      const url = delta ? `/api/v1/connectors/${id}/sync-delta` : `/api/v1/connectors/${id}/sync`
      await fetch(url, { method: "POST", headers: { "Content-Type": "application/json" } })
      load()
      if (expanded === id) loadTab(id, activeTab)
    } catch (e: any) { alert("Sync failed: " + e.message) } finally { setBusySync(null) }
  }

  async function handleFullSync(id: string) {
    setBusySync(id)
    try {
      await fullSyncConnector(id)
      await load()
      if (expanded === id) { setTabData({}); loadTab(id, activeTab) }
    } catch (e: any) { alert("Full sync failed: " + e.message) } finally { setBusySync(null) }
  }

  async function loadTab(id: string, tab: TabKey) {
    setActiveTab(tab)
    const key = `${id}:${tab}`
    setTabLoading(t => ({ ...t, [key]: true }))
    try {
      let data: any = null
      switch (tab) {
        case "accounts":
          data = await fetchConnectorIdentities(id)
          break
        case "groups":
          data = await fetchConnectorGroups(id)
          break
        case "entitlements":
          data = await fetchConnectorEntitlements(id)
          break
        case "resources":
          data = await fetchConnectorResources(id)
          break
        case "schema": {
          const d = await fetch(`/api/v1/connectors/${id}/schema`).then(r => r.json()).catch(() => null)
          data = d?.schema ? { schema: d.schema } : null
          break
        }
      }
      if (expandedRef.current !== id) return
      setTabData(t => ({ ...t, [key]: data }))
    } catch (_) { } finally {
      if (expandedRef.current === id) setTabLoading(t => ({ ...t, [key]: false }))
    }
  }

  function toggle(id: string) {
    if (expanded === id) { setExpanded(null); expandedRef.current = null; setActiveTab("accounts"); setTabData({}); return }
    expandedRef.current = id
    setExpanded(id)
    setActiveTab("accounts")
    setTabData({})
    loadTab(id, "accounts")
  }

  // ── Helpers ──
  function safeFormatDate(d?: string) {
    if (!d) return "Never"
    return new Date(d).toLocaleDateString()
  }

  function syncCounts(c: Connector) {
    return c.sync_stats || { users: 0, groups: 0, entitlements: 0, resources: 0 } as SyncStats
  }

  // ── Render helpers ──
  const statusOpts = ["connected", "disconnected", "error", "syncing", "degraded"]
  const typeOpts = Object.entries(CONNECTOR_TYPES).map(([k, v]) => k)

  function onTypeSelect(t: string) {
    setAddType(t)
    setAddForm({ name: "", auth_type: "oauth2" })
  }

  // ── Render ────────────────────────────────────────────
  return (
    <div className="space-y-4">
      {/* ── Header ── */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Directory Connectors</h1>
          <p className="text-sm text-gray-400 mt-1">
            {stats ? `${stats.total_connectors} connectors · ${stats.connected_count} connected · ${stats.total_identities} identities synced` : "Loading..."}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button className="btn-secondary text-xs px-3 py-1.5" onClick={load}>Refresh</button>
          <button className="btn-primary text-xs px-3 py-1.5" onClick={() => setShowAdd(true)}>+ Add Directory</button>
        </div>
      </div>

      {/* ── Stats Bar ── */}
      {stats && (
        <div className="flex gap-3 flex-wrap">
          {[
            ["Connectors", stats.total_connectors, "text-white"],
            ["Connected", stats.connected_count, "text-emerald-400"],
            ["Error", stats.error_count, "text-red-400"],
            ["Syncing", stats.syncing_count, "text-amber-400"],
            ["Identities", stats.total_identities, "text-blue-400"],
            ["Groups", stats.total_groups, "text-purple-400"],
            ["Entitlements", stats.total_entitlements, "text-cyan-400"],
            ["Resources", stats.total_resources, "text-teal-400"],
          ].map(([label, count, color]) => (
            <div key={label as string} className="glass-card px-4 py-2 flex items-center gap-2">
              <span className="text-xs text-gray-500 uppercase tracking-wider">{label}</span>
              <span className={`text-lg font-semibold ${color}`}>{String(count)}</span>
            </div>
          ))}
        </div>
      )}

      {/* ── Search & Filter ── */}
      <div className="glass-card p-3 space-y-3">
        <div className="flex gap-3 items-center">
          <div className="relative flex-1 max-w-md">
            <input className="input text-sm py-1.5 pl-8 w-full" placeholder="Search connectors..." value={searchInput} onChange={e => setSearchInput(e.target.value)} />
            <svg className="absolute left-2.5 top-2 w-3.5 h-3.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg>
            {searchInput && <button className="absolute right-2 top-1.5 text-gray-500 hover:text-gray-300" onClick={() => { setSearchInput(""); setSearch("") }}>&times;</button>}
          </div>
          <div className="flex items-center gap-1"><span className="text-xs text-gray-500 mr-1">Status:</span>
            {statusOpts.map(s => (
              <button key={s} onClick={() => setStatusFilter(statusFilter === s ? "" : s)} className={`px-2 py-0.5 rounded text-xs font-medium transition-all ${statusFilter === s ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50" : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"}`}>{s}</button>
            ))}
          </div>
          <div className="flex items-center gap-1"><span className="text-xs text-gray-500 mr-1">Type:</span>
            {typeOpts.slice(0, 4).map(t => (
              <button key={t} onClick={() => setTypeFilter(typeFilter === t ? "" : t)} className={`px-2 py-0.5 rounded text-xs font-medium transition-all ${typeFilter === t ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50" : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"}`}>{t.replace("_", " ")}</button>
            ))}
          </div>
          {hasFilters && <button className="text-xs text-brand-400 hover:text-brand-300 ml-auto" onClick={() => { setSearch(""); setSearchInput(""); setStatusFilter(""); setTypeFilter(""); }}>Clear All</button>}
        </div>
        {hasFilters && (
          <div className="flex gap-1.5 flex-wrap pt-1 border-t border-gray-800/50">
            {search && <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">"{search}"<button className="hover:text-white ml-0.5" onClick={() => { setSearch(""); setSearchInput("") }}>&times;</button></span>}
            {statusFilter && <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">Status: {statusFilter}<button className="hover:text-white ml-0.5" onClick={() => setStatusFilter("")}>&times;</button></span>}
            {typeFilter && <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/20 text-brand-400">Type: {typeFilter}<button className="hover:text-white ml-0.5" onClick={() => setTypeFilter("")}>&times;</button></span>}
          </div>
        )}
      </div>

      {/* ── Connector Cards ── */}
      <div className="space-y-3">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading connectors...</div>
        ) : error ? (
          <div className="p-12 text-center text-red-400">{error}</div>
        ) : connectors.length === 0 ? (
          <div className="p-12 text-center text-gray-500"><p className="mb-2">{hasFilters ? "No connectors match filters" : "No directories configured"}</p><p className="text-xs text-gray-600">{hasFilters ? "Try clearing filters" : "Click + Add Directory to configure your first connection"}</p></div>
        ) : (
          connectors.map(c => {
            const counts = syncCounts(c)
            const h = (c.health || {}) as HealthReport
            const isTestOk = testMsg[c.id]
            const isBusy = busySync === c.id
            const isExpanded = expanded === c.id
            return (
              <div key={c.id} className="glass-card overflow-hidden">
                {/* Card header */}
                <div className="p-4 cursor-pointer hover:bg-surface-100/30 transition-colors" onClick={() => toggle(c.id)}>
                  <div className="flex items-start justify-between">
                    <div className="flex items-start gap-3 flex-1">
                      {/* Icon */}
                      <div className={`w-10 h-10 rounded-lg flex items-center justify-center text-lg shrink-0 ${
                        c.type === "entra_id" ? "bg-blue-500/15 text-blue-400" :
                        c.type === "ldap" || c.type === "active_directory" ? "bg-purple-500/15 text-purple-400" :
                        c.type === "scim" || c.type === "okta" ? "bg-cyan-500/15 text-cyan-400" :
                        c.type === "csv" ? "bg-emerald-500/15 text-emerald-400" :
                        "bg-gray-500/15 text-gray-400"
                      }`}>
                        {c.type === "entra_id" ? "\u2601" : c.type === "ldap" ? "\u{1F310}" : c.type === "csv" ? "\u{1F4C4}" : "\u{1F517}"}
                      </div>

                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <h3 className="text-base font-semibold text-white">{c.name}</h3>
                          <span className={`px-2 py-0.5 rounded-full text-xs border ${STATUS_COLORS[c.status] || ""}`}>{c.status}</span>
                          <span className="px-2 py-0.5 rounded text-xs bg-gray-800 text-gray-400">{CONNECTOR_TYPES[c.type] || c.type}</span>
                        </div>
                        <div className="flex gap-4 mt-1.5 text-xs text-gray-400">
                          <span>Synced: {safeFormatDate(c.last_sync_at)}</span>
                          {counts.users > 0 && <span>{counts.users} users</span>}
                          {counts.groups > 0 && <span>{counts.groups} groups</span>}
                          {counts.entitlements > 0 && <span>{counts.entitlements} entitlements</span>}
                          {counts.resources > 0 && <span>{counts.resources} resources</span>}
                          {c.last_error && <span className="text-red-400 truncate max-w-[250px]" title={c.last_error}>{c.last_error}</span>}
                        </div>
                      </div>
                    </div>

                    {/* Actions */}
                    <div className="flex items-center gap-1.5 ml-4 shrink-0" onClick={e => e.stopPropagation()}>
                      <button className="btn-secondary text-xs px-2 py-1" onClick={() => handleTestConnector(c.id)} disabled={isBusy}>
                        {isTestOk ? (isTestOk.ok ? "OK" : isTestOk.msg) : "Test"}
                      </button>
                      <button className="btn-primary text-xs px-2 py-1" onClick={() => handleSync(c.id)} disabled={isBusy}>
                        {isBusy ? "..." : "Sync"}
                      </button>
                      {h.delta_supported && (
                        <button className="btn-secondary text-xs px-2 py-1" onClick={() => handleSync(c.id, true)} disabled={isBusy}>Delta</button>
                      )}
                      <button className="btn-primary text-xs px-2 py-1" onClick={() => handleFullSync(c.id)} disabled={isBusy}>Full Sync</button>
                      <button className="btn-danger text-xs px-2 py-1" onClick={() => handleDelete(c.id)} disabled={isBusy}>Del</button>
                      <span className="text-gray-600 ml-1">{isExpanded ? "\u25B2" : "\u25BC"}</span>
                    </div>
                  </div>

                  {/* Health bar */}
                  {h.consecutive_success !== undefined && (
                    <div className="flex gap-3 mt-2 pt-2 border-t border-gray-800/50 text-xs text-gray-500">
                      <span>Success: <span className="text-emerald-400">{h.consecutive_success || 0}</span></span>
                      <span>Errors: <span className={h.consecutive_errors ? "text-red-400" : "text-gray-500"}>{h.consecutive_errors || 0}</span></span>
                      {h.last_sync_duration && <span>Duration: {h.last_sync_duration}</span>}
                      {h.last_error && <span className="text-red-400">Last: {h.last_error}</span>}
                    </div>
                  )}
                </div>

                {/* Expandable detail */}
                {isExpanded && (
                  <div className="border-t border-gray-800">
                    {/* Tabs */}
                    <div className="flex border-b border-gray-800 overflow-x-auto">
                      {(["accounts", "groups", "entitlements", "resources", "schema"] as TabKey[]).map(t => (
                        <button key={t} onClick={() => loadTab(c.id, t)} className={`px-4 py-2 text-xs font-medium whitespace-nowrap border-b-2 transition-colors ${activeTab === t ? "border-brand-500 text-brand-400" : "border-transparent text-gray-400 hover:text-gray-300"}`}>
                          {t.charAt(0).toUpperCase() + t.slice(1)}
                          {t === "accounts" && counts.users > 0 && <span className="ml-1.5 text-gray-600">({counts.users})</span>}
                          {t === "groups" && counts.groups > 0 && <span className="ml-1.5 text-gray-600">({counts.groups})</span>}
                          {t === "entitlements" && counts.entitlements > 0 && <span className="ml-1.5 text-gray-600">({counts.entitlements})</span>}
                          {t === "resources" && counts.resources > 0 && <span className="ml-1.5 text-gray-600">({counts.resources})</span>}
                        </button>
                      ))}
                    </div>

                    {/* Tab content */}
                    <div className="p-4 max-h-[500px] overflow-y-auto">
                      {tabLoading[`${c.id}:${activeTab}`] ? (
                        <div className="text-center text-gray-500 py-8">Loading...</div>
                      ) : !tabData[`${c.id}:${activeTab}`] ? (
                        <div className="text-center text-gray-500 py-8">No data</div>
                      ) : activeTab === "accounts" ? (
                        <AccountsTab data={tabData[`${c.id}:${activeTab}`]} />
                      ) : activeTab === "groups" ? (
                        <GroupsTab data={tabData[`${c.id}:${activeTab}`]} />
                      ) : activeTab === "entitlements" ? (
                        <EntitlementsTab data={tabData[`${c.id}:${activeTab}`]} />
                      ) : activeTab === "resources" ? (
                        <ResourcesTab data={tabData[`${c.id}:${activeTab}`]} />
                      ) : activeTab === "schema" ? (
                        <SchemaTab data={tabData[`${c.id}:${activeTab}`]} />
                      ) : null}
                    </div>
                  </div>
                )}
              </div>
            )
          })
        )}
      </div>

      {/* ── Add Connector Modal ── */}
      {showAdd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setShowAdd(false)}>
          <div className="w-full max-w-2xl glass-card p-6 space-y-4 max-h-[85vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-semibold text-white">Add Directory Connector</h2>
              <button className="text-gray-400 hover:text-white text-xl" onClick={() => setShowAdd(false)}>&times;</button>
            </div>

            {/* Connector type selector */}
            <div className="flex gap-2 flex-wrap">
              {Object.entries(CONNECTOR_TYPES).map(([k, label]) => (
                <button key={k} onClick={() => onTypeSelect(k)} className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-all ${addType === k ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50" : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"}`}>
                  {label}
                </button>
              ))}
            </div>

            {/* Dynamic form fields */}
            <div className="grid grid-cols-2 gap-3">
              <div className="col-span-2">
                <label className="text-xs text-gray-400 block mb-0.5">Connector Name</label>
                <input className="input text-sm py-1.5" placeholder={`My ${CONNECTOR_TYPES[addType] || addType} Directory`} value={addForm.name || ""} onChange={e => setAddForm({ ...addForm, name: e.target.value })} />
              </div>
              {(CONNECTOR_TYPE_FIELDS[addType] || CONNECTOR_TYPE_FIELDS.generic).map(f => (
                <div key={f.key} className={f.key === "endpoint" || f.key === "properties" ? "col-span-2" : ""}>
                  <label className="text-xs text-gray-400 block mb-0.5">{f.label}</label>
                  {f.key === "properties" && addType === "csv" ? (
                    <textarea className="input text-sm py-1.5 h-24 font-mono" placeholder={f.placeholder}
                      value={addForm[f.key] || ""}
                      onChange={e => setAddForm({ ...addForm, [f.key]: e.target.value })} />
                  ) : (
                    <input className="input text-sm py-1.5" type={f.key.includes("password") || f.key.includes("secret") || f.key.includes("token") ? "password" : "text"}
                      placeholder={f.placeholder}
                      value={addForm[f.key] || ""}
                      onChange={e => setAddForm({ ...addForm, [f.key]: e.target.value })} />
                  )}
                </div>
              ))}
            </div>

            {/* Test result */}
            {testResult && (
              <div className={`p-3 rounded text-sm ${testResult.success ? "bg-emerald-500/10 border border-emerald-500/30 text-emerald-400" : "bg-red-500/10 border border-red-500/30 text-red-400"}`}>
                {testResult.success ? "Connection successful" : `Connection failed: ${testResult.error || "Unknown error"}`}
              </div>
            )}

            <div className="flex gap-2 justify-end pt-2">
              <button className="btn-secondary text-xs px-4 py-2" onClick={() => setShowAdd(false)}>Cancel</button>
              <button className="btn-secondary text-xs px-4 py-2" onClick={handleTest}>Test Connection</button>
              <button className="btn-primary text-xs px-4 py-2" onClick={handleCreate}>Add Connector</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Tab Components ───────────────────────────────────────

function AccountsTab({ data }: { data: any }) {
  const identities = data?.identities || []
  const total = data?.total || identities.length
  return (
    <div>
      <div className="text-xs text-gray-500 mb-2">{total} accounts</div>
      {identities.length === 0 ? <div className="text-gray-500 text-sm py-4">No synced accounts</div> : (
        <table className="w-full text-sm">
          <thead><tr className="border-b border-gray-800">
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">User</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Email</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Dept</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Title</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Groups</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Status</th>
            <th className="text-right py-1.5 px-2 text-xs text-gray-500">Synced</th>
          </tr></thead>
          <tbody className="divide-y divide-gray-800/50">
            {identities.map((i: any, idx: number) => (
              <tr key={i.id || idx} className="hover:bg-surface-100/30">
                <td className="py-1.5 px-2 text-gray-200">{i.display_name || i.username || "?"}</td>
                <td className="py-1.5 px-2 text-gray-400 font-mono text-xs">{i.email || "-"}</td>
                <td className="py-1.5 px-2 text-gray-400 text-xs">{i.department || "-"}</td>
                <td className="py-1.5 px-2 text-gray-400 text-xs">{i.title || "-"}</td>
                <td className="py-1.5 px-2 text-gray-400 text-xs">{i.groups?.length || 0}</td>
                <td className="py-1.5 px-2"><span className={`px-1.5 py-0.5 rounded text-xs ${i.enabled !== false ? "text-emerald-400 bg-emerald-500/10" : "text-gray-400 bg-gray-500/10"}`}>{i.enabled !== false ? "Active" : "Disabled"}</span></td>
                <td className="py-1.5 px-2 text-right text-gray-500 text-xs font-mono">{i.last_synced_at ? new Date(i.last_synced_at).toLocaleDateString() : "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {total > identities.length && <div className="text-center text-gray-500 text-xs py-2">Showing {identities.length} of {total}</div>}
    </div>
  )
}

function GroupsTab({ data }: { data: any }) {
  const groups = data?.groups || []
  return (
    <div>
      <div className="text-xs text-gray-500 mb-2">{data?.total || groups.length} groups</div>
      {groups.length === 0 ? <div className="text-gray-500 text-sm py-4">No synced groups</div> : (
        <table className="w-full text-sm">
          <thead><tr className="border-b border-gray-800">
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Name</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Type</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Scope</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Members</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Description</th>
          </tr></thead>
          <tbody className="divide-y divide-gray-800/50">
            {groups.map((g: any, idx: number) => (
              <tr key={g.id || idx} className="hover:bg-surface-100/30">
                <td className="py-1.5 px-2 text-gray-200">{g.name || "?"}</td>
                <td className="py-1.5 px-2 text-gray-400 text-xs">{g.group_type || "-"}</td>
                <td className="py-1.5 px-2 text-gray-400 text-xs">{g.scope || "-"}</td>
                <td className="py-1.5 px-2 text-gray-400 text-xs">{g.member_ids?.length || 0}</td>
                <td className="py-1.5 px-2 text-gray-400 text-xs truncate max-w-[200px]">{g.description || "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function EntitlementsTab({ data }: { data: any }) {
  const entitlements = data?.entitlements || []
  const colorMap: Record<string, string> = {
    directory_role: "text-blue-400 bg-blue-500/10 border-blue-500/30",
    app_role: "text-purple-400 bg-purple-500/10 border-purple-500/30",
    group_membership: "text-emerald-400 bg-emerald-500/10 border-emerald-500/30",
    license: "text-amber-400 bg-amber-500/10 border-amber-500/30",
    oauth2_permission: "text-cyan-400 bg-cyan-500/10 border-cyan-500/30",
  }
  return (
    <div>
      <div className="text-xs text-gray-500 mb-2">{data?.total || entitlements.length} entitlements</div>
      {entitlements.length === 0 ? <div className="text-gray-500 text-sm py-4">No synced entitlements</div> : (
        <table className="w-full text-sm">
          <thead><tr className="border-b border-gray-800">
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Type</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">User</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Source</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Application</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Active</th>
          </tr></thead>
          <tbody className="divide-y divide-gray-800/50">
            {entitlements.map((e: any, idx: number) => (
              <tr key={idx} className="hover:bg-surface-100/30">
                <td className="py-1.5 px-2"><span className={`px-1.5 py-0.5 rounded text-xs border ${colorMap[e.entitlement_type] || "text-gray-400 bg-gray-500/10 border-gray-500/30"}`}>{e.entitlement_type ? e.entitlement_type.replace("_", " ") : "?"}</span></td>
                <td className="py-1.5 px-2 text-gray-400 text-xs font-mono">{e.identity_external_id || "-"}</td>
                <td className="py-1.5 px-2 text-gray-200 text-xs">{e.source_name || "-"}</td>
                <td className="py-1.5 px-2 text-gray-200 text-xs">{e.app_name || "-"}</td>
                <td className="py-1.5 px-2"><span className={`px-1.5 py-0.5 rounded text-xs ${e.is_active ? "text-emerald-400 bg-emerald-500/10" : "text-gray-400 bg-gray-500/10"}`}>{e.is_active ? "Yes" : "No"}</span></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}

function ResourcesTab({ data }: { data: any }) {
  const resources = data?.resources || []
  const grouped: Record<string, any[]> = {}
  for (const r of resources) {
    const t = r.resource_type || "other"
    if (!grouped[t]) grouped[t] = []
    grouped[t].push(r)
  }
  return (
    <div>
      <div className="text-xs text-gray-500 mb-2">{data?.total || resources.length} resources</div>
      {resources.length === 0 ? <div className="text-gray-500 text-sm py-4">No synced resources</div> : (
        <div className="space-y-3">
          {Object.entries(grouped).map(([type, items]) => (
            <div key={type}>
              <div className="text-xs text-gray-500 uppercase tracking-wider mb-1">{type.replace("_", " ")} ({items.length})</div>
              <table className="w-full text-sm">
                <thead><tr className="border-b border-gray-800">
                  <th className="text-left py-1 px-2 text-xs text-gray-500">Name</th>
                  <th className="text-left py-1 px-2 text-xs text-gray-500">Description</th>
                  <th className="text-left py-1 px-2 text-xs text-gray-500">Status</th>
                  <th className="text-left py-1 px-2 text-xs text-gray-500">Owners</th>
                </tr></thead>
                <tbody className="divide-y divide-gray-800/50">
                  {items.map((r: any, idx: number) => (
                    <tr key={r.id || idx} className="hover:bg-surface-100/30">
                      <td className="py-1 px-2 text-gray-200 text-xs">{r.name || "?"}</td>
                      <td className="py-1 px-2 text-gray-400 text-xs truncate max-w-[150px]">{r.description || "-"}</td>
                      <td className="py-1 px-2"><span className={`px-1.5 py-0.5 rounded text-xs ${r.enabled ? "text-emerald-400 bg-emerald-500/10" : "text-gray-400 bg-gray-500/10"}`}>{r.enabled ? "Active" : "Inactive"}</span></td>
                      <td className="py-1 px-2 text-gray-400 text-xs">{r.owner_ids?.length || 0}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function SchemaTab({ data }: { data: any }) {
  const attrs = data?.schema?.attributes || []
  return (
    <div>
      <div className="text-xs text-gray-500 mb-2">{attrs.length} attribute{attrs.length !== 1 ? "s" : ""}</div>
      {attrs.length === 0 ? <div className="text-gray-500 text-sm py-4">No schema discovered</div> : (
        <table className="w-full text-sm">
          <thead><tr className="border-b border-gray-800">
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Attribute</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Type</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Required</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Multi</th>
            <th className="text-left py-1.5 px-2 text-xs text-gray-500">Description</th>
          </tr></thead>
          <tbody className="divide-y divide-gray-800/50">
            {attrs.map((a: any, idx: number) => (
              <tr key={idx} className="hover:bg-surface-100/30">
                <td className="py-1.5 px-2 text-gray-200 font-mono text-xs">{a.name || a.attribute || "?"}</td>
                <td className="py-1.5 px-2 text-gray-400 text-xs font-mono">{a.type || a.data_type || "-"}</td>
                <td className="py-1.5 px-2"><span className={`px-1.5 py-0.5 rounded text-xs ${a.required ? "text-amber-400 bg-amber-500/10" : "text-gray-400 bg-gray-500/10"}`}>{a.required ? "Yes" : "No"}</span></td>
                <td className="py-1.5 px-2"><span className={`px-1.5 py-0.5 rounded text-xs ${a.multi_valued ? "text-cyan-400 bg-cyan-500/10" : "text-gray-400 bg-gray-500/10"}`}>{a.multi_valued ? "Yes" : "No"}</span></td>
                <td className="py-1.5 px-2 text-gray-400 text-xs truncate max-w-[200px]">{a.description || "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
