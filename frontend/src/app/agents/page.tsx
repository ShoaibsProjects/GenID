"use client"

import { useState, useEffect, useCallback } from "react"
import { fetchAgents, Agent, authFetch } from "@/lib/api"

// ─── Constants ──────────────────────────────────────────────

const STATUS_COLORS: Record<string, string> = {
  active: "text-emerald-400 bg-emerald-500/10 border-emerald-500/30",
  inactive: "text-gray-400 bg-gray-500/10 border-gray-500/30",
  suspended: "text-amber-400 bg-amber-500/10 border-amber-500/30",
  revoked: "text-red-400 bg-red-500/10 border-red-500/30",
  pending_review: "text-purple-400 bg-purple-500/10 border-purple-500/30",
}

const TYPE_COLORS: Record<string, string> = {
  service_account: "text-teal-400 bg-teal-500/10 border-teal-500/30",
  ai_agent: "text-violet-400 bg-violet-500/10 border-violet-500/30",
  robot: "text-orange-400 bg-orange-500/10 border-orange-500/30",
  api_key: "text-yellow-400 bg-yellow-500/10 border-yellow-500/30",
}

function riskClass(score: number) {
  if (score >= 0.7) return "text-red-400"
  if (score >= 0.4) return "text-amber-400"
  return "text-emerald-400"
}

