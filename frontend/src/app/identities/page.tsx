"use client"

import { useState, useEffect } from "react"
import { fetchIdentities, fetchAgents, Identity, Agent } from "@/lib/api"

export default function IdentitiesPage() {
  const [selectedTab, setSelectedTab] = useState<"human" | "agents">("human")
  const [humans, setHumans] = useState<Identity[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState("")
  const [statusFilter, setStatusFilter] = useState("")

  useEffect(() => {
    async function load() {
      try {
        const [idents, agentsData] = await Promise.all([
          fetchIdentities().catch(() => null),
          fetchAgents().catch(() => null),
        ])
        setHumans(idents?.identities || [])
        setAgents(agentsData?.agents || [])
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

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Identities</h1>
          <p className="text-sm text-gray-400 mt-1">Complete inventory of human and non-human identities</p>
        </div>
        <div className="flex items-center gap-3">
          <div className="flex bg-surface-200 rounded-lg p-1">
            <button
              onClick={() => setSelectedTab("human")}
              className={`px-4 py-1.5 rounded-md text-sm font-medium transition-all ${
                selectedTab === "human" ? "bg-brand-600 text-white" : "text-gray-400 hover:text-gray-200"
              }`}
            >
              Humans ({humans.length})
            </button>
            <button
              onClick={() => setSelectedTab("agents")}
              className={`px-4 py-1.5 rounded-md text-sm font-medium transition-all ${
                selectedTab === "agents" ? "bg-brand-600 text-white" : "text-gray-400 hover:text-gray-200"
              }`}
            >
              AI Agents & NHI ({agents.length})
            </button>
          </div>
          <button className="btn-primary text-sm">Add Identity</button>
        </div>
      </div>

      {/* Filters */}
      <div className="glass-card p-4">
        <div className="flex items-center gap-4">
          <input className="input max-w-xs" placeholder="Search by name, email, or ID..." value={search} onChange={(e) => setSearch(e.target.value)} />
          <select className="input max-w-[150px]" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
            <option value="">All Status</option>
            <option value="active">Active</option>
            <option value="inactive">Inactive</option>
            <option value="suspended">Suspended</option>
            <option value="terminated">Terminated</option>
            <option value="revoked">Revoked</option>
          </select>
          <span className="text-xs text-gray-500">{loading ? "Loading..." : `${filteredHumans.length} results`}</span>
        </div>
      </div>

      {loading ? (
        <div className="glass-card p-12 text-center text-gray-500">Loading identities from API...</div>
      ) : selectedTab === "human" ? (
        <HumanIdentitiesList identities={filteredHumans} />
      ) : (
        <AgentIdentitiesList agents={filteredAgents} />
      )}
    </div>
  )
}

function HumanIdentitiesList({ identities }: { identities: Identity[] }) {
  if (identities.length === 0) {
    return <div className="glass-card p-12 text-center text-gray-500">No identities found. Add some via the API.</div>
  }

  return (
    <div className="glass-card overflow-hidden">
      <table className="w-full">
        <thead>
          <tr className="border-b border-gray-800">
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Identity</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Email</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Department</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Type</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Risk Score</th>
            <th className="text-right py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-800/50">
          {identities.map((identity) => (
            <tr key={identity.uuid} className="hover:bg-surface-100/50 transition-colors">
              <td className="py-3 px-4">
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-full bg-brand-600/20 border border-brand-500/30 flex items-center justify-center text-xs font-medium text-brand-400">
                    {(identity.name || identity.email || "?").charAt(0).toUpperCase()}
                  </div>
                  <div>
                    <p className="text-sm font-medium text-white">{identity.name || "Unnamed"}</p>
                  </div>
                </div>
              </td>
              <td className="py-3 px-4 text-sm text-gray-400">{identity.email}</td>
              <td className="py-3 px-4 text-sm text-gray-400">{identity.department || "-"}</td>
              <td className="py-3 px-4">
                <StatusBadge status={identity.status} />
              </td>
              <td className="py-3 px-4 text-sm text-gray-400">{identity.type}</td>
              <td className="py-3 px-4">
                <RiskBadge score={parseFloat(identity.risk_score || "0")} />
              </td>
              <td className="py-3 px-4 text-right">
                <button className="btn-ghost text-xs">View</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function AgentIdentitiesList({ agents }: { agents: Agent[] }) {
  if (agents.length === 0) {
    return <div className="glass-card p-12 text-center text-gray-500">No agents registered. Register one via the API.</div>
  }

  return (
    <div className="glass-card overflow-hidden">
      <table className="w-full">
        <thead>
          <tr className="border-b border-gray-800">
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Agent</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Type</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Owner</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Risk</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Governed</th>
            <th className="text-right py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-800/50">
          {agents.map((agent) => (
            <tr key={agent.uuid} className="hover:bg-surface-100/50 transition-colors">
              <td className="py-3 px-4">
                <div className="flex items-center gap-3">
                  <div className={`w-8 h-8 rounded-lg flex items-center justify-center text-xs font-medium ${
                    agent.type === "ai_agent" ? "bg-violet-600/20 text-violet-400 border border-violet-500/30" :
                    agent.type === "rpa_bot" ? "bg-amber-600/20 text-amber-400 border border-amber-500/30" :
                    "bg-sky-600/20 text-sky-400 border border-sky-500/30"
                  }`}>
                    {agent.type === "ai_agent" ? "AI" : agent.type === "rpa_bot" ? "RP" : "SA"}
                  </div>
                  <span className="text-sm font-medium text-white">{agent.name}</span>
                </div>
              </td>
              <td className="py-3 px-4 text-sm text-gray-400 capitalize">{agent.type?.replace("_", " ")}</td>
              <td className="py-3 px-4 text-sm text-gray-400">{agent.owner_name || "-"}</td>
              <td className="py-3 px-4">
                <StatusBadge status={agent.status} />
              </td>
              <td className="py-3 px-4">
                <RiskBadge score={parseFloat(agent.risk_score || "0")} />
              </td>
              <td className="py-3 px-4">
                <span className={agent.is_governed === "true" ? "badge-success" : "badge-danger"}>
                  {agent.is_governed === "true" ? "Governed" : "Shadow"}
                </span>
              </td>
              <td className="py-3 px-4 text-right">
                <button className="btn-ghost text-xs">View</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ─── Shared Components ─────────────────────────────────────

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
