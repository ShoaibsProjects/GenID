"use client"

import { useState, useEffect } from "react"
import { fetchAgents, Agent, getAuthToken } from "@/lib/api"

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)
  const [killing, setKilling] = useState<string | null>(null)
  const [showModal, setShowModal] = useState(false)

  useEffect(() => {
    fetchAgents()
      .then((d) => setAgents(d.agents || []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const handleKillSwitch = async (agentId: string, name: string) => {
    if (!confirm("Kill " + name + "?")) return
    setKilling(agentId)
    try {
      const headers: Record<string, string> = {}
      const token = getAuthToken()
      if (token) headers["Authorization"] = "Bearer " + token
      await fetch("/api/v1/agents/" + agentId + "/kill-switch", {
        method: "POST",
        headers,
      })
      setAgents(prev => prev.map(a => a.id === agentId ? { ...a, status: "revoked" } : a))
    } catch {
      alert("Kill switch failed")
    } finally {
      setKilling(null)
    }
  }

  const onAgentRegistered = (agent: Agent) => {
    setAgents(prev => [agent, ...prev])
    setShowModal(false)
  }

  return (
    <div>
      <div>
        <h1>AI Agents & NHI</h1>
        <p>{agents.length} registered</p>
    </div>
      <button onClick={() => setShowModal(true)}>+ Register Agent</button>
      <div>
        {loading ? (
          <p>Loading</p>
        ) : agents.length === 0 ? (
          <p>No agents registered</p>
        ) : (
          <ul>
            {agents.map((a) => (
              <li key={a.id}>
                {a.name} ({a.status}) - <button onClick={() => handleKillSwitch(a.id, a.name)} disabled={killing === a.id}>Kill</button>
          </li>
            ))}
       </ul>
        )}
   </div>
      {showModal && (
        <RegisterAgentModal
          onClose={() => setShowModal(false)}
          onCreated={onAgentRegistered}
        />
      )}
 </div>
  )
}

function RegisterAgentModal({ onClose, onCreated }: { onClose: () => void; onCreated: (a: Agent) => void }) {
  const [name, setName] = useState("")
  const [type, setType] = useState("service_account")
  const [submitting, setSubmitting] = useState(false)

  const submit = async () => {
    if (!name.trim()) return
    setSubmitting(true)
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json" }
      const token = getAuthToken()
      if (token) headers["Authorization"] = "Bearer " + token
      const res = await fetch("/api/v1/agents", {
        method: "POST",
        headers,
        body: JSON.stringify({ name, type }),
      })
      const agent = await res.json()
      onCreated(agent.agent || agent)
    } catch {
      alert("failed")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div onClick={onClose}>
      <div onClick={(e) => e.stopPropagation()}>
        <h2>Register AI Agent</h2>
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="name" />
        <select value={type} onChange={(e) => setType(e.target.value)}>
          <option value="service_account">Service Account</option>
          <option value="ai_agent">AI Agent</option>
       </select>
        <button onClick={onClose}>Cancel</button>
        <button onClick={submit} disabled={submitting}>{submitting ? "..." : "Register"}</button>
  </div>
 </div>
  )
}
