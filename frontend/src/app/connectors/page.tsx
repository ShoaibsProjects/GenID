"use client"

import { useState, useEffect, useCallback } from "react"
import {
  fetchConnectors, createConnector, testConnectorConnection,
  connectConnector, syncConnector, deleteConnector, fetchConnectorIdentities
} from "@/lib/api"
import { PageHeader } from "@/components/ui/PageHeader"
import { Badge } from "@/components/ui/Badge"
import { Button } from "@/components/ui/Button"
import { Card, CardHeader, CardBody, CardFooter } from "@/components/ui/Card"
import { Input, Select } from "@/components/ui/Input"
import { Modal } from "@/components/ui/Modal"
import { EmptyState } from "@/components/ui/EmptyState"

/* ─── Status mapping ──────────────────────────────────────── */
const statusVariant: Record<string, "success"|"warning"|"danger"|"info"|"neutral"> = {
  connected: "success", disconnected: "neutral", error: "danger",
  syncing: "info", degraded: "warning",
}

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({
    name: "", type: "entra_id", endpoint: "", client_id: "", client_secret: "",
    tenant_name: "", auth_type: "oauth2", base_dn: "", username: "", password: "", domain: ""
  })
  const [testResult, setTestResult] = useState<any>(null)
  const [busySync, setBusySync] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)
  const [acctsData, setAcctsData] = useState<any>(null)
  const [acctsLoad, setAcctsLoad] = useState(false)
  const [schemaData, setSchemaData] = useState<any>(null)
  const [schemaLoad, setSchemaLoad] = useState(false)
  const [healthData, setHealthData] = useState<Record<string, any>>({})

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const d = await fetchConnectors()
      setConnectors(d.connectors || [])

      // Load health for each connector
      const h: Record<string, any> = {}
      for (const c of (d.connectors || [])) {
        try {
          const hr = await fetch(`/api/v1/connectors/${c.id}/health`).then(r => r.json()).catch(() => null)
          if (hr) h[c.id] = hr
        } catch (_) {}
      }
      setHealthData(h)
    } catch (_) {} finally { setLoading(false) }
  }, [])

  useEffect(() => { load() }, [load])

  async function handleCreate() {
    try { await createConnector(form as any); setShowForm(false); setTestResult(null); load() }
    catch (e: any) { alert("Create failed: " + e.message) }
  }

  async function handleTest() {
    try { const r = await testConnectorConnection(form as any); setTestResult(r) }
    catch (e: any) { setTestResult({ success: false, error: e.message }) }
  }

  async function handleConnect(id: string) {
    try { await connectConnector(id); load() }
    catch (e: any) { alert("Connect failed: " + e.message) }
  }

  async function handleSync(id: string, delta?: boolean) {
    setBusySync(id)
    try {
      const url = delta ? `/api/v1/connectors/${id}/sync-delta` : `/api/v1/connectors/${id}/sync`
      await fetch(url, { method: "POST" })
      load()
      if (expanded === id) { loadAccts(id); loadSchema(id) }
    } catch (e: any) { alert("Sync failed: " + e.message) }
    finally { setBusySync(null) }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this connector and all synced identities?")) return
    try { await deleteConnector(id); if (expanded === id) setExpanded(null); load() }
    catch (e: any) { alert("Delete: " + e.message) }
  }

  async function loadAccts(id: string) {
    setAcctsLoad(true)
    try {
      const d = await fetchConnectorIdentities(id)
      setAcctsData(d)
    } catch (_) { setAcctsData({ error: "Failed", identities: [], total: 0 }) }
    finally { setAcctsLoad(false) }
  }

  async function loadSchema(id: string) {
    setSchemaLoad(true)
    try {
      const d = await fetch(`/api/v1/connectors/${id}/schema`).then(r => r.json()).catch(() => null)
      setSchemaData(d?.schema || null)
    } catch (_) { setSchemaData(null) }
    finally { setSchemaLoad(false) }
  }

  async function toggle(id: string) {
    if (expanded === id) { setExpanded(null); setAcctsData(null); setSchemaData(null); return }
    setExpanded(id)
    loadAccts(id)
    loadSchema(id)
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Directories"
        description={`${connectors.length} connector${connectors.length !== 1 ? "s" : ""} configured — connect to identity sources`}
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowForm(!showForm)}>
            {showForm ? "Cancel" : "Add Directory"}
          </Button>
        }
      />

      {/* Stats row */}
      {connectors.length > 0 && (
        <div className="grid grid-cols-5 gap-3">
          <StatBox label="Total" value={connectors.length} />
          <StatBox label="Connected" value={connectors.filter(c => c.status === "connected").length} variant="success" />
          <StatBox label="Error" value={connectors.filter(c => c.status === "error").length} variant="danger" />
          <StatBox label="Delta Ready" value={connectors.filter(c => healthData[c.id]?.delta_supported).length} variant="info" />
          <StatBox label="Synced IDs" value={connectors.reduce((sum, c) => sum + (healthData[c.id]?.total_users_synced || 0), 0)} />
        </div>
      )}

      {/* New connector form */}
      {showForm && (
        <Card>
          <CardHeader><h2 className="text-sm font-bold">New Directory Connection</h2></CardHeader>
          <CardBody>
            <div className="grid grid-cols-2 gap-3">
              <Input label="Name *" placeholder="e.g. Corporate Entra ID" value={form.name} onChange={e => setForm({...form, name: e.target.value})} />
              <Select label="Type" value={form.type} onChange={e => setForm({...form, type: e.target.value})} options={[
                { value: "entra_id", label: "Microsoft Entra ID" },
                { value: "active_directory", label: "Active Directory" },
                { value: "ldap", label: "LDAP" },
                { value: "scim", label: "SCIM 2.0" },
                { value: "okta", label: "Okta (SCIM)" },
              ]} />
              {form.type === "entra_id" && <>
                <Input label="Tenant Name / ID" placeholder="tenant.onmicrosoft.com or GUID" value={form.tenant_name} onChange={e => setForm({...form, tenant_name: e.target.value})} />
                <Input label="Client ID" placeholder="Application (client) ID" value={form.client_id} onChange={e => setForm({...form, client_id: e.target.value})} />
                <Input label="Client Secret" type="password" placeholder="Secret value" value={form.client_secret} onChange={e => setForm({...form, client_secret: e.target.value})} />
              </>}
              {(form.type === "active_directory" || form.type === "ldap") && <>
                <Input label="Host:Port" placeholder="ldap.company.com:389" value={form.endpoint} onChange={e => setForm({...form, endpoint: e.target.value})} />
                <Input label="Base DN" placeholder="DC=company,DC=com" value={form.base_dn} onChange={e => setForm({...form, base_dn: e.target.value})} />
                <Input label="Username" placeholder="CN=admin,CN=Users,DC=..." value={form.username} onChange={e => setForm({...form, username: e.target.value})} />
                <Input label="Password" type="password" value={form.password} onChange={e => setForm({...form, password: e.target.value})} />
              </>}
              {form.type === "scim" && <>
                <Input label="SCIM Endpoint" placeholder="https://api.example.com/scim/v2" value={form.endpoint} onChange={e => setForm({...form, endpoint: e.target.value})} />
                <Input label="Bearer Token" type="password" placeholder="OAuth token" value={form.password} onChange={e => setForm({...form, password: e.target.value})} />
              </>}
            </div>
          </CardBody>
          <CardFooter>
            <div className="flex items-center gap-2 flex-1">
              <Button variant="primary" size="sm" onClick={handleCreate}>Create</Button>
              <Button variant="secondary" size="sm" onClick={handleTest}>Test Connection</Button>
            </div>
            {testResult && (
              <span className={`text-xs font-medium ${testResult.success ? "text-green-400" : "text-red-400"}`}>
                {testResult.success ? "Connection OK" : `Error: ${testResult.error || "Unknown"}`}
              </span>
            )}
          </CardFooter>
        </Card>
      )}

      {/* Connector list */}
      {loading ? (
        <Card><CardBody><div className="p-8 animate-pulse space-y-3">{[1,2,3].map(i => <div key={i} className="h-12 bg-white/[0.03] rounded"/>)}</div></CardBody></Card>
      ) : connectors.length === 0 ? (
        <Card>
          <CardBody>
            <EmptyState
              title="No directories connected"
              description="Connect to Entra ID, Active Directory, LDAP, or any SCIM provider to import identities."
              action={{ label: "Add Directory", onClick: () => setShowForm(true) }}
              icon={<PlugIcon />}
            />
          </CardBody>
        </Card>
      ) : (
        <div className="space-y-3">
          {connectors.map((c) => {
            const h = healthData[c.id]
            const isConnected = c.status === "connected"
            const supportsDelta = h?.delta_supported

            return (
              <Card key={c.id} variant={c.status === "error" ? "error" : "default"}>
                {/* Header row */}
                <div className="flex items-center px-5 py-3 cursor-pointer hover:bg-white/[0.01] transition-colors" onClick={() => toggle(c.id)}>
                  <div className="flex-1 flex items-center gap-4">
                    <div className="w-2 h-2 rounded-full" style={{ background: isConnected ? "var(--green-accent)" : c.status === "error" ? "var(--red-accent)" : "var(--text-muted)" }} />
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-sm">{c.name}</span>
                        <Badge variant={statusVariant[c.status] || "neutral"}>{c.status}</Badge>
                        {supportsDelta && <Badge variant="info">Delta</Badge>}
                      </div>
                      <p className="text-xs text-muted mt-0.5">
                        {c.type?.replace("_", " ")} 
                        {h?.last_sync_at && ` · Synced ${new Date(h.last_sync_at).toLocaleDateString()}`}
                        {h?.total_users_synced ? ` · ${h.total_users_synced} users` : ""}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5" onClick={e => e.stopPropagation()}>
                    {!isConnected && <Button variant="ghost" size="sm" onClick={() => handleConnect(c.id)}>Connect</Button>}
                    <Button variant="ghost" size="sm" onClick={() => handleSync(c.id)} disabled={busySync === c.id}>
                      {busySync === c.id ? "Syncing" : "Full Sync"}
                    </Button>
                    {supportsDelta && (
                      <Button variant="ghost" size="sm" onClick={() => handleSync(c.id, true)} disabled={busySync === c.id}>
                        Delta
                      </Button>
                    )}
                    <Button variant="ghost" size="sm" className="text-red-400" onClick={() => handleDelete(c.id)}>Delete</Button>
                    <svg className={`w-4 h-4 text-muted transition-transform ${expanded === c.id ? "rotate-180" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
                      <path d="M6 9l6 6 6-6"/>
                    </svg>
                  </div>
                </div>

                {/* Expanded detail */}
                {expanded === c.id && (
                  <div className="border-t border-border">
                    {/* Tabs: Accounts | Schema */}
                    <div className="px-5 py-2 border-b border-border flex gap-4">
                      <TabButton active={!!acctsData && !schemaData} onClick={() => { setSchemaData(null); loadAccts(c.id) }}>Accounts</TabButton>
                      <TabButton active={!!schemaData} onClick={() => { setAcctsData(null); loadSchema(c.id) }}>Schema</TabButton>
                    </div>

                    {/* Accounts table */}
                    {acctsLoad ? (
                      <div className="p-8 text-center text-muted text-sm">Loading accounts...</div>
                    ) : acctsData?.error ? (
                      <div className="p-4 text-center text-sm text-red-400">{acctsData.error}</div>
                    ) : acctsData?.identities?.length > 0 ? (
                      <div className="overflow-x-auto">
                        <table className="w-full text-sm">
                          <thead>
                            <tr className="border-b border-border/50">
                              <th className="table-header">User</th>
                              <th className="table-header">Email</th>
                              <th className="table-header">Department</th>
                              <th className="table-header">Title</th>
                              <th className="table-header">Status</th>
                              <th className="table-header text-right">Synced</th>
                            </tr>
                          </thead>
                          <tbody>
                            {acctsData.identities.slice(0, 50).map((u: any) => (
                              <tr key={u.id} className="table-row">
                                <td className="table-cell">
                                  <div className="flex items-center gap-2">
                                    <div className="w-6 h-6 rounded-full bg-accent/10 border border-accent/30 flex items-center justify-center text-xs font-bold text-accent">
                                      {(u.display_name || u.email || "?").charAt(0).toUpperCase()}
                                    </div>
                                    <span className="text-sm font-medium">{u.display_name || u.username || "-"}</span>
                                  </div>
                                </td>
                                <td className="table-cell text-xs text-secondary">{u.email || "-"}</td>
                                <td className="table-cell text-xs text-secondary">{u.department || "-"}</td>
                                <td className="table-cell text-xs text-secondary">{u.title || "-"}</td>
                                <td className="table-cell"><Badge variant={u.enabled ? "success" : "neutral"}>{u.enabled ? "Active" : "Disabled"}</Badge></td>
                                <td className="table-cell text-xs text-muted text-right">{u.last_synced_at ? new Date(u.last_synced_at).toLocaleDateString() : "-"}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                        <div className="px-4 py-2 text-xs text-muted border-t border-border/50">
                          {acctsData.total} accounts · {acctsData.total > 50 ? `Showing 50 of ${acctsData.total}` : "All shown"}
                        </div>
                      </div>
                    ) : acctsData ? (
                      <div className="p-8 text-center text-sm text-secondary">No accounts synced yet. Click "Full Sync" to import.</div>
                    ) : null}

                    {/* Schema table */}
                    {schemaLoad ? (
                      <div className="p-8 text-center text-muted text-sm">Loading schema...</div>
                    ) : schemaData ? (
                      <div className="overflow-x-auto">
                        <table className="w-full text-sm">
                          <thead>
                            <tr className="border-b border-border/50">
                              <th className="table-header">Attribute</th>
                              <th className="table-header">Type</th>
                              <th className="table-header">Required</th>
                              <th className="table-header">Multi</th>
                              <th className="table-header">Description</th>
                            </tr>
                          </thead>
                          <tbody>
                            {schemaData.attributes.map((a: any) => (
                              <tr key={a.name} className="table-row">
                                <td className="table-cell font-mono text-xs text-accent">{a.name}</td>
                                <td className="table-cell text-xs"><Badge variant="info">{a.type}</Badge></td>
                                <td className="table-cell text-xs">{a.required ? <Badge variant="success">Yes</Badge> : <span className="text-muted">No</span>}</td>
                                <td className="table-cell text-xs">{a.multi_valued ? <Badge variant="warning">Multi</Badge> : <span className="text-muted">Single</span>}</td>
                                <td className="table-cell text-xs text-secondary">{a.description}</td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                        <div className="px-4 py-2 text-xs text-muted border-t border-border/50">
                          {schemaData.count} attributes discovered · {schemaData.object_type} schema
                        </div>
                      </div>
                    ) : null}
                  </div>
                )}
              </Card>
            )
          })}
        </div>
      )}
    </div>
  )
}

/* ─── Sub-components ──────────────────────────────────────── */
function StatBox({ label, value, variant }: { label: string; value: string | number; variant?: string }) {
  const colors: Record<string, string> = {
    success: "text-green-400", danger: "text-red-400", info: "text-blue-400",
  }
  return (
    <Card>
      <CardBody className="px-4 py-3">
        <p className="text-[0.6rem] font-bold text-muted uppercase tracking-wider font-mono">{label}</p>
        <p className={`text-xl font-bold font-mono mt-0.5 ${colors[variant || ""] || ""}`}>{value}</p>
      </CardBody>
    </Card>
  )
}

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`text-xs font-semibold pb-2 border-b-2 transition-colors ${
        active ? "border-accent text-primary" : "border-transparent text-muted hover:text-secondary"
      }`}
    >
      {children}
    </button>
  )
}

function PlugIcon() {
  return <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1" className="text-muted">
    <path d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z"/>
  </svg>
}
