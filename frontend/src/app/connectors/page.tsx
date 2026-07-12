"use client"

import { useState, useEffect, useCallback } from "react"
import {
  fetchConnectors, createConnector, testConnectorConnection,
  connectConnector, syncConnector, deleteConnector, fetchConnectorIdentities,
  fetchConnectorGroups, fetchConnectorEntitlements, fetchConnectorResources,
  fullSyncConnector
} from "@/lib/api"
import { PageHeader } from "@/components/ui/PageHeader"
import { Badge } from "@/components/ui/Badge"
import { Button } from "@/components/ui/Button"
import { Card, CardHeader, CardBody, CardFooter } from "@/components/ui/Card"
import { Input, Select } from "@/components/ui/Input"
import { Modal } from "@/components/ui/Modal"
import { EmptyState } from "@/components/ui/EmptyState"

const statusVariant: Record<string, "success"|"warning"|"danger"|"info"|"neutral"> = {
  connected: "success", disconnected: "neutral", error: "danger",
  syncing: "info", degraded: "warning",
}

type TabKey = "accounts" | "groups" | "entitlements" | "resources" | "schema"

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
  const [activeTab, setActiveTab] = useState<TabKey>("accounts")
  const [tabData, setTabData] = useState<Record<string, any>>({})
  const [tabLoad, setTabLoad] = useState<Record<string, boolean>>({})
  const [healthData, setHealthData] = useState<Record<string, any>>({})

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const d = await fetchConnectors()
      setConnectors(d.connectors || [])
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
      if (expanded === id) loadTab(id, activeTab)
    } catch (e: any) { alert("Sync failed: " + e.message) }
    finally { setBusySync(null) }
  }

  const [testMsg, setTestMsg] = useState<Record<string, {ok: boolean; msg: string}>>({})

  async function handleTestConnector(id: string) {
    const prev = testMsg[id]
    setTestMsg(t => ({...t, [id]: {ok: false, msg: "Testing..."}}))
    try {
      const r = await fetch(`/api/v1/connectors/${id}/test`, { method: "POST" }).then(r => r.json())
      setTestMsg(t => ({...t, [id]: {ok: r.success, msg: r.success ? "OK" : (r.error || "Failed")}}))
      setTimeout(() => setTestMsg(t => { const n = {...t}; delete n[id]; return n }), 3000)
    } catch (e: any) {
      setTestMsg(t => ({...t, [id]: {ok: false, msg: e.message}}))
      setTimeout(() => setTestMsg(t => { const n = {...t}; delete n[id]; return n }), 3000)
    }
  }

  async function handleFullSync(id: string) {
    setBusySync(id)
    try {
      await fullSyncConnector(id)
      await load()
      if (expanded === id) {
        setTabData({})
        loadTab(id, activeTab)
      }
    } catch (e: any) { alert("Full sync failed: " + e.message) }
    finally { setBusySync(null) }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this connector and all synced data?")) return
    try { await deleteConnector(id); if (expanded === id) setExpanded(null); load() }
    catch (e: any) { alert("Delete: " + e.message) }
  }

  async function loadTab(id: string, tab: TabKey) {
    setActiveTab(tab)
    const key = `${id}:${tab}`
    setTabLoad(t => ({...t, [key]: true}))
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
      if (expanded !== id) return // stale — user switched connector
      setTabData(t => ({...t, [key]: data}))
    } catch (_) {
      if (expanded !== id) return
      setTabData(t => ({...t, [key]: { error: "Failed to load" }}))
    } finally {
      if (expanded === id) setTabLoad(t => ({...t, [key]: false}))
    }
  }

  async function toggle(id: string) {
    if (expanded === id) { setExpanded(null); setActiveTab("accounts"); setTabData({}); return }
    setExpanded(id)
    setActiveTab("accounts")
    setTabData({})
    loadTab(id, "accounts")
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
        <div className="grid grid-cols-6 gap-3">
          <StatBox label="Total" value={connectors.length} />
          <StatBox label="Connected" value={connectors.filter(c => c.status === "connected").length} variant="success" />
          <StatBox label="Error" value={connectors.filter(c => c.status === "error").length} variant="danger" />
          <StatBox label="Delta Ready" value={connectors.filter(c => healthData[c.id]?.delta_supported).length} variant="info" />
          <StatBox label="Synced IDs" value={connectors.reduce((sum, c) => sum + (healthData[c.id]?.total_users_synced || 0), 0)} />
          <StatBox label="Groups" value={connectors.reduce((sum, c) => sum + (healthData[c.id]?.total_groups_synced || 0), 0)} />
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
              description="Connect to Entra ID, Active Directory, LDAP, or any SCIM provider to import identities, groups, entitlements, and resources."
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
                    <Button variant="ghost" size="sm" onClick={() => handleTestConnector(c.id)}>Test</Button>
                    {testMsg[c.id] && (
                      <span className={`text-[10px] font-mono ${testMsg[c.id].ok ? "text-green-400" : "text-red-400"}`}>
                        {testMsg[c.id].msg}
                      </span>
                    )}
                    <Button variant="ghost" size="sm" onClick={() => handleFullSync(c.id)} disabled={busySync === c.id}>
                      {busySync === c.id ? "Syncing..." : "Full Sync"}
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
                    {/* Tabs */}
                    <div className="px-5 py-2 border-b border-border flex gap-4 overflow-x-auto">
                      <TabButton active={activeTab === "accounts"} onClick={() => loadTab(c.id, "accounts")}>Accounts</TabButton>
                      <TabButton active={activeTab === "groups"} onClick={() => loadTab(c.id, "groups")}>Groups</TabButton>
                      <TabButton active={activeTab === "entitlements"} onClick={() => loadTab(c.id, "entitlements")}>Entitlements</TabButton>
                      <TabButton active={activeTab === "resources"} onClick={() => loadTab(c.id, "resources")}>Resources</TabButton>
                      <TabButton active={activeTab === "schema"} onClick={() => loadTab(c.id, "schema")}>Schema</TabButton>
                    </div>

                    {activeTab === "accounts" && renderAccountsTab(tabData[`${c.id}:accounts`], tabLoad[`${c.id}:accounts`])}

                    {activeTab === "groups" && renderGroupsTab(tabData[`${c.id}:groups`], tabLoad[`${c.id}:groups`], c.id)}

                    {activeTab === "entitlements" && renderEntitlementsTab(tabData[`${c.id}:entitlements`], tabLoad[`${c.id}:entitlements`], c.id)}

                    {activeTab === "resources" && renderResourcesTab(tabData[`${c.id}:resources`], tabLoad[`${c.id}:resources`], c.id)}

                    {activeTab === "schema" && renderSchemaTab(tabData[`${c.id}:schema`], tabLoad[`${c.id}:schema`])}
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

/* ─── Tab renderers ───────────────────────────────────────── */

function renderAccountsTab(data: any, load: boolean) {
  if (load) return <div className="p-8 text-center text-muted text-sm">Loading accounts...</div>
  if (data?.error) return <div className="p-4 text-center text-sm text-red-400">{data.error}</div>
  if (data?.identities?.length > 0) {
    return (
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border/50">
              <th className="table-header">User</th>
              <th className="table-header">Email</th>
              <th className="table-header">Department</th>
              <th className="table-header">Title</th>
              <th className="table-header">Groups</th>
              <th className="table-header">Status</th>
              <th className="table-header text-right">Synced</th>
            </tr>
          </thead>
          <tbody>
            {data.identities.slice(0, 100).map((u: any) => (
              <tr key={u.id} className="table-row">
                <td className="table-cell">
                  <div className="flex items-center gap-2">
                    <div className="w-6 h-6 rounded-full bg-accent/10 border border-accent/30 flex items-center justify-center text-xs font-bold text-accent">
                      {(u.display_name || u.email || "?").charAt(0).toUpperCase()}
                    </div>
                    <div>
                      <span className="text-sm font-medium">{u.display_name || u.username || "-"}</span>
                      {u.manager_id && <p className="text-[10px] text-muted">Manager ID: {u.manager_id.slice(0, 8)}...</p>}
                    </div>
                  </div>
                </td>
                <td className="table-cell text-xs text-secondary">{u.email || "-"}</td>
                <td className="table-cell text-xs text-secondary">{u.department || "-"}</td>
                <td className="table-cell text-xs text-secondary">{u.title || "-"}</td>
                <td className="table-cell text-xs text-secondary">{u.groups?.length || 0} groups</td>
                <td className="table-cell"><Badge variant={u.enabled ? "success" : "neutral"}>{u.enabled ? "Active" : "Disabled"}</Badge></td>
                <td className="table-cell text-xs text-muted text-right">{u.last_synced_at ? new Date(u.last_synced_at).toLocaleDateString() : "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="px-4 py-2 text-xs text-muted border-t border-border/50">
          {data.total} accounts · {data.total > 100 ? `Showing 100 of ${data.total}` : "All shown"}
        </div>
      </div>
    )
  }
  return <div className="p-8 text-center text-sm text-secondary">No accounts synced yet. Click Full Sync to import.</div>
}

function renderGroupsTab(data: any, load: boolean, _connectorId: string) {
  if (load) return <div className="p-8 text-center text-muted text-sm">Loading groups...</div>
  if (data?.error) return <div className="p-4 text-center text-sm text-red-400">{data.error}</div>
  if (data?.groups?.length > 0) {
    return (
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border/50">
              <th className="table-header">Name</th>
              <th className="table-header">Type</th>
              <th className="table-header">Scope</th>
              <th className="table-header">Members</th>
              <th className="table-header">Description</th>
              <th className="table-header text-right">Synced</th>
            </tr>
          </thead>
          <tbody>
            {data.groups.slice(0, 100).map((g: any) => (
              <tr key={g.id} className="table-row">
                <td className="table-cell font-medium text-sm">{g.name || g.external_id?.slice(0, 20)}</td>
                <td className="table-cell"><Badge variant="info">{g.group_type || "group"}</Badge></td>
                <td className="table-cell text-xs text-secondary">{g.scope || "-"}</td>
                <td className="table-cell text-xs">{g.member_ids?.length || 0} members</td>
                <td className="table-cell text-xs text-muted max-w-[200px] truncate">{g.description || "-"}</td>
                <td className="table-cell text-xs text-muted text-right">{g.last_synced_at ? new Date(g.last_synced_at).toLocaleDateString() : "-"}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="px-4 py-2 text-xs text-muted border-t border-border/50">
          {data.total} groups · {data.total > 100 ? `Showing 100 of ${data.total}` : "All shown"}
        </div>
      </div>
    )
  }
  return <div className="p-8 text-center text-sm text-secondary">No groups synced yet. Full Sync will import groups.</div>
}

function renderEntitlementsTab(data: any, load: boolean, _connectorId: string) {
  if (load) return <div className="p-8 text-center text-muted text-sm">Loading entitlements...</div>
  if (data?.error) return <div className="p-4 text-center text-sm text-red-400">{data.error}</div>
  if (data?.entitlements?.length > 0) {
    return (
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border/50">
              <th className="table-header">Type</th>
              <th className="table-header">Identity</th>
              <th className="table-header">Role / Source</th>
              <th className="table-header">Application</th>
              <th className="table-header">Status</th>
            </tr>
          </thead>
          <tbody>
            {data.entitlements.map((e: any, i: number) => (
              <tr key={i} className="table-row">
                <td className="table-cell">
                  <Badge variant={
                    e.entitlement_type === "directory_role" ? "info" :
                    e.entitlement_type === "app_role" ? "warning" : "success"
                  }>
                    {e.entitlement_type?.replace("_", " ")}
                  </Badge>
                </td>
                <td className="table-cell font-mono text-xs">{e.identity_external_id?.slice(0, 12)}...</td>
                <td className="table-cell text-sm">{e.source_name || e.source_id?.slice(0, 12) || "-"}</td>
                <td className="table-cell text-xs text-secondary">{e.app_name || "-"}</td>
                <td className="table-cell"><Badge variant={e.is_active ? "success" : "neutral"}>{e.is_active ? "Active" : "Inactive"}</Badge></td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="px-4 py-2 text-xs text-muted border-t border-border/50">
          {data.total} entitlements
        </div>
      </div>
    )
  }
  return <div className="p-8 text-center text-sm text-secondary">No entitlements synced. Full Sync will import directory roles and app role assignments.</div>
}

function renderResourcesTab(data: any, load: boolean, _connectorId: string) {
  if (load) return <div className="p-8 text-center text-muted text-sm">Loading resources...</div>
  if (data?.error) return <div className="p-4 text-center text-sm text-red-400">{data.error}</div>
  if (data?.resources?.length > 0) {
    const byType: Record<string, any[]> = {}
    for (const r of data.resources) {
      (byType[r.resource_type] = byType[r.resource_type] || []).push(r)
    }

    return (
      <div className="px-5 py-4 space-y-4">
        {Object.entries(byType).map(([type, items]) => (
          <div key={type}>
            <h4 className="text-xs font-bold text-muted uppercase tracking-wider mb-2 font-mono">
              {type.replace("_", " ")} ({items.length})
            </h4>
            <div className="grid grid-cols-2 gap-2">
              {items.slice(0, 20).map((r: any) => (
                <div key={r.id} className="p-3 rounded bg-white/[0.02] border border-border/50">
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-sm font-medium">{r.name || r.external_id?.slice(0, 16)}</span>
                    <Badge variant={r.enabled ? "success" : "neutral"}>{r.enabled ? "Enabled" : "Disabled"}</Badge>
                  </div>
                  {r.description && <p className="text-xs text-muted truncate">{r.description}</p>}
                  <p className="text-[10px] text-muted mt-1">{r.owner_ids?.length || 0} owners</p>
                </div>
              ))}
              {items.length > 20 && (
                <p className="text-xs text-muted col-span-2 text-center py-2">+{items.length - 20} more {type}</p>
              )}
            </div>
          </div>
        ))}
        <div className="text-xs text-muted pt-2 border-t border-border/50">
          {data.total} total resources
        </div>
      </div>
    )
  }
  return <div className="p-8 text-center text-sm text-secondary">No resources synced. Full Sync will import applications, service principals, and devices.</div>
}

function renderSchemaTab(data: any, load: boolean) {
  if (load) return <div className="p-8 text-center text-muted text-sm">Loading schema...</div>
  if (data?.schema?.attributes?.length > 0) {
    return (
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
            {data.schema.attributes.map((a: any) => (
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
          {data.schema.count} attributes discovered · {data.schema.object_type} schema
        </div>
      </div>
    )
  }
  return <div className="p-8 text-center text-sm text-secondary">Schema not available. Connect and sync to discover attributes.</div>
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
      className={`text-xs font-semibold pb-2 border-b-2 transition-colors whitespace-nowrap ${
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
