"use client"

import { useState, useEffect } from "react"
import { fetchConnectors, createConnector, testConnectorConnection, connectConnector, syncConnector, deleteConnector } from "@/lib/api"

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: "", type: "entra_id", endpoint: "", client_id: "", client_secret: "", tenant_name: "", auth_type: "oauth2", base_dn: "", username: "", password: "", domain: "" })
  const [testResult, setTestResult] = useState<any>(null)

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
      const result = await createConnector(form as any)
      setShowForm(false)
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
    try {
      await syncConnector(id)
      loadConnectors()
    } catch (e: any) {
      alert("Sync failed: " + e.message)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this connector?")) return
    try {
      await deleteConnector(id)
      loadConnectors()
    } catch (e: any) {
      alert("Delete failed: " + e.message)
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
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-800">
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Name</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Type</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Last Sync</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Error</th>
                <th className="text-right py-3 px-4 text-xs font-medium text-gray-500 uppercase">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800/50">
              {connectors.map((c: any) => (
                <tr key={c.id} className="hover:bg-surface-100/50 transition-colors">
                  <td className="py-3 px-4 text-sm font-medium text-white">{c.name}</td>
                  <td className="py-3 px-4 text-sm text-gray-400 capitalize">{c.type?.replace("_", " ")}</td>
                  <td className="py-3 px-4">
                    <span className={c.status === "connected" ? "badge-success" : c.status === "error" ? "badge-danger" : "badge-neutral"}>
                      {c.status}
                    </span>
                  </td>
                  <td className="py-3 px-4 text-sm text-gray-400">{c.last_sync_at ? new Date(c.last_sync_at).toLocaleString() : "Never"}</td>
                  <td className="py-3 px-4 text-sm text-rose-400 max-w-[200px] truncate">{c.last_error || "-"}</td>
                  <td className="py-3 px-4 text-right space-x-1">
                    <button className="btn-ghost text-xs" onClick={() => handleConnect(c.id)}>Connect</button>
                    <button className="btn-ghost text-xs" onClick={() => handleSync(c.id)}>Sync</button>
                    <button className="btn-ghost text-xs text-rose-400" onClick={() => handleDelete(c.id)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