// ─── Main Page ──────────────────────────────────────────────

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [killing, setKilling] = useState<string | null>(null)
  const [showModal, setShowModal] = useState(false)
  const [toast, setToast] = useState("")

  const load = useCallback(async () => {
    try {
      const d = await fetchAgents()
      setAgents(d.agents || [])
      setError("")
    } catch (e: any) {
      setError(e.message || "Failed to load agents")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  function notify(msg: string) {
    setToast(msg)
    setTimeout(() => setToast(""), 3000)
  }

  const handleKillSwitch = async (agentId: string, name: string) => {
    if (!confirm(`Kill agent "${name}"? This revokes all active sessions and tokens immediately.`)) return
    setKilling(agentId)
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json" }
      const res = await authFetch(`/api/v1/agents/${agentId}/kill-switch`, { method: "POST", headers })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) {
        notify(`Kill switch failed: ${body.detail || body.error || res.status}`)
        return
      }
      console.log("kill-switch response:", body)
      setAgents(prev => prev.map(a => a.id === agentId ? { ...a, status: "revoked" } : a))
      notify(`Agent "${name}" killed — all sessions revoked`)
    } catch (e: any) {
      notify("Kill switch failed: " + e.message)
    } finally {
      setKilling(null)
    }
  }

  const governed = agents.filter(a => a.is_governed).length

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">AI Agents & NHI</h1>
          <p className="text-sm text-gray-400 mt-1">
            {agents.length} registered · {governed} governed · {agents.length - governed} ungoverned
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button className="btn-secondary text-xs px-3 py-1.5" onClick={load}>Refresh</button>
          <button className="btn-primary text-xs px-3 py-1.5" onClick={() => setShowModal(true)}>+ Register Agent</button>
        </div>
      </div>

      {/* Toast */}
      {toast && (
        <div className="glass-card px-4 py-2.5 text-sm text-amber-400 border border-amber-500/30 animate-slide-in">
          {toast}
        </div>
      )}

      {/* Table */}
      <div className="glass-card overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading agents...</div>
        ) : error ? (
          <div className="p-12 text-center text-red-400">{error}</div>
        ) : agents.length === 0 ? (
          <div className="p-12 text-center text-gray-500">
            <p className="mb-2">No agents registered</p>
            <p className="text-xs text-gray-600">Click + Register Agent to onboard your first non-human identity</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-800">
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">Agent</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">Type</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">Status</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">Risk</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">Governed</th>
                  <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">Owner</th>
                  <th className="text-right py-2.5 px-3 text-xs font-medium text-gray-500 uppercase w-28">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800/50">
                {agents.map((a) => (
                  <tr key={a.id} className="hover:bg-surface-100/50 transition-colors">
                    <td className="py-2.5 px-3">
                      <div className="flex items-center gap-2.5">
                        <span className="shrink-0 w-8 h-8 rounded-lg bg-violet-500/15 border border-violet-500/30 flex items-center justify-center text-sm">
                          🤖
                        </span>
                        <div>
                          <div className="text-sm text-gray-200 font-medium">{a.name}</div>
                          <div className="text-xs text-gray-600 font-mono">{a.id?.substring(0, 8)}…</div>
                        </div>
                      </div>
                    </td>
                    <td className="py-2.5 px-3">
                      <span className={`px-2 py-0.5 rounded-full text-xs border ${TYPE_COLORS[a.type] || "text-gray-400 bg-gray-500/10 border-gray-500/30"}`}>
                        {a.type?.replace("_", " ") || "agent"}
                      </span>
                    </td>
                    <td className="py-2.5 px-3">
                      <span className={`px-2 py-0.5 rounded-full text-xs border ${STATUS_COLORS[a.status] || STATUS_COLORS.inactive}`}>
                        {a.status?.replace("_", " ")}
                      </span>
                    </td>
                    <td className="py-2.5 px-3">
                      <span className={`text-sm font-mono font-medium ${riskClass(a.risk_score || 0)}`}>
                        {(a.risk_score || 0).toFixed(2)}
                      </span>
                    </td>
                    <td className="py-2.5 px-3">
                      {a.is_governed ? (
                        <span className="badge-success">Yes</span>
                      ) : (
                        <span className="badge-warning">No</span>
                      )}
                    </td>
                    <td className="py-2.5 px-3 text-sm text-gray-400">{a.owner_name || "—"}</td>
                    <td className="py-2.5 px-3 text-right">
                      <button
                        className="btn-danger text-xs px-2.5 py-1"
                        disabled={killing === a.id || a.status === "revoked"}
                        onClick={() => handleKillSwitch(a.id, a.name)}
                        title={a.status === "revoked" ? "Already revoked" : "Immediately revoke all agent access"}
                      >
                        {killing === a.id ? "Killing…" : a.status === "revoked" ? "Revoked" : "⛔ Kill Switch"}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Explainer card */}
      <div className="glass-card p-4">
        <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-2">Kill Switch</h3>
        <p className="text-xs text-gray-500 leading-relaxed">
          The kill switch immediately revokes the agent&apos;s active JWTs (Redis blocklist), marks the NHI as
          <span className="text-red-400"> revoked </span> in Neo4j, and triggers the Temporal
          <span className="text-amber-400"> CascadeRevokeWorkflow </span> to terminate any delegated child agents.
          Requires master permission (admin role or X-Master-Key).
        </p>
      </div>

      {showModal && (
        <RegisterAgentModal
          onClose={() => setShowModal(false)}
          onCreated={(name) => { setShowModal(false); notify(`Agent "${name}" registered`); load() }}
        />
      )}
    </div>
  )
}

// ─── Register Agent Modal ───────────────────────────────────

function RegisterAgentModal({ onClose, onCreated }: { onClose: () => void; onCreated: (name: string) => void }) {
  const [name, setName] = useState("")
  const [type, setType] = useState("ai_agent")
  const [owner, setOwner] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [err, setErr] = useState("")

  const submit = async () => {
    if (!name.trim()) { setErr("Name is required"); return }
    setSubmitting(true)
    setErr("")
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json" }
      const res = await authFetch("/api/v1/agents", {
        method: "POST",
        headers,
        body: JSON.stringify({ name: name.trim(), type, owner_name: owner.trim() || undefined }),
      })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) {
        setErr(body.detail || body.error || `Registration failed (${res.status})`)
        return
      }
      onCreated(name.trim())
    } catch (e: any) {
      setErr(e.message || "Registration failed")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={onClose}>
      <div className="w-full max-w-md glass-card p-6 space-y-4" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold text-white">Register AI Agent</h2>
          <button className="text-gray-400 hover:text-white text-xl leading-none" onClick={onClose}>&times;</button>
        </div>

        <div className="space-y-3">
          <div>
            <label className="text-xs text-gray-400 block mb-1">Agent Name *</label>
            <input
              className="input text-sm py-1.5"
              placeholder="e.g. deploy-bot-prod"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
          </div>
          <div>
            <label className="text-xs text-gray-400 block mb-1">Type</label>
            <select className="input text-sm py-1.5" value={type} onChange={(e) => setType(e.target.value)}>
              <option value="ai_agent">AI Agent</option>
              <option value="service_account">Service Account</option>
              <option value="robot">Robot</option>
              <option value="api_key">API Key</option>
            </select>
          </div>
          <div>
            <label className="text-xs text-gray-400 block mb-1">Owner (optional)</label>
            <input
              className="input text-sm py-1.5"
              placeholder="e.g. platform-team"
              value={owner}
              onChange={(e) => setOwner(e.target.value)}
            />
          </div>
        </div>

        {err && (
          <div className="p-3 rounded border border-red-900/50 bg-red-900/10">
            <p className="text-xs text-red-400">{err}</p>
          </div>
        )}

        <div className="flex gap-2 justify-end pt-2">
          <button className="btn-secondary text-xs px-4 py-2" onClick={onClose}>Cancel</button>
          <button className="btn-primary text-xs px-4 py-2" onClick={submit} disabled={submitting}>
            {submitting ? "Registering…" : "Register Agent"}
          </button>
        </div>
      </div>
    </div>
  )
}
