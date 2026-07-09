"use client"

import { useState, useEffect } from "react"
import { fetchIdentities, fetchAgents, Identity, Agent } from "@/lib/api"

export default function IdentitiesPage() {
  const [selectedTab, setSelectedTab] = useState<"human" | "agents" | "synced">("human")
  const [humans, setHumans] = useState<Identity[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [synced, setSynced] = useState<any[]>([])
  const [connectors, setConnectors] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState("")
  const [statusFilter, setStatusFilter] = useState("")

  useEffect(() => {
    async function load() {
      try {
        const [idents, agentsData, conns] = await Promise.all([
          fetchIdentities().catch(() => null),
          fetchAgents().catch(() => null),
          fetch("/api/v1/connectors").then(r => r.json()).catch(() => null),
        ])
        setHumans(idents?.identities || [])
        setAgents(agentsData?.agents || [])

        // Fetch synced identities from all connectors
        const connList = conns?.connectors || []
        setConnectors(connList)
        const allSynced: any[] = []
        for (const c of connList) {
          try {
            const data = await fetch(`/api/v1/connectors/${c.id}/identities`).then(r => r.json())
            if (data?.identities) {
              for (const id of data.identities) {
                allSynced.push({ ...id, connector_name: c.name, connector_type: c.type })
              }
            }
          } catch (_) {}
        }
        setSynced(allSynced)
      } catch (err) {
        console.error("Failed to load identities:", err)
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [])

  const filteredHumans = humans.filter((i) => {
    if (search && !i.name?.toLowerCase().includes(search.toLowerCase()) && !i.email?.toLowerCase().includes(search.toLowerCase())) return false
    if (statusFilter && i.status !== statusFilter) return false
    return true
  })

  const filteredAgents = agents.filter((a) => {
    if (search && !a.name?.toLowerCase().includes(search.toLowerCase())) return false
    if (statusFilter && a.status !== statusFilter) return false
    return true
  })

  const filteredSynced = synced.filter((s) => {
    if (search && !s.display_name?.toLowerCase().includes(search.toLowerCase()) && !s.email?.toLowerCase().includes(search.toLowerCase()) && !s.username?.toLowerCase().includes(search.toLowerCase())) return false
    return true
  })

  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Identities</h1>
          <p className="text-sm text-gray-400 mt-1">Complete inventory of all identity types</p>
        </div>
        <div className="flex bg-surface-200 rounded-lg p-1">
          <button
            onClick={() => setSelectedTab("human")}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
              selectedTab === "human" ? "bg-brand-600 text-white" : "text-gray-400 hover:text-gray-200"
            }`}
          >
            Registry ({humans.length})
          </button>
          <button
            onClick={() => setSelectedTab("agents")}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
              selectedTab === "agents" ? "bg-brand-600 text-white" : "text-gray-400 hover:text-gray-200"
            }`}
          >
            NHI ({agents.length})
          </button>
          <button
            onClick={() => setSelectedTab("synced")}
            className={`px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
              selectedTab === "synced" ? "bg-brand-600 text-white" : "text-gray-400 hover:text-gray-200"
            }`}
          >
            Connector ({synced.length})
          </button>
        </div>
      </div>

      <div className="glass-card p-4">
        <div className="flex items-center gap-4">
          <input className="input max-w-xs" placeholder="Search..." value={search} onChange={(e) => setSearch(e.target.value)} />
          {selectedTab !== "synced" && (
            <select className="input max-w-[150px]" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
              <option value="">All Status</option>
              <option value="active">Active</option>
              <option value="inactive">Inactive</option>
              <option value="suspended">Suspended</option>
              <option value="terminated">Terminated</option>
              <option value="revoked">Revoked</option>
            </select>
          )}
          <span className="text-xs text-gray-500">
            {loading ? "Loading..." :
              selectedTab === "human" ? `${filteredHumans.length} results` :
              selectedTab === "agents" ? `${filteredAgents.length} results` :
              `${filteredSynced.length} results`}
          </span>
        </div>
      </div>

      {loading ? (
        <div className="glass-card p-12 text-center text-gray-500">Loading...</div>
      ) : selectedTab === "human" ? (
        <HumanIdentitiesList identities={filteredHumans} />
      ) : selectedTab === "agents" ? (
        <AgentIdentitiesList agents={filteredAgents} />
      ) : (
        <SyncedIdentitiesList identities={filteredSynced} connectors={connectors} onRefresh={() => window.location.reload()} />
      )}
    </div>
  )
}

function HumanIdentitiesList({ identities }: { identities: Identity[] }) {
  if (identities.length === 0) {
    return <div className="glass-card p-12 text-center text-gray-500">No identities in registry. Create one via API or SCIM.</div>
  }
  return (
    <div className="glass-card overflow-hidden">
      <table className="w-full">
        <thead>
          <tr className="border-b border-gray-800">
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Identity</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Email</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Department</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Status</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Type</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Risk</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-800/50">
          {identities.map((identity) => (
            <tr key={identity.uuid} className="hover:bg-surface-100/50">
              <td className="py-3 px-4">
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-full bg-brand-600/20 border border-brand-500/30 flex items-center justify-center text-xs font-medium text-brand-400">
                    {(identity.name || identity.email || "?").charAt(0).toUpperCase()}
                  </div>
                  <span className="text-sm font-medium text-white">{identity.name || "Unnamed"}</span>
                </div>
              </td>
              <td className="py-3 px-4 text-sm text-gray-400">{identity.email}</td>
              <td className="py-3 px-4 text-sm text-gray-400">{identity.department || "-"}</td>
              <td className="py-3 px-4"><StatusBadge status={identity.status} /></td>
              <td className="py-3 px-4 text-sm text-gray-400">{identity.type}</td>
              <td className="py-3 px-4"><RiskBadge score={parseFloat(identity.risk_score || "0")} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function AgentIdentitiesList({ agents }: { agents: Agent[] }) {
  if (agents.length === 0) {
    return <div className="glass-card p-12 text-center text-gray-500">No NHI agents registered</div>
  }
  return (
    <div className="glass-card overflow-hidden">
      <table className="w-full">
        <thead>
          <tr className="border-b border-gray-800">
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Agent</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Type</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Owner</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Status</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Risk</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Governed</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-800/50">
          {agents.map((agent) => (
            <tr key={agent.uuid} className="hover:bg-surface-100/50">
              <td className="py-3 px-4">
                <div className="flex items-center gap-3">
                  <div className={`w-8 h-8 rounded-lg flex items-center justify-center text-xs font-medium ${
                    agent.type === "ai_agent" ? "bg-violet-600/20 text-violet-400 border border-violet-500/30" :
                    agent.type === "rpa_bot" ? "bg-amber-600/20 text-amber-400 border border-amber-500/30" :
                    "bg-sky-600/20 text-sky-400 border border-sky-500/30"}`}>
                    {agent.type === "ai_agent" ? "AI" : agent.type === "rpa_bot" ? "RP" : "SA"}
                  </div>
                  <span className="text-sm font-medium text-white">{agent.name}</span>
                </div>
              </td>
              <td className="py-3 px-4 text-sm text-gray-400 capitalize">{agent.type?.replace("_", " ")}</td>
              <td className="py-3 px-4 text-sm text-gray-400">{agent.owner_name || "-"}</td>
              <td className="py-3 px-4"><StatusBadge status={agent.status} /></td>
              <td className="py-3 px-4"><RiskBadge score={parseFloat(agent.risk_score || "0")} /></td>
              <td className="py-3 px-4">
                <span className={agent.is_governed === "true" ? "badge-success" : "badge-danger"}>
                  {agent.is_governed === "true" ? "Governed" : "Shadow"}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function SyncedIdentitiesList({ identities, connectors, onRefresh }: { identities: any[]; connectors: any[]; onRefresh: () => void }) {
  if (identities.length === 0) {
    return (
      <div className="glass-card p-12 text-center text-gray-500">
        <p className="mb-2">No connector-synced identities</p>
        <p className="text-xs text-gray-600">Go to Connectors → Add a connector → Connect → Sync to import identities</p>
        {connectors.length > 0 && (
          <div className="mt-4">
            <p className="text-xs text-gray-500 mb-2">Available connectors ({connectors.length}):</p>
            {connectors.map((c: any) => (
              <span key={c.id} className="inline-block px-2 py-1 mx-1 rounded text-xs bg-surface-200 text-gray-400">{c.name} ({c.status})</span>
            ))}
          </div>
        )}
      </div>
    )
  }

  return (
    <div className="glass-card overflow-hidden">
      <table className="w-full">
        <thead>
          <tr className="border-b border-gray-800">
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">User</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Email</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Source</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Department</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Status</th>
            <th className="text-right py-3 px-4 text-xs font-medium text-gray-500 uppercase">Last Synced</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-800/50">
          {identities.map((s: any) => (
            <tr key={s.id} className="hover:bg-surface-100/50">
              <td className="py-3 px-4">
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-full bg-brand-600/20 border border-brand-500/30 flex items-center justify-center text-xs font-medium text-brand-400">
                    {(s.display_name || s.email || s.username || "?").charAt(0).toUpperCase()}
                  </div>
                  <span className="text-sm font-medium text-white">{s.display_name || s.username || "Unnamed"}</span>
                </div>
              </td>
              <td className="py-3 px-4 text-sm text-gray-400">{s.email || "-"}</td>
              <td className="py-3 px-4">
                <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs bg-brand-500/10 text-brand-400 border border-brand-500/20">
                  {s.connector_name || "Unknown"}
                </span>
              </td>
              <td className="py-3 px-4 text-sm text-gray-400">{s.department || "-"}</td>
              <td className="py-3 px-4">
                <span className={s.enabled ? "badge-success" : "badge-neutral"}>
                  {s.enabled ? "Active" : "Disabled"}
                </span>
              </td>
              <td className="py-3 px-4 text-right text-sm text-gray-400">
                {s.last_synced_at ? new Date(s.last_synced_at).toLocaleDateString() : "-"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="px-4 py-3 text-xs text-gray-500 border-t border-gray-800/50 text-center">
        {identities.length} identity{identities.length !== 1 ? "ies" : "y"} synced from {new Set(identities.map((s: any) => s.connector_name)).size} connector(s)
      </div>
    </div>
  )
}

function StatusBadge({ status }: { status: string }) {
  const config: Record<string, { class: string; label: string }> = {
    active: { class: "badge-success", label: "Active" },
    inactive: { class: "badge-neutral", label: "Inactive" },
    suspended: { class: "badge-warning", label: "Suspended" },
    terminated: { class: "badge-danger", label: "Terminated" },
    revoked: { class: "badge-danger", label: "Revoked" },
    pending_review: { class: "badge-info", label: "Pending" },
  }
  const c = config[status] || { class: "badge-neutral", label: status }
  return <span className={c.class}>{c.label}</span>
}

function RiskBadge({ score }: { score: number }) {
  if (score >= 0.7) return <span className="badge-danger">Critical ({Math.round(score * 100)}%)</span>
  if (score >= 0.4) return <span className="badge-warning">Elevated ({Math.round(score * 100)}%)</span>
  return <span className="badge-success">Low ({Math.round(score * 100)}%)</span>
}
