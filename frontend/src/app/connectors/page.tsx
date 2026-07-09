"use client"

import { useState, useEffect } from "react"
import { fetchConnectors, createConnector, testConnectorConnection, connectConnector, syncConnector, deleteConnector, fetchConnectorIdentities } from "@/lib/api"

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: "", type: "entra_id", endpoint: "", client_id: "", client_secret: "", tenant_name: "", auth_type: "oauth2", base_dn: "", username: "", password: "", domain: "" })
  const [testResult, setTestResult] = useState<any>(null)
  const [viewingAccts, setViewingAccts] = useState<string | null>(null)
  const [acctsData, setAcctsData] = useState<any>(null)
  const [acctsLoading, setAcctsLoading] = useState(false)
  const [syncBusy, setSyncBusy] = useState<string | null>(null)

  function loadConnectors() {
    setLoading(true)
    fetchConnectors()
      .then((d) => setConnectors(d.connectors || []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { loadConnectors() }, [])

  async function handleCreate() {
    try {
      await createConnector(form as any)
      setShowForm(false)
      setTestResult(null)
      loadConnectors()
    } catch (e: any) {
      alert("Create failed: " + e.message)
    }
  }

  async function handleTest() {
    try {
      const result = await testConnectorConnection(form as any)
      setTestResult(result)
    } catch (e: any) {
      setTestResult({ success: false, error: e.message })
    }
  }

  async function handleConnect(id: string) {
    try {
      await connectConnector(id)
      loadConnectors()
    } catch (e: any) {
      alert("Connect failed: " + e.message)
    }
  }

  async function handleSync(id: string) {
    setSyncBusy(id)
    try {
      const res = await syncConnector(id)
      loadConnectors()
      if (viewingAccts === id) loadAccts(id)
    } catch (e: any) {
      alert("Sync failed: " + e.message)
    } finally {
      setSyncBusy(null)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this connector and all its synced identities?")) return
    try {
      await deleteConnector(id)
      if (viewingAccts === id) { setViewingAccts(null); setAcctsData(null) }
      loadConnectors()
    } catch (e: any) {
      alert("Delete failed: " + e.message)
    }
  }

  async function loadAccts(id: string) {
    setAcctsLoading(true)
    try {
      const data = await fetchConnectorIdentities(id)
      setAcctsData(data)
    } catch (e: any) {
      setAcctsData({ error: e.message, identities: [], total: 0 })
    } finally {
      setAcctsLoading(false)
    }
  }

  async function toggleAccts(id: string) {
    if (viewingAccts === id) {
      setViewingAccts(null)
      setAcctsData(null)
      return
    }
    setViewingAccts(id)
    await loadAccts(id)
  }

  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Connectors</h1>
          <p className="text-sm text-gray-400 mt-1">Connect to external identity providers and directories</p>
        </div>
        <button className="btn-primary text-sm" onClick={() => setShowForm(!showForm)}>
          {showForm ? "Cancel" : "Add Connector"}
        </button>
      </div>

      {showForm && (
        <div className="glass-card p-6">
          <h2 className="text-sm font-semibold text-gray-200 mb-4">New Connector</h2>
          <div className="grid grid-cols-2 gap-4">
            <input className="input" placeholder="Name" value={form.name} onChange={(e) => setForm({...form, name: e.target.value})} />
            <select className="input" value={form.type} onChange={(e) => setForm({...form, type: e.target.value})}>
              <option value="entra_id">Microsoft Entra ID</option>
              <option value="active_directory">Active Directory</option>
              <option value="ldap">LDAP</option>
              <option value="scim">SCIM 2.0</option>
              <option value="okta">Okta (SCIM)</option>
            </select>
            {form.type === "entra_id" && (
              <>
                <input className="input" placeholder="Tenant Name (e.g. mytenant.onmicrosoft.com)" value={form.tenant_name} onChange={(e) => setForm({...form, tenant_name: e.target.value})} />
                <input className="input" placeholder="Client ID" value={form.client_id} onChange={(e) => setForm({...form, client_id: e.target.value})} />
                <input className="input" type="password" placeholder="Client Secret" value={form.client_secret} onChange={(e) => setForm({...form, client_secret: e.target.value})} />
              </>
            )}
            {(form.type === "active_directory" || form.type === "ldap") && (
              <>
                <input className="input" placeholder="Host:port" value={form.endpoint} onChange={(e) => setForm({...form, endpoint: e.target.value})} />
                <input className="input" placeholder="Base DN" value={form.base_dn} onChange={(e) => setForm({...form, base_dn: e.target.value})} />
                <input className="input" placeholder="Username" value={form.username} onChange={(e) => setForm({...form, username: e.target.value})} />
                <input className="input" type="password" placeholder="Password" value={form.password} onChange={(e) => setForm({...form, password: e.target.value})} />
                <input className="input" placeholder="Domain (optional)" value={form.domain} onChange={(e) => setForm({...form, domain: e.target.value})} />
              </>
            )}
            {form.type === "scim" && (
              <>
                <input className="input" placeholder="SCIM Endpoint URL" value={form.endpoint} onChange={(e) => setForm({...form, endpoint: e.target.value})} />
                <select className="input" value={form.auth_type} onChange={(e) => setForm({...form, auth_type: e.target.value})}>
                  <option value="oauth2">OAuth 2.0</option>
                  <option value="basic">Basic Auth</option>
                  <option value="bearer">Bearer Token</option>
                  <option value="api_key">API Key</option>
                </select>
                {form.auth_type === "oauth2" && (
                  <>
                    <input className="input" placeholder="Client ID" value={form.client_id} onChange={(e) => setForm({...form, client_id: e.target.value})} />
                    <input className="input" type="password" placeholder="Client Secret" value={form.client_secret} onChange={(e) => setForm({...form, client_secret: e.target.value})} />
                  </>
                )}
                {(form.auth_type === "basic" || form.auth_type === "bearer") && (
                  <>
                    <input className="input" placeholder="Username / Token" value={form.username} onChange={(e) => setForm({...form, username: e.target.value})} />
                    <input className="input" type="password" placeholder="Password" value={form.password} onChange={(e) => setForm({...form, password: e.target.value})} />
                  </>
                )}
              </>
            )}
          </div>
          <div className="flex gap-3 mt-6">
            <button className="btn-primary text-sm" onClick={handleCreate}>Create Connector</button>
            <button className="btn-secondary text-sm" onClick={handleTest}>Test Connection</button>
          </div>
          {testResult && (
            <div className={`mt-4 p-3 rounded-lg text-sm ${testResult.success ? "bg-emerald-500/10 text-emerald-400" : "bg-rose-500/10 text-rose-400"}`}>
              {testResult.success ? "Connection successful!" : "Error: " + (testResult.error || "Unknown error")}
            </div>
          )}
        </div>
      )}

      <div className="glass-card overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading connectors...</div>
        ) : connectors.length === 0 ? (
          <div className="p-12 text-center text-gray-500">
            <p className="mb-2">No connectors configured</p>
            <p className="text-xs text-gray-600">Click "Add Connector" to connect to Entra ID, Active Directory, LDAP, or any SCIM provider</p>
          </div>
        ) : (
          <div className="divide-y divide-gray-800/50">
            {connectors.map((c: any) => (
              <div key={c.id}>
                <div className="flex items-center px-4 py-3 hover:bg-surface-100/30 transition-colors">
                  <div className="flex-1 grid grid-cols-5 gap-4 items-center text-sm">
                    <span className="font-medium text-white">{c.name}</span>
                    <span className="text-gray-400 capitalize">{c.type?.replace("_", " ")}</span>
                    <span>
                      <span className={c.status === "connected" ? "badge-success" : c.status === "error" ? "badge-danger" : "badge-neutral"}>
                        {c.status}
                      </span>
                    </span>
                    <span className="text-gray-400 text-xs">{c.last_sync_at ? new Date(c.last_sync_at).toLocaleString() : "Never"}</span>
                    <span className="text-rose-400 text-xs truncate">{c.last_error || "-"}</span>
                  </div>
                  <div className="flex gap-1.5 ml-4 shrink-0">
                    <button className="btn-ghost text-xs" onClick={() => handleConnect(c.id)}>Connect</button>
                    <button className="btn-ghost text-xs" onClick={() => handleSync(c.id)} disabled={syncBusy === c.id}>
                      {syncBusy === c.id ? "Syncing..." : "Sync"}
                    </button>
                    <button className="btn-ghost text-xs" onClick={() => toggleAccts(c.id)}>
                      {viewingAccts === c.id ? "Hide" : "Accounts"}
                    </button>
                    <button className="btn-ghost text-xs text-rose-400" onClick={() => handleDelete(c.id)}>Delete</button>
                  </div>
                </div>

                {/* Accounts table (persisted identities) */}
                {viewingAccts === c.id && (
                  <div className="border-t border-gray-800/50 bg-surface-100/20">
                    {acctsLoading ? (
                      <div className="p-8 text-center text-sm text-gray-500">Loading accounts...</div>
                    ) : acctsData?.error ? (
                      <div className="p-8 text-center text-sm text-red-400">
                        Failed: {acctsData.error}
                      </div>
                    ) : !acctsData?.identities || acctsData.identities.length === 0 ? (
                      <div className="p-8 text-center text-sm text-gray-500">
                        <p className="mb-1">No accounts synced yet</p>
                        <p className="text-xs text-gray-600">Click "Connect" then "Sync" to pull users from this directory</p>
                      </div>
                    ) : (
                      <>
                        <div className="overflow-x-auto">
                          <table className="w-full text-sm">
                            <thead>
                              <tr className="border-b border-gray-800/50">
                                <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">User</th>
                                <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">Email</th>
                                <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">Department</th>
                                <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">Title</th>
                                <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">Status</th>
                                <th className="text-right py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">Synced</th>
                              </tr>
                            </thead>
                            <tbody className="divide-y divide-gray-800/30">
                              {acctsData.identities.map((u: any) => (
                                <tr key={u.id} className="hover:bg-surface-100/30">
                                  <td className="py-2 px-4">
                                    <div className="flex items-center gap-2">
                                      <div className="w-7 h-7 rounded-full bg-brand-600/20 border border-brand-500/30 flex items-center justify-center text-xs font-medium text-brand-400">
                                        {(u.display_name || u.email || u.username || "?").charAt(0).toUpperCase()}
                                      </div>
                                      <div>
                                        <span className="text-gray-200 font-medium text-sm">{u.display_name || u.username || "Unnamed"}</span>
                                        {u.external_id && <span className="text-gray-600 text-xs ml-1 truncate max-w-[120px] inline-block align-middle">{u.external_id}</span>}
                                      </div>
                                    </div>
                                  </td>
                                  <td className="py-2 px-4 text-gray-400 text-xs">{u.email || "-"}</td>
                                  <td className="py-2 px-4 text-gray-400 text-xs">{u.department || "-"}</td>
                                  <td className="py-2 px-4 text-gray-400 text-xs">{u.title || "-"}</td>
                                  <td className="py-2 px-4">
                                    <span className={u.enabled ? "badge-success" : "badge-neutral"}>
                                      {u.enabled ? "Active" : "Disabled"}
                                    </span>
                                  </td>
                                  <td className="py-2 px-4 text-right text-gray-500 text-xs">
                                    {u.last_synced_at ? new Date(u.last_synced_at).toLocaleDateString() : "-"}
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                        <div className="px-4 py-2 text-xs text-gray-500 border-t border-gray-800/30 flex items-center justify-between">
                          <span>{acctsData.total} account{acctsData.total !== 1 ? "s" : ""} synced from {c.name}</span>
                          <button className="btn-ghost text-xs" onClick={() => handleSync(c.id)} disabled={syncBusy === c.id}>
                            {syncBusy === c.id ? "Syncing..." : "Re-sync"}
                          </button>
                        </div>
                      </>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
