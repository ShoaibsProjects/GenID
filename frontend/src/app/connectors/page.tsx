"use client"

import { useState, useEffect, useCallback } from "react"
import { fetchConnectors, syncConnector } from "@/lib/api"

// ─── Types ──────────────────────────────────────────────────

interface Connector {
  id: string
  tenant_id: string
  name: string
  type: string
  status: string
  last_sync_at?: string
  last_error?: string
  created_at: string
  updated_at: string
}

const CONNECTOR_TYPES: Record<string, string> = {
  entra_id: "Microsoft Entra ID",
  ldap: "LDAP",
  active_directory: "Active Directory",
  scim: "SCIM 2.0",
  okta: "Okta",
  aws_iam: "AWS IAM",
  gcp_iam: "GCP IAM",
  csv: "CSV Import",
  generic: "Generic",
}

const STATUS_COLORS: Record<string, string> = {
  connected: "text-emerald-400 bg-emerald-500/10 border-emerald-500/30",
  disconnected: "text-gray-400 bg-gray-500/10 border-gray-500/30",
  error: "text-red-400 bg-red-500/10 border-red-500/30",
  syncing: "text-amber-400 bg-amber-500/10 border-amber-500/30",
  degraded: "text-yellow-400 bg-yellow-500/10 border-yellow-500/30",
}

const TYPE_ICONS: Record<string, string> = {
  entra_id: "☁️",
  active_directory: "🪟",
  ldap: "🌐",
  scim: "🔗",
  okta: "🟦",
  aws_iam: "🟧",
  gcp_iam: "🟨",
  csv: "📄",
  generic: "🔌",
}

// ─── Main Page ──────────────────────────────────────────────

export default function ConnectorsPage() {
  const [connectors, setConnectors] = useState<Connector[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [syncing, setSyncing] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await fetchConnectors()
      setConnectors(data.connectors || [])
      setError("")
    } catch (e: any) {
      setError(e.message || "Failed to load connectors")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  async function handleSync(id: string, name: string) {
    setSyncing(id)
    try {
      const res = await syncConnector(id)
      console.log(`sync response for ${name}:`, res)
    } catch (e: any) {
      console.error(`sync failed for ${name}:`, e.message)
    } finally {
      setSyncing(null)
    }
  }

  const connectedCount = connectors.filter(c => c.status === "connected").length

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Directory Connectors</h1>
          <p className="text-sm text-gray-400 mt-1">
            {connectors.length} configured · {connectedCount} connected
          </p>
        </div>
        <button className="btn-secondary text-xs px-3 py-1.5" onClick={load}>Refresh</button>
      </div>

      {/* Responsive grid of connector cards */}
      {loading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {[1, 2, 3].map(i => <div key={i} className="skeleton h-40 rounded-xl" />)}
        </div>
      ) : error ? (
        <div className="glass-card p-12 text-center text-red-400">{error}</div>
      ) : connectors.length === 0 ? (
        <div className="glass-card p-12 text-center text-gray-500">
          <p className="mb-2">No directories configured</p>
          <p className="text-xs text-gray-600">Connect Entra ID, LDAP, SCIM, or import a CSV to start syncing identities</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {connectors.map((c) => (
            <div
              key={c.id}
              className="rounded-xl border border-zinc-800 bg-zinc-900/50 backdrop-blur-sm p-5 transition-all duration-300 hover:border-amber-500/30 hover:shadow-[0_0_24px_rgba(245,158,11,0.06)]"
            >
              {/* Card header */}
              <div className="flex items-start justify-between mb-4">
                <div className="flex items-center gap-3">
                  <span className="w-10 h-10 rounded-lg bg-white/[0.04] border border-zinc-800 flex items-center justify-center text-lg">
                    {TYPE_ICONS[c.type] || "🔌"}
                  </span>
                  <div>
                    <h3 className="text-sm font-semibold text-white leading-tight">{c.name}</h3>
                    <p className="text-xs text-gray-500 mt-0.5">{CONNECTOR_TYPES[c.type] || c.type}</p>
                  </div>
                </div>
                <span className={`px-2 py-0.5 rounded-full text-xs border ${STATUS_COLORS[c.status] || STATUS_COLORS.disconnected}`}>
                  {c.status}
                </span>
              </div>

              {/* Meta */}
              <div className="flex items-center gap-4 text-xs text-gray-500 mb-4">
                <span>Last sync: {c.last_sync_at ? new Date(c.last_sync_at).toLocaleDateString() : "Never"}</span>
                {c.last_error && (
                  <span className="text-red-400 truncate max-w-[160px]" title={c.last_error}>{c.last_error}</span>
                )}
              </div>

              {/* Actions */}
              <div className="flex gap-2">
                <button
                  className="btn-primary text-xs px-3 py-1.5 flex-1"
                  disabled={syncing === c.id}
                  onClick={() => handleSync(c.id, c.name)}
                >
                  {syncing === c.id ? "Syncing…" : "⟳ Sync Now"}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
