"use client"

import { useEffect, useState } from "react"
import { authFetch } from "@/lib/api"

interface JITSession {
  identity_id: string
  identity_name: string
  resource_id: string
  resource_name: string
  expires_at: string
}

export default function JITSessions() {
  const [sessions, setSessions] = useState<JITSession[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")

  async function load() {
    setLoading(true)
    try {
      const res = await authFetch("/api/v1/access/sessions")
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data = await res.json()
      setSessions(data.sessions || [])
      setError("")
    } catch {
      setError("Failed to load sessions")
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    const interval = setInterval(load, 15000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-900/50 backdrop-blur-sm p-5">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-sm font-semibold text-white">Active JIT Sessions</h3>
        <button
          className="text-xs px-2 py-1 rounded-lg border border-zinc-700 text-gray-400 hover:text-white hover:border-zinc-600"
          onClick={load}
        >
          Refresh
        </button>
      </div>

      {loading && sessions.length === 0 ? (
        <div className="text-center text-gray-500 text-sm py-8">Loading sessions…</div>
      ) : error ? (
        <div className="text-center text-red-400 text-sm py-4">{error}</div>
      ) : sessions.length === 0 ? (
        <div className="text-center text-gray-500 text-sm py-8">
          No active JIT sessions
        </div>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-zinc-800">
              <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">
                Identity
              </th>
              <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">
                Resource
              </th>
              <th className="text-left py-2.5 px-3 text-xs font-medium text-gray-500 uppercase">
                Expires At
              </th>
            </tr>
          </thead>
          <tbody>
            {sessions.map((s) => (
              <tr key={`${s.identity_id}-${s.resource_id}`} className="border-b border-zinc-800/50 last:border-0">
                <td className="py-2.5 px-3 text-white">
                  {s.identity_name || s.identity_id.slice(0, 8)}
                </td>
                <td className="py-2.5 px-3 text-white">
                  {s.resource_name || s.resource_id.slice(0, 8)}
                </td>
                <td className="py-2.5 px-3">
                  <span className="px-2 py-0.5 rounded-full text-xs border text-amber-400 bg-amber-500/10 border-amber-500/30">
                    {new Date(s.expires_at).toLocaleString()}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}