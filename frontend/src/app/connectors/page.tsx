"use client"

import { useState, useEffect, useCallback } from "react"
import {
  fetchConnectors,
  syncConnector,
  syncHRConnector,
  createConnector,
  getConnector,
  deleteConnector,
  testConnector,
} from "@/lib/api"

// ─── Types ──────────────────────────────────────────────────

interface Connector {
  id: string
  tenant_id: string
  name: string
  type: string
  status: string
  last_sync_at?: string
  last_error?: string
  created_at: string
  updated_at: string
  config?: ConnectorConfig
  schedule_cron?: string
  sync_interval_minutes?: number
  is_hr_source?: boolean
}

interface ConnectorConfig {
  client_id?: string
  client_secret?: string
  tenant_name?: string
  endpoint?: string
  base_dn?: string
  domain?: string
  username?: string
  password?: string
  properties?: Record<string, string>
}

interface ConnectorDetail extends Connector {
  last_sync?: SyncResult
  health?: ConnectorHealth
}

interface SyncResult {
  connector_id: string
  connector_name: string
  status: string
  started_at: string
  completed_at: string
  users_created: number
  users_updated: number
  users_deleted: number
  users_total: number
  groups_created: number
  groups_updated: number
  groups_deleted: number
  groups_total: number
  errors: string[]
}

interface ConnectorHealth {
  connector_id: string
  connector_name: string
  status: string
  last_sync_at: string
  last_error: string
  total_users_synced: number
  total_groups_synced: number
  consecutive_success: number
  consecutive_errors: number
  delta_supported: boolean
  supports_schema: boolean
}

const CONNECTOR_TYPES: Record<string, string> = {
  entra_id: "Microsoft Entra ID",
  ldap: "LDAP",
  active_directory: "Active Directory",
  scim: "SCIM 2.0",
  okta: "Okta",
  aws_iam: "AWS IAM",
  gcp_iam: "GCP IAM",
  csv: "CSV Import",
  generic: "Generic",
}

const STATUS_COLORS: Record<string, string> = {
  connected: "text-emerald-400 bg-emerald-500/10 border-emerald-500/30",
  disconnected: "text-gray-400 bg-gray-500/10 border-gray-500/30",
  error: "text-red-400 bg-red-500/10 border-red-500/30",
  syncing: "text-amber-400 bg-amber-500/10 border-amber-500/30",
  degraded: "text-yellow-400 bg-yellow-500/10 border-yellow-500/30",
}

const TYPE_ICONS: Record<string, string> = {
  entra_id: "☁️",
  active_directory: "🪟",
  ldap: "🌐",
  scim: "🔗",
  okta: "🟦",
  aws_iam: "🟧",
  gcp_iam: "🟨",
  csv: "📄",
  generic: "🔌",
}

// Per-type config field definitions
const CONNECTOR_FIELDS: Record<string, ConfigField[]> = {
  entra_id: [
    { key: "client_id", label: "Client ID (App ID)", type: "text", required: true, placeholder: "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" },
    { key: "client_secret", label: "Client Secret", type: "password", required: true, placeholder: "Enter client secret" },
    { key: "tenant_name", label: "Tenant Name / Domain", type: "text", required: true, placeholder: "contoso.onmicrosoft.com" },
    { key: "schedule_cron", label: "Sync Schedule (Cron)", type: "text", required: false, placeholder: "*/20 * * * *" },
  ],
  ldap: [
    { key: "endpoint", label: "Server URL", type: "text", required: true, placeholder: "ldap://ldap.example.com:389" },
    { key: "base_dn", label: "Base DN", type: "text", required: true, placeholder: "dc=example,dc=com" },
    { key: "username", label: "Bind Username", type: "text", required: true, placeholder: "cn=admin,dc=example,dc=com" },
    { key: "password", label: "Bind Password", type: "password", required: true, placeholder: "Bind password" },
  ],
  active_directory: [
    { key: "endpoint", label: "Domain Controller URL", type: "text", required: true, placeholder: "ldap://dc.example.com:389" },
    { key: "base_dn", label: "Base DN", type: "text", required: true, placeholder: "dc=example,dc=com" },
    { key: "domain", label: "Domain Name", type: "text", required: true, placeholder: "example.com" },
    { key: "username", label: "Bind Username", type: "text", required: true, placeholder: "EXAMPLE\\admin" },
    { key: "password", label: "Bind Password", type: "password", required: true, placeholder: "Bind password" },
  ],
  okta: [
    { key: "endpoint", label: "Okta Domain", type: "text", required: true, placeholder: "your-domain.okta.com" },
    { key: "client_id", label: "API Token", type: "password", required: true, placeholder: "Okta API token" },
  ],
  csv: [
    { key: "endpoint", label: "File Path", type: "text", required: true, placeholder: "/path/to/file.csv" },
  ],
  generic: [
    { key: "endpoint", label: "Endpoint URL", type: "text", required: true, placeholder: "https://api.example.com" },
    { key: "client_id", label: "Client ID", type: "text", required: false, placeholder: "Client ID" },
    { key: "client_secret", label: "Client Secret", type: "password", required: false, placeholder: "Client Secret" },
  ],
}

interface ConfigField {
  key: string
  label: string
  type: "text" | "password" | "number"
  required: boolean
  placeholder: string
}

// ─── Main Page ──────────────────────────────────────────────

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [syncing, setSyncing] = useState<string | null>(null)

  // Add/Edit modal state
  const [showAddModal, setShowAddModal] = useState(false)
  const [editingConnector, setEditingConnector] = useState<Connector | null>(null)
  const [formData, setFormData] = useState<Partial<ConnectorConfig> & { name: string; type: string; is_hr_source: boolean }>({
    name: "", type: "entra_id", is_hr_source: false
  })
  const [formErrors, setFormErrors] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)

  // Detail view state
  const [selectedConnector, setSelectedConnector] = useState<ConnectorDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [detailTab, setDetailTab] = useState<"overview" | "sync" | "config" | "health">("overview")
  const [testLoading, setTestLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await fetchConnectors()
      setConnectors(data.connectors || [])
      setError("")
    } catch (e: any) {
      setError(e.message || "Failed to load connectors")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  async function handleSync(id: string, name: string) {
    setSyncing(id)
    try {
      const res = await syncConnector(id)
      console.log(`sync response for ${name}:`, res)
      load()
      if (selectedConnector?.id === id) {
        await loadDetail(id)
      }
    } catch (e: any) {
      console.error(`sync failed for ${name}:`, e.message)
      alert(`Sync failed: ${e.message}`)
    } finally {
      setSyncing(null)
    }
  }

  async function handleSyncHR(id: string, name: string) {
    setSyncing(id)
    try {
      const res = await syncHRConnector(id)
      const j = res?.jml ?? {}
      const summary = `+${j.identities_created ?? 0} created · ~${j.identities_updated ?? 0} updated · −${j.identities_terminated ?? 0} terminated`
      console.log(`HR sync for ${name}:`, summary, res)
      alert(`HR sync complete for ${name}\n${summary}`)
      load()
      if (selectedConnector?.id === id) {
        await loadDetail(id)
      }
    } catch (e: any) {
      console.error(`HR sync failed for ${name}:`, e.message)
      alert(`HR sync failed: ${e.message}`)
    } finally {
      setSyncing(null)
    }
  }

  async function loadDetail(id: string) {
    setDetailLoading(true)
    try {
      const data = await getConnector(id)
      setSelectedConnector({
        ...data.connector,
        last_sync: data.last_sync,
        health: data.health,
      } as ConnectorDetail)
    } catch (e: any) {
      console.error("Failed to load connector detail:", e)
    } finally {
      setDetailLoading(false)
    }
  }

  async function handleTestConnection(id: string) {
    setTestLoading(true)
    try {
      await testConnector(id)
      alert("Connection test successful!")
    } catch (e: any) {
      alert(`Connection test failed: ${e.message}`)
    } finally {
      setTestLoading(false)
    }
  }

  // Form handlers
  function openAddModal() {
    setEditingConnector(null)
    setFormData({ name: "", type: "entra_id", is_hr_source: false })
    setFormErrors({})
    setShowAddModal(true)
  }

  function openEditModal(connector: Connector) {
    setEditingConnector(connector)
    setFormData({
      name: connector.name,
      type: connector.type,
      is_hr_source: connector.is_hr_source || false,
      client_id: connector.config?.client_id || "",
      client_secret: "",
      tenant_name: connector.config?.tenant_name || "",
      endpoint: connector.config?.endpoint || "",
      base_dn: connector.config?.base_dn || "",
      domain: connector.config?.domain || "",
      username: connector.config?.username || "",
      password: "",
    })
    setFormErrors({})
    setShowAddModal(true)
  }

  function validateForm(): boolean {
    const errors: Record<string, string> = {}
    if (!formData.name.trim()) errors.name = "Name is required"
    if (!formData.type) errors.type = "Type is required"

    const fields = CONNECTOR_FIELDS[formData.type] || []
    for (const field of fields) {
      const value = formData[field.key as keyof typeof formData] as string
      if (field.required && (!value || !value.trim())) {
        errors[field.key] = `${field.label} is required`
      }
    }
    setFormErrors(errors)
    return Object.keys(errors).length === 0
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!validateForm()) return

    setSubmitting(true)
    try {
      const config: Record<string, string> = {}
      const fields = CONNECTOR_FIELDS[formData.type] || []
      for (const field of fields) {
        const value = formData[field.key as keyof typeof formData] as string
        if (value && value.trim()) {
          config[field.key] = value
        }
      }

      const payload = {
        name: formData.name,
        type: formData.type,
        config,
        is_hr_source: formData.is_hr_source,
      }

      if (editingConnector) {
        await deleteConnector(editingConnector.id)
        await createConnector(payload)
      } else {
        await createConnector(payload)
      }

      setShowAddModal(false)
      load()
    } catch (e: any) {
      alert(`Failed to save connector: ${e.message}`)
    } finally {
      setSubmitting(false)
    }
  }

  function handleInputChange(e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) {
    const { name, value, type } = e.target
    setFormData(prev => ({ ...prev, [name]: type === "checkbox" ? (e.target as HTMLInputElement).checked : value }))
    if (formErrors[name]) setFormErrors(prev => ({ ...prev, [name]: "" }))
  }

  const connectedCount = connectors.filter(c => c.status === "connected").length

  // ─── Detail View (Early Return) ──────────────────────────
  if (selectedConnector) {
    return (
      <div className="fixed inset-0 z-50 flex justify-end">
        <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={() => setSelectedConnector(null)} />
        <div className="relative z-10 w-full max-w-3xl h-full overflow-y-auto bg-zinc-950 border-l border-zinc-800" onClick={e => e.stopPropagation()}>
          <div className="flex items-center justify-between p-4 border-b border-zinc-800 sticky top-0 bg-zinc-950/90 backdrop-blur z-10">
            <div className="flex items-center gap-3">
              <span className="w-10 h-10 rounded-lg bg-white/[0.04] border border-zinc-800 flex items-center justify-center text-lg">
                {TYPE_ICONS[selectedConnector.type] || "🔌"}
              </span>
              <div>
                <h2 className="text-lg font-semibold text-white">{selectedConnector.name}</h2>
                <p className="text-xs text-gray-500">{CONNECTOR_TYPES[selectedConnector.type] || selectedConnector.type}</p>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <span className={`px-2 py-0.5 rounded-full text-xs border ${STATUS_COLORS[selectedConnector.status] || STATUS_COLORS.disconnected}`}>
                {selectedConnector.status}
              </span>
              <button className="text-gray-400 hover:text-white text-xl p-1" onClick={() => setSelectedConnector(null)}>&times;</button>
            </div>
          </div>

          {detailLoading ? (
            <div className="p-8 text-center text-gray-500">Loading...</div>
          ) : (
            <div className="p-4 space-y-4">
              {/* Tabs */}
              <div className="flex border-b border-zinc-800">
                {(["overview", "sync", "config", "health"] as const).map(t => (
                  <button
                    key={t}
                    onClick={() => setDetailTab(t)}
                    className={`px-4 py-2 text-xs font-medium border-b-2 transition-colors ${
                      detailTab === t
                        ? "border-amber-500 text-amber-400"
                        : "border-transparent text-gray-400 hover:text-gray-300"
                    }`}
                  >
                    {t.charAt(0).toUpperCase() + t.slice(1)}
                  </button>
                ))}
              </div>

              {/* Overview Tab */}
              {detailTab === "overview" && (
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                      <p className="text-xs text-gray-500">Status</p>
                      <p className="font-semibold text-white capitalize">{selectedConnector.status}</p>
                    </div>
                    <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                      <p className="text-xs text-gray-500">Type</p>
                      <p className="font-semibold text-white">{CONNECTOR_TYPES[selectedConnector.type] || selectedConnector.type}</p>
                    </div>
                    <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                      <p className="text-xs text-gray-500">Last Sync</p>
                      <p className="font-semibold text-white">
                        {selectedConnector.last_sync_at
                          ? new Date(selectedConnector.last_sync_at).toLocaleString()
                          : "Never"}
                      </p>
                    </div>
                    <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                      <p className="text-xs text-gray-500">HR Source</p>
                      <p className="font-semibold text-white">
                        {selectedConnector.is_hr_source ? "Yes" : "No"}
                      </p>
                    </div>
                  </div>

                  {selectedConnector.last_error && (
                    <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/30">
                      <p className="text-xs text-red-400">Last Error</p>
                      <p className="text-sm text-red-300 font-mono">{selectedConnector.last_error}</p>
                    </div>
                  )}

                  {selectedConnector.last_sync && (
                    <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                      <p className="text-xs text-gray-500 mb-2">Last Sync Result</p>
                      <div className="grid grid-cols-4 gap-2 text-center">
                        <div className="p-2 bg-emerald-500/10 rounded"><p className="text-lg font-bold text-emerald-400">{selectedConnector.last_sync.users_created}</p><p className="text-xs text-gray-500">Created</p></div>
                        <div className="p-2 bg-amber-500/10 rounded"><p className="text-lg font-bold text-amber-400">{selectedConnector.last_sync.users_updated}</p><p className="text-xs text-gray-500">Updated</p></div>
                        <div className="p-2 bg-red-500/10 rounded"><p className="text-lg font-bold text-red-400">{selectedConnector.last_sync.users_deleted}</p><p className="text-xs text-gray-500">Deleted</p></div>
                        <div className="p-2 bg-blue-500/10 rounded"><p className="text-lg font-bold text-blue-400">{selectedConnector.last_sync.users_total}</p><p className="text-xs text-gray-500">Total</p></div>
                      </div>
                    </div>
                  )}
                </div>
              )}

              {/* Sync Tab */}
              {detailTab === "sync" && (
                <div className="space-y-4">
                  <div className="flex gap-2">
                    <button
                      className="btn-primary text-xs px-3 py-1.5 flex-1"
                      disabled={syncing === selectedConnector.id}
                      onClick={() => handleSync(selectedConnector.id, selectedConnector.name)}
                    >
                      {syncing === selectedConnector.id ? "Syncing…" : "⟳ Full Sync"}
                    </button>
                    {selectedConnector.is_hr_source && (
                      <button
                        className="btn-secondary text-xs px-3 py-1.5 flex-1"
                        disabled={syncing === selectedConnector.id}
                        onClick={() => handleSyncHR(selectedConnector.id, selectedConnector.name)}
                        title="Diff against identities: create/update/terminate/reinstate"
                      >
                        {syncing === selectedConnector.id ? "Syncing…" : "🧬 Sync HR"}
                      </button>
                    )}
                    <button
                      className="btn-secondary text-xs px-3 py-1.5"
                      disabled={testLoading}
                      onClick={() => handleTestConnection(selectedConnector.id)}
                    >
                      {testLoading ? "Testing…" : "🔍 Test Connection"}
                    </button>
                  </div>

                  <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                    <p className="text-xs text-gray-500 mb-2">Sync Schedule</p>
                    <p className="font-mono text-sm">{selectedConnector.schedule_cron || "*/20 * * * *"}</p>
                    <p className="text-xs text-gray-500 mt-1">(Every 20 minutes by default)</p>
                  </div>
                </div>
              )}

              {/* Config Tab */}
              {detailTab === "config" && (
                <div className="space-y-4">
                  <h3 className="text-sm font-semibold text-white">Configuration</h3>
                  <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800 font-mono text-xs">
                    {JSON.stringify(selectedConnector.config, null, 2)}
                  </div>
                  <button
                    className="btn-secondary text-xs px-3 py-1.5"
                    onClick={() => openEditModal(selectedConnector)}
                  >
                    Edit Configuration
                  </button>
                </div>
              )}

              {/* Health Tab */}
              {detailTab === "health" && (
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                      <p className="text-xs text-gray-500">Health Status</p>
                      <p className="font-semibold text-white capitalize">
                        {selectedConnector.health?.status || "Unknown"}
                      </p>
                    </div>
                    <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                      <p className="text-xs text-gray-500">Delta Sync</p>
                      <p className="font-semibold text-white">
                        {selectedConnector.health?.delta_supported ? "Supported" : "Not Supported"}
                      </p>
                    </div>
                    <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                      <p className="text-xs text-gray-500">Total Users Synced</p>
                      <p className="font-semibold text-white">{selectedConnector.health?.total_users_synced || 0}</p>
                    </div>
                    <div className="p-3 rounded-lg bg-zinc-900/50 border border-zinc-800">
                      <p className="text-xs text-gray-500">Consecutive Success</p>
                      <p className="font-semibold text-white">{selectedConnector.health?.consecutive_success || 0}</p>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    )
}

// ─── Main Connector List View ────────────────────────────
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Directory Connectors</h1>
          <p className="text-sm text-gray-400 mt-1">
            {connectors.length} configured · {connectedCount} connected
          </p>
        </div>
        <div className="flex gap-2">
          <button className="btn-secondary text-xs px-3 py-1.5" onClick={load}>Refresh</button>
          <button className="btn-primary text-xs px-3 py-1.5" onClick={openAddModal}>+ Add Connector</button>
        </div>
      </div>

      {/* Add/Edit Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setShowAddModal(false)}>
          <div className="w-full max-w-md bg-zinc-950 border border-zinc-800 rounded-xl p-6" onClick={e => e.stopPropagation()}>
            <div className="flex justify-between items-center mb-4">
              <h2 className="text-lg font-semibold text-white">
                {editingConnector ? "Edit Connector" : "Add Connector"}
              </h2>
              <button className="text-gray-400 hover:text-white text-xl" onClick={() => setShowAddModal(false)}>&times;</button>
            </div>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="text-xs text-gray-400 block mb-1">Name *</label>
                <input
                  className={`input text-sm py-1.5 w-full ${formErrors.name ? "border-red-500" : ""}`}
                  name="name"
                  value={formData.name}
                  onChange={handleInputChange}
                  placeholder="e.g. Azure Production"
                />
                {formErrors.name && <p className="text-xs text-red-400 mt-1">{formErrors.name}</p>}
              </div>

              <div>
                <label className="text-xs text-gray-400 block mb-1">Type *</label>
                <select
                  className="input text-sm py-1.5 w-full"
                  name="type"
                  value={formData.type}
                  onChange={handleInputChange}
                >
                  {Object.entries(CONNECTOR_TYPES).map(([key, label]) => (
                    <option key={key} value={key}>{label}</option>
                  ))}
                </select>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <label className="flex items-center gap-2 text-sm text-gray-400 cursor-pointer">
                  <input
                    type="checkbox"
                    name="is_hr_source"
                    checked={formData.is_hr_source}
                    onChange={handleInputChange}
                  />
                  HR Source (Authoritative)
                </label>
                <label className="flex items-center gap-2 text-sm text-gray-400 cursor-pointer">
                  <input type="checkbox" disabled className="opacity-50" />
                  Vault Secured (Coming Soon)
                </label>
              </div>

              <div className="border-t border-zinc-800 pt-4">
                <h3 className="text-sm font-semibold text-white mb-3">
                  {CONNECTOR_TYPES[formData.type] || formData.type} Configuration
                </h3>
                <div className="space-y-3">
                  {(CONNECTOR_FIELDS[formData.type] || []).map(field => (
                    <div key={field.key}>
                      <label className="text-xs text-gray-400 block mb-0.5">
                        {field.label} {field.required && <span className="text-red-400">*</span>}
                      </label>
                      <input
                        className={`input text-sm py-1.5 w-full ${formErrors[field.key] ? "border-red-500" : ""}`}
                        type={field.type}
                        name={field.key}
                        placeholder={field.placeholder}
                        value={(formData as any)[field.key] || ""}
                        onChange={handleInputChange}
                        required={field.required}
                      />
                      {formErrors[field.key] && <p className="text-xs text-red-400 mt-1">{formErrors[field.key]}</p>}
                    </div>
                  ))}
                </div>
              </div>

              <div className="flex gap-2 justify-end pt-4 border-t border-zinc-800">
                <button type="button" className="btn-secondary text-xs px-4 py-2" onClick={() => setShowAddModal(false)}>Cancel</button>
                <button type="submit" className="btn-primary text-xs px-4 py-2" disabled={submitting}>
                  {submitting ? "Saving…" : (editingConnector ? "Update" : "Create")}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Connector Cards */}
      {loading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {[1, 2, 3].map(i => <div key={i} className="skeleton h-40 rounded-xl" />)}
        </div>
      ) : error ? (
        <div className="glass-card p-12 text-center text-red-400">{error}</div>
      ) : connectors.length === 0 ? (
        <div className="glass-card p-12 text-center text-gray-500">
          <p className="mb-2">No directories configured</p>
          <p className="text-xs text-gray-600">Connect Entra ID, LDAP, SCIM, or import a CSV to start syncing identities</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {connectors.map((c) => (
            <div
              key={c.id}
              className="rounded-xl border border-zinc-800 bg-zinc-900/50 backdrop-blur-sm p-5 transition-all duration-300 hover:border-amber-500/30 hover:shadow-[0_0_24px_rgba(245,158,11,0.06)] cursor-pointer"
              onClick={() => loadDetail(c.id)}
            >
              {/* Card header */}
              <div className="flex items-start justify-between mb-4">
                <div className="flex items-center gap-3">
                  <span className="w-10 h-10 rounded-lg bg-white/[0.04] border border-zinc-800 flex items-center justify-center text-lg">
                    {TYPE_ICONS[c.type] || "🔌"}
                  </span>
                  <div>
                    <h3 className="text-sm font-semibold text-white leading-tight">{c.name}</h3>
                    <p className="text-xs text-gray-500 mt-0.5">{CONNECTOR_TYPES[c.type] || c.type}</p>
                  </div>
                </div>
                <span className={`px-2 py-0.5 rounded-full text-xs border ${STATUS_COLORS[c.status] || STATUS_COLORS.disconnected}`}>
                  {c.status}
                </span>
              </div>

              {/* Meta */}
              <div className="flex items-center justify-between gap-4 text-xs text-gray-500 mb-4">
                <span>Last sync: {c.last_sync_at ? new Date(c.last_sync_at).toLocaleDateString() : "Never"}</span>
                {c.is_hr_source && (
                  <span className="px-1.5 py-0.5 rounded text-xs bg-amber-500/10 text-amber-400 border border-amber-500/30">HR Source</span>
                )}
              </div>
              {c.last_error && (
                <div className="p-2 rounded bg-red-500/10 border border-red-500/20 mb-3">
                  <p className="text-xs text-red-400 truncate" title={c.last_error}>{c.last_error}</p>
                </div>
              )}

              {/* Actions */}
              <div className="flex gap-2">
                <button
                  className="btn-primary text-xs px-3 py-1.5 flex-1"
                  disabled={syncing === c.id}
                  onClick={(e) => { e.stopPropagation(); handleSync(c.id, c.name) }}
                >
                  {syncing === c.id ? "Syncing…" : "⟳ Sync"}
                </button>
                {c.is_hr_source && (
                  <button
                    className="btn-secondary text-xs px-3 py-1.5 flex-1"
                    disabled={syncing === c.id}
                    onClick={(e) => { e.stopPropagation(); handleSyncHR(c.id, c.name) }}
                    title="Diff against identities: create/update/terminate/reinstate"
                  >
                    🧬 HR Sync
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}