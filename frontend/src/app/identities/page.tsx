"use client"

import { useState } from "react"

export default function IdentitiesPage() {
  const [selectedTab, setSelectedTab] = useState<"human" | "agents">("human")

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
              Humans
            </button>
            <button
              onClick={() => setSelectedTab("agents")}
              className={`px-4 py-1.5 rounded-md text-sm font-medium transition-all ${
                selectedTab === "agents" ? "bg-brand-600 text-white" : "text-gray-400 hover:text-gray-200"
              }`}
            >
              AI Agents & NHI
            </button>
          </div>
          <button className="btn-primary text-sm">Add Identity</button>
        </div>
      </div>

      {/* Filters */}
      <div className="glass-card p-4">
        <div className="flex items-center gap-4">
          <input className="input max-w-xs" placeholder="Search by name, email, or ID..." />
          <select className="input max-w-[150px]">
            <option>All Status</option>
            <option>Active</option>
            <option>Suspended</option>
            <option>Terminated</option>
          </select>
          <select className="input max-w-[150px]">
            <option>All Departments</option>
            <option>Engineering</option>
            <option>Finance</option>
            <option>HR</option>
          </select>
          <button className="btn-secondary text-sm">Export</button>
        </div>
      </div>

      {/* Identity List */}
      {selectedTab === "human" ? <HumanIdentitiesList /> : <AgentIdentitiesList />}
    </div>
  )
}

