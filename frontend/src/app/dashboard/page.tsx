"use client"
import { useState, useEffect } from "react"

export default function DashboardPage() {
  const [identityStats, setIdentityStats] = useState<any>(null)
  const [connectorStats, setConnectorStats] = useState<any>(null)
  const [auditStats, setAuditStats] = useState<any>(null)
  const [health, setHealth] = useState<any>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    async function load() {
      try {
        const [conn, audit, h] = await Promise.all([
          fetch("/api/v1/connectors/stats").then(r => r.json()).catch(() => null),
          fetch("/api/v1/audit/stats").then(r => r.json()).catch(() => null),
          fetch("/healthz").then(r => r.json()).catch(() => null),
        ])
        // Identity stats from list endpoint
        const idRes = await fetch("/api/v1/identities?limit=1").then(r => r.json()).catch(() => null)
        setIdentityStats(idRes)
        setConnectorStats(conn)
        setAuditStats(audit)
        setHealth(h)
      } catch { } finally { setLoading(false) }
    }
    load()
    const interval = setInterval(load, 15000)
    return () => clearInterval(interval)
  }, [])

  const metrics = [
    { label: "Identities", value: identityStats?.total ?? "?", color: "text-blue-400", icon: "\u{1F464}" },
    { label: "Connectors", value: connectorStats?.total_connectors ?? "?", color: "text-emerald-400", icon: "\u{1F517}" },
    { label: "Connected", value: connectorStats?.connected_count ?? "?", color: "text-emerald-400", icon: "\u2705" },
    { label: "Errors", value: connectorStats?.error_count ?? "0", color: connectorStats?.error_count > 0 ? "text-red-400" : "text-gray-400", icon: "\u274C" },
    { label: "Synced Users", value: connectorStats?.total_identities ?? "?", color: "text-purple-400", icon: "\u{1F465}" },
    { label: "Synced Groups", value: connectorStats?.total_groups ?? "?", color: "text-amber-400", icon: "\u{1F4E6}" },
    { label: "Audit Entries", value: auditStats?.total ?? "?", color: "text-cyan-400", icon: "\u{1F4CB}" },
    { label: "Buffer Usage", value: auditStats?.usage_pct ? Math.round(auditStats.usage_pct) + "%" : "?", color: auditStats?.usage_pct > 80 ? "text-amber-400" : "text-emerald-400", icon: "\u{1F4BE}" },
  ]

  const services = health?.checks ? Object.entries(health.checks).map(([name, status]) => ({ name, ok: status === "ok" })) : []

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Dashboard</h1>
        <p className="text-sm text-gray-400 mt-1">ObserveID Identity Fabric — System Overview</p>
      </div>

      {loading ? <div className="p-12 text-center text-gray-500">Loading metrics...</div> : (
        <>
          {/* Metrics Grid */}
          <div className="grid grid-cols-4 gap-3">
            {metrics.map(m => (
              <div key={m.label} className="glass-card p-4">
                <div className="flex items-center justify-between mb-2"><span className="text-xs text-gray-500 uppercase tracking-wider">{m.label}</span><span className="text-lg">{m.icon}</span></div>
                <div className={`text-3xl font-bold ${m.color}`}>{m.value}</div>
              </div>
            ))}
          </div>

          {/* Services Health */}
          <div className="glass-card p-4">
            <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-3">Service Health</h3>
            <div className="flex gap-4 flex-wrap">
              {services.map(s => (
                <div key={s.name} className="flex items-center gap-2">
                  <div className={`w-2.5 h-2.5 rounded-full ${s.ok ? "bg-emerald-500" : "bg-red-500"}`} />
                  <span className="text-sm text-gray-300 capitalize">{s.name === "neo4j" ? "Neo4j" : s.name === "postgres" ? "PostgreSQL" : s.name === "temporal" ? "Temporal" : s.name}</span>
                  <span className={`text-xs ${s.ok ? "text-emerald-400" : "text-red-400"}`}>{s.ok ? "OK" : "DOWN"}</span>
                </div>
              ))}
              {services.length === 0 && <span className="text-sm text-gray-500">Health check unavailable</span>}
            </div>
          </div>

          {/* System Architecture */}
          <div className="glass-card p-4">
            <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-3">System Architecture</h3>
            <div className="grid grid-cols-4 gap-3 text-xs">
              {[
                ["Go HTTP API", ":8080", "gorilla/mux + 30+ handlers"],
                ["PostgreSQL", "Source of Truth", "ACID, 17 tables, GIN indexes"],
                ["Neo4j 5", "Identity Graph", "Entities + Relationships + Paths"],
                ["Redis 7", "Cache Layer", "Revocation cache + Rate limiting"],
                ["Temporal", "Workflow Engine", "9 workflows + 25+ activities"],
                ["OTel", "Observability", "Traces + Metrics + Prometheus"],
                ["Grafana", "Dashboards", ":3000 — metrics visualization"],
                ["Entra ID", "Directory Sync", "Users + Groups + Roles + Apps"],
              ].map(([name, tag, desc]) => (
                <div key={name} className="p-3 rounded bg-surface-100/30">
                  <div className="text-gray-200 font-medium mb-1">{name}</div>
                  <div className="text-brand-400 font-mono mb-1">{tag}</div>
                  <div className="text-gray-500">{desc}</div>
                </div>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  )
}
