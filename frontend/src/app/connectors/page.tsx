"use client"

import { useState, useEffect } from "react"
import { fetchConnectors, createConnector, testConnectorConnection, connectConnector, syncConnector, deleteConnector, fetchConnectorUsers } from "@/lib/api"

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: "", type: "entra_id", endpoint: "", client_id: "", client_secret: "", tenant_name: "", auth_type: "oauth2", base_dn: "", username: "", password: "", domain: "" })
  const [testResult, setTestResult] = useState<any>(null)
  const [viewingUsers, setViewingUsers] = useState<string | null>(null)
  const [usersData, setUsersData] = useState<any>(null)
  const [usersLoading, setUsersLoading] = useState(false)
  const [syncResult, setSyncResult] = useState<string | null>(null)

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
    setSyncResult(id)
    try {
      const res = await syncConnector(id)
      loadConnectors()
      // If viewing users for this connector, refresh
      if (viewingUsers === id) handleViewUsers(id)
    } catch (e: any) {
      alert("Sync failed: " + e.message)
    } finally {
      setSyncResult(null)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this connector?")) return
    try {
      await deleteConnector(id)
      if (viewingUsers === id) setViewingUsers(null)
      loadConnectors()
    } catch (e: any) {
      alert("Delete failed: " + e.message)
    }
  }

  async function handleViewUsers(id: string) {
    if (viewingUsers === id) {
      setViewingUsers(null)
      setUsersData(null)
      return
    }
    setViewingUsers(id)
    setUsersLoading(true)
    try {
      const data = await fetchConnectorUsers(id)
      setUsersData(data)
    } catch (e: any) {
      setUsersData({ error: e.message, users: [], total: 0 })
    } finally {
      setUsersLoading(false)
    }
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
                <input className="input" placeholder="Host:port (e.g. ldap.company.com:389)" value={form.endpoint} onChange={(e) => setForm({...form, endpoint: e.target.value})} />
                <input className="input" placeholder="Base DN (e.g. DC=company,DC=com)" value={form.base_dn} onChange={(e) => setForm({...form, base_dn: e.target.value})} />
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
                    <button className="btn-ghost text-xs" onClick={() => handleSync(c.id)} disabled={syncResult === c.id}>
                      {syncResult === c.id ? "Syncing..." : "Sync"}
                    </button>
                    <button className="btn-ghost text-xs" onClick={() => handleViewUsers(c.id)}>
                      {viewingUsers === c.id ? "Hide" : "Accounts"}
                    </button>
                    <button className="btn-ghost text-xs text-rose-400" onClick={() => handleDelete(c.id)}>Delete</button>
                  </div>
                </div>

                {/* Expandable users table */}
                {viewingUsers === c.id && (
                  <div className="border-t border-gray-800/50 bg-surface-100/20">
                    {usersLoading ? (
                      <div className="p-8 text-center text-sm text-gray-500">Loading accounts...</div>
                    ) : usersData?.error ? (
                      <div className="p-8 text-center text-sm text-red-400">
                        Failed to load accounts: {usersData.error}
                      </div>
                    ) : !usersData?.users || usersData.users.length === 0 ? (
                      <div className="p-8 text-center text-sm text-gray-500">
                        <p className="mb-1">No accounts found</p>
                        <p className="text-xs text-gray-600">Click "Sync" to pull users from this connector</p>
                      </div>
                    ) : (
                      <div className="overflow-x-auto">
                        <table className="w-full text-sm">
                          <thead>
                            <tr className="border-b border-gray-800/50">
                              <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase w-8">#</th>
                              <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">User</th>
                              <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">Email</th>
                              <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">Username</th>
                              <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">Department</th>
                              <th className="text-left py-2.5 px-4 text-xs font-medium text-gray-500 uppercase">Status</th>
                            </tr>
                          </thead>
                          <tbody className="divide-y divide-gray-800/30">
                            {usersData.users.map((u: any, i: number) => (
                              <tr key={u.external_id || i} className="hover:bg-surface-100/30">
                                <td className="py-2 px-4 text-xs text-gray-500">{i + 1}</td>
                                <td className="py-2 px-4">
                                  <div className="flex items-center gap-2">
                                    <div className="w-7 h-7 rounded-full bg-brand-600/20 border border-brand-500/30 flex items-center justify-center text-xs font-medium text-brand-400">
                                      {(u.display_name || u.email || u.username || "?").charAt(0).toUpperCase()}
                                    </div>
                                    <span className="text-gray-200 font-medium">{u.display_name || u.username || "Unnamed"}</span>
                                  </div>
                                </td>
                                <td className="py-2 px-4 text-gray-400">{u.email || "-"}</td>
                                <td className="py-2 px-4 text-gray-400 font-mono text-xs">{u.username || "-"}</td>
                                <td className="py-2 px-4 text-gray-400">{u.department || "-"}</td>
                                <td className="py-2 px-4">
                                  <span className={u.enabled ? "badge-success" : "badge-neutral"}>
                                    {u.enabled ? "Enabled" : "Disabled"}
                                  </span>
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                        <div className="px-4 py-2 text-xs text-gray-500 border-t border-gray-800/30">
                          {usersData.total} account{usersData.total !== 1 ? "s" : ""} synced from {c.name}
                        </div>
                      </div>
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
