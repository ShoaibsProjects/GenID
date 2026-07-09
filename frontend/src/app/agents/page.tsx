"use client"

import { useState, useEffect } from "react"
import { fetchAgents, Agent } from "@/lib/api"

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetchAgents()
      .then((d) => setAgents(d.agents || []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">AI Agents & NHI</h1>
          <p className="text-sm text-gray-400 mt-1">{agents.length} non-human identities registered</p>
        </div>
        <button className="btn-primary text-sm">Register Agent</button>
      </div>

      <div className="glass-card overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading agents...</div>
        ) : agents.length === 0 ? (
          <div className="p-12 text-center text-gray-500">No agents registered. Use POST /api/v1/agents to register one.</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-800">
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Agent</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Type</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Owner</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Risk</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Governed</th>
                <th className="text-right py-3 px-4 text-xs font-medium text-gray-500 uppercase">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800/50">
              {agents.map((a) => (
                <tr key={a.uuid} className="hover:bg-surface-100/50 transition-colors">
                  <td className="py-3 px-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-lg bg-violet-600/20 flex items-center justify-center text-xs font-medium text-violet-400 border border-violet-500/30">
                        {(a.name || "?").charAt(0).toUpperCase()}
                      </div>
                      <span className="text-sm font-medium text-white">{a.name}</span>
                    </div>
                  </td>
                  <td className="py-3 px-4 text-sm text-gray-400 capitalize">{a.type?.replace("_", " ")}</td>
                  <td className="py-3 px-4 text-sm text-gray-400">{a.owner_name || "-"}</td>
                  <td className="py-3 px-4">
                    <StatusBadge status={a.status} />
                  </td>
                  <td className="py-3 px-4">
                    <RiskBadge score={parseFloat(a.risk_score || "0")} />
                  </td>
                  <td className="py-3 px-4">
                    <span className={a.is_governed === "true" ? "badge-success" : "badge-danger"}>
                      {a.is_governed === "true" ? "Governed" : "Shadow"}
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
        )}
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
  }
  const c = config[status] || { class: "badge-neutral", label: status }
  return <span className={c.class}>{c.label}</span>
}

function RiskBadge({ score }: { score: number }) {
  if (score >= 0.7) return <span className="badge-danger">Critical ({Math.round(score * 100)}%)</span>
  if (score >= 0.4) return <span className="badge-warning">Elevated ({Math.round(score * 100)}%)</span>
  return <span className="badge-success">Low ({Math.round(score * 100)}%)</span>
}
