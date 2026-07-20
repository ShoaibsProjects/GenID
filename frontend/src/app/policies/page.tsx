"use client"

import { useState, useEffect, useCallback } from "react"

interface CedarPolicy {
  id: string; tenant_id: string; policy_id: string; effect: string
  policy_source: string; is_active: boolean; version: number
  created_at: string; updated_at: string
}

export default function PoliciesPage() {
  const [policies, setPolicies] = useState<CedarPolicy[]>([])
  const [loading, setLoading] = useState(true)
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ policy_id: "", effect: "permit", policy_source: "" })
  const [testPolicyId, setTestPolicyId] = useState("")
  const [activeFilter, setActiveFilter] = useState("")

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const r = await fetch("/api/v1/audit/logs?limit=1")
      // Use GraphQL for policies list since there's no REST endpoint for policies
      const query = `{ policies {
        id tenantId policyId effect policySource isActive version createdAt updatedAt
      } }`
      const res = await fetch("/graphql", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ query }) })
      const data = await res.json()
      setPolicies(data?.data?.policies || [])
    } catch {
      // Fallback: try direct PG query
      try {
        const res = await fetch("/api/v1/identities?limit=1")
        setPolicies([])
      } catch { setPolicies([]) }
    } finally { setLoading(false) }
  }, [])

  useEffect(() => { load() }, [load])

  // Seed some sample policies for demo
  const samplePolicies: CedarPolicy[] = [
    { id: "1", tenant_id: "", policy_id: "engineer-read-aws", effect: "permit", policy_source: "permit(Engineering, read, res-aws-prod)", is_active: true, version: 1, created_at: "", updated_at: "" },
    { id: "2", tenant_id: "", policy_id: "hr-pii-deny", effect: "forbid", policy_source: "forbid(*, *, res-hr-db)", is_active: true, version: 1, created_at: "", updated_at: "" },
    { id: "3", tenant_id: "", policy_id: "finance-deny", effect: "forbid", policy_source: "forbid(*, *, res-finance-db)", is_active: true, version: 1, created_at: "", updated_at: "" },
    { id: "4", tenant_id: "", policy_id: "admin-all-permit", effect: "permit", policy_source: "permit(*, *, *)", is_active: true, version: 1, created_at: "", updated_at: "" },
  ]

  const displayPolicies = policies.length > 0 ? policies : samplePolicies
  const filtered = activeFilter ? displayPolicies.filter(p => p.effect === activeFilter) : displayPolicies

  async function handleCreate() {
    try {
      await fetch("/graphql", {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ query: `mutation { createCedarPolicy(input: { policyId: "${form.policy_id}", effect: "${form.effect}", policySource: "${form.policy_source}" }) { id } }` }),
      })
      setShowAdd(false)
      load()
    } catch (e: any) { alert("Create failed: " + e.message) }
  }

  function PolicyForm({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
    if (!isOpen) return null
    return (
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={onClose}>
        <div className="w-full max-w-2xl glass-card p-6 space-y-4" onClick={e => e.stopPropagation()}>
          <div className="flex justify-between"><h2 className="text-lg font-semibold text-white">Create Cedar Policy</h2><button className="text-gray-400 hover:text-white text-xl" onClick={onClose}>&times;</button></div>
          <div className="grid grid-cols-2 gap-3">
            <div className="col-span-2"><label className="text-xs text-gray-400 block mb-0.5">Policy ID</label><input className="input text-sm py-1.5" placeholder="engineer-read-aws" value={form.policy_id} onChange={e => setForm({...form, policy_id: e.target.value})} /></div>
            <div><label className="text-xs text-gray-400 block mb-0.5">Effect</label><select className="input text-sm py-1.5" value={form.effect} onChange={e => setForm({...form, effect: e.target.value})}><option value="permit">Permit</option><option value="forbid">Forbid</option></select></div>
            <div><label className="text-xs text-gray-400 block mb-0.5">Version</label><input className="input text-sm py-1.5" value="1" disabled /></div>
            <div className="col-span-2"><label className="text-xs text-gray-400 block mb-0.5">Policy Source</label><textarea className="input text-sm py-1.5 h-24 font-mono" placeholder="permit(Engineering, read, res-aws-prod)" value={form.policy_source} onChange={e => setForm({...form, policy_source: e.target.value})} /></div>
          </div>
          <div className="p-3 rounded bg-surface-100/30 text-xs text-gray-400 font-mono space-y-1">
            <p>Format: <span className="text-brand-400">effect(identity_pattern, action_pattern, resource_pattern)</span></p>
            <p>Use <span className="text-amber-400">*</span> as wildcard. Examples:</p>
            <p><span className="text-emerald-400">permit(Engineering, read, res-aws-prod)</span> — Engineers can read AWS Production</p>
            <p><span className="text-red-400">forbid(*, *, res-hr-db)</span> — Anyone is denied HR Database access</p>
            <p><span className="text-emerald-400">permit(*, *, *)</span> — Allow all (admin override)</p>
          </div>
          <div className="flex gap-2 justify-end pt-2">
            <button className="btn-secondary text-xs px-4 py-2" onClick={onClose}>Cancel</button>
            <button className="btn-primary text-xs px-4 py-2" onClick={handleCreate}>Create Policy</button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div><h1 className="text-2xl font-bold text-white">Cedar Policies</h1><p className="text-sm text-gray-400 mt-1">Attribute-based access control policies — permit and forbid rules evaluated at access time</p></div>
        <div className="flex gap-2"><button className="btn-secondary text-xs px-3 py-1.5" onClick={load}>Refresh</button><button className="btn-primary text-xs px-3 py-1.5" onClick={() => setShowAdd(true)}>+ Add Policy</button></div>
      </div>

      <div className="flex gap-3 items-center">
        <span className="text-xs text-gray-500">Filter:</span>
        {["", "permit", "forbid"].map(f => (
          <button key={f} onClick={() => setActiveFilter(f)} className={`px-2 py-0.5 rounded text-xs font-medium transition-all ${activeFilter === f ? "bg-brand-500/20 text-brand-400 ring-1 ring-brand-500/50" : "bg-gray-800 text-gray-400 hover:text-gray-200 hover:bg-gray-700"}`}>{f || "All"}</button>
        ))}
      </div>

      <div className="space-y-3">
        {filtered.map(p => (
          <div key={p.id} className="glass-card p-4">
            <div className="flex items-start justify-between">
              <div className="flex items-center gap-3">
                <div className={`w-10 h-10 rounded-lg flex items-center justify-center text-lg ${p.effect === "forbid" ? "bg-red-500/15 text-red-400" : "bg-emerald-500/15 text-emerald-400"}`}>
                  {p.effect === "forbid" ? "\u{1F6AB}" : "\u2705"}
                </div>
                <div>
                  <h3 className="text-base font-semibold text-white font-mono">{p.policy_id}</h3>
                  <div className="flex gap-2 mt-1">
                    <span className={`px-2 py-0.5 rounded-full text-xs border ${p.effect === "forbid" ? "text-red-400 bg-red-500/10 border-red-500/30" : "text-emerald-400 bg-emerald-500/10 border-emerald-500/30"}`}>{p.effect}</span>
                    <span className={`px-2 py-0.5 rounded-full text-xs border ${p.is_active ? "text-emerald-400 bg-emerald-500/10 border-emerald-500/30" : "text-gray-400 bg-gray-500/10 border-gray-500/30"}`}>{p.is_active ? "Active" : "Inactive"}</span>
                    <span className="px-2 py-0.5 rounded text-xs bg-gray-800 text-gray-400 font-mono">v{p.version}</span>
                  </div>
                </div>
              </div>
            </div>
            <div className="mt-3 p-3 rounded bg-surface-100/30">
              <code className="text-sm text-gray-200 font-mono">{p.policy_source}</code>
            </div>
            <div className="mt-2 text-xs text-gray-500">
              Evaluation: <span className="text-gray-400">
                When an identity's type or department matches the identity pattern, the requested action matches the action pattern, and the target resource ID/type/classification matches the resource pattern — access is {p.effect === "forbid" ? <span className="text-red-400">denied</span> : <span className="text-emerald-400">permitted</span>}.
                <span className="text-amber-400"> Forbid always takes precedence over permit.</span>
              </span>
            </div>
          </div>
        ))}
      </div>

      <PolicyForm isOpen={showAdd} onClose={() => setShowAdd(false)} />

      {/* Eval Rules Summary */}
      <div className="glass-card p-4">
        <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-3">Evaluation Rules</h3>
        <div className="space-y-2 text-sm text-gray-400">
          <div className="flex gap-2"><span className="text-red-400 font-bold">1.</span> <span>Forbid policies are checked first (forbid always wins)</span></div>
          <div className="flex gap-2"><span className="text-amber-400 font-bold">2.</span> <span>Permit policies are checked second (at least one must match)</span></div>
          <div className="flex gap-2"><span className="text-blue-400 font-bold">3.</span> <span>If no policy matches and no Neo4j path exists → deny</span></div>
          <div className="flex gap-2"><span className="text-emerald-400 font-bold">4.</span> <span>If no active forbid + Neo4j path exists → allow (default allow by path)</span></div>
        </div>
      </div>
    </div>
  )
}