function HumanIdentitiesList() {
  return (
    <div className="glass-card overflow-hidden">
      <table className="w-full">
        <thead>
          <tr className="border-b border-gray-800">
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Identity</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Email</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Department</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Risk Score</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Roles</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">MFA</th>
            <th className="text-right py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-800/50">
          {mockIdentities.map((identity) => (
            <tr key={identity.id} className="hover:bg-surface-100/50 transition-colors">
              <td className="py-3 px-4">
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-full bg-brand-600/20 border border-brand-500/30 flex items-center justify-center text-xs font-medium text-brand-400">
                    {identity.initials}
                  </div>
                  <div>
                    <p className="text-sm font-medium text-white">{identity.name}</p>
                    <p className="text-xs text-gray-500">{identity.employeeId}</p>
                  </div>
                </div>
              </td>
              <td className="py-3 px-4 text-sm text-gray-400">{identity.email}</td>
              <td className="py-3 px-4 text-sm text-gray-400">{identity.department}</td>
              <td className="py-3 px-4">
                <StatusBadge status={identity.status} />
              </td>
              <td className="py-3 px-4">
                <RiskBadge score={identity.riskScore} />
              </td>
              <td className="py-3 px-4 text-sm text-gray-400">{identity.roleCount}</td>
              <td className="py-3 px-4">
                <span className={identity.mfaEnabled ? "badge-success" : "badge-warning"}>
                  {identity.mfaEnabled ? "AAL2+" : "AAL1"}
                </span>
              </td>
              <td className="py-3 px-4 text-right">
                <button className="btn-ghost text-xs">View</button>
                <button className="btn-ghost text-xs text-rose-400 hover:text-rose-300">Revoke</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function AgentIdentitiesList() {
  return (
    <div className="glass-card overflow-hidden">
      <table className="w-full">
        <thead>
          <tr className="border-b border-gray-800">
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Agent</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Type</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Owner</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Protocols</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Risk</th>
            <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Governed</th>
            <th className="text-right py-3 px-4 text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-800/50">
          {mockAgents.map((agent) => (
            <tr key={agent.id} className="hover:bg-surface-100/50 transition-colors">
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
              <td className="py-3 px-4 text-sm text-gray-400 capitalize">{agent.type.replace("_", " ")}</td>
              <td className="py-3 px-4 text-sm text-gray-400">{agent.owner}</td>
              <td className="py-3 px-4">
                <div className="flex gap-1">
                  {agent.protocols.map((p) => (
                    <span key={p} className="badge-neutral text-[10px]">{p.toUpperCase()}</span>
                  ))}
                </div>
              </td>
              <td className="py-3 px-4">
                <StatusBadge status={agent.status} />
              </td>
              <td className="py-3 px-4">
                <RiskBadge score={agent.riskScore} />
              </td>
              <td className="py-3 px-4">
                <span className={agent.isGoverned ? "badge-success" : "badge-danger"}>
                  {agent.isGoverned ? "Governed" : "Shadow"}
                </span>
              </td>
              <td className="py-3 px-4 text-right">
                <button className="btn-ghost text-xs">View</button>
                <button className="btn-ghost text-xs text-rose-400 hover:text-rose-300">Kill</button>
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
    suspended: { class: "badge-warning", label: "Suspended" },
    terminated: { class: "badge-danger", label: "Terminated" },
    revoked: { class: "badge-danger", label: "Revoked" },
    pending: { class: "badge-info", label: "Pending" },
  }
  const c = config[status] || { class: "badge-neutral", label: status }
  return <span className={c.class}>{c.label}</span>
}

function RiskBadge({ score }: { score: number }) {
  if (score >= 0.7) return <span className="badge-danger">Critical ({Math.round(score * 100)}%)</span>
  if (score >= 0.4) return <span className="badge-warning">Elevated ({Math.round(score * 100)}%)</span>
  return <span className="badge-success">Low ({Math.round(score * 100)}%)</span>
}

// ─── Mock Data ─────────────────────────────────────────────

const mockIdentities = [
  { id: "1", name: "Alice Johnson", initials: "AJ", email: "alice@observeid.io", department: "Engineering", employeeId: "EMP-001", status: "active", riskScore: 0.15, roleCount: 3, mfaEnabled: true },
  { id: "2", name: "Bob Smith", initials: "BS", email: "bob@observeid.io", department: "Engineering", employeeId: "EMP-002", status: "terminated", riskScore: 0.05, roleCount: 0, mfaEnabled: true },
  { id: "3", name: "Charlie Davis", initials: "CD", email: "charlie@observeid.io", department: "Finance", employeeId: "EMP-003", status: "active", riskScore: 0.35, roleCount: 5, mfaEnabled: true },
  { id: "4", name: "Diana Moore", initials: "DM", email: "diana@partner.com", department: "Engineering", employeeId: "CON-001", status: "active", riskScore: 0.68, roleCount: 2, mfaEnabled: false },
  { id: "5", name: "Eve Wilson", initials: "EW", email: "eve@observeid.io", department: "HR", employeeId: "EMP-004", status: "active", riskScore: 0.22, roleCount: 4, mfaEnabled: true },
  { id: "6", name: "Frank Taylor", initials: "FT", email: "frank@observeid.io", department: "Security", employeeId: "EMP-005", status: "active", riskScore: 0.08, roleCount: 6, mfaEnabled: true },
  { id: "7", name: "Grace Lee", initials: "GL", email: "grace@vendor.com", department: "Finance", employeeId: "CON-002", status: "suspended", riskScore: 0.72, roleCount: 3, mfaEnabled: false },
  { id: "8", name: "Henry Brown", initials: "HB", email: "henry@observeid.io", department: "Engineering", employeeId: "EMP-006", status: "active", riskScore: 0.45, roleCount: 8, mfaEnabled: true },
]

const mockAgents = [
  { id: "a1", name: "deploy-bot", type: "ai_agent", owner: "Alice Johnson", protocols: ["mcp", "a2a"], status: "active", riskScore: 0.35, isGoverned: true },
  { id: "a2", name: "doc-analyzer", type: "ai_agent", owner: "Alice Johnson", protocols: ["mcp"], status: "active", riskScore: 0.42, isGoverned: true },
  { id: "a3", name: "invoice-bot", type: "rpa_bot", owner: "Charlie Davis", protocols: ["oauth2"], status: "active", riskScore: 0.55, isGoverned: false },
  { id: "a4", name: "prod-db-reader", type: "service_account", owner: "Frank Taylor", protocols: ["spiffe", "oauth2"], status: "active", riskScore: 0.72, isGoverned: true },
  { id: "a5", name: "data-sync", type: "service_account", owner: "Henry Brown", protocols: ["oauth2"], status: "active", riskScore: 0.61, isGoverned: false },
  { id: "a6", name: "slack-notifier", type: "ai_agent", owner: "None", protocols: ["mcp"], status: "active", riskScore: 0.88, isGoverned: false },
  { id: "a7", name: "spot-robot-01", type: "robot", owner: "Henry Brown", protocols: ["spiffe"], status: "active", riskScore: 0.25, isGoverned: true },
]
