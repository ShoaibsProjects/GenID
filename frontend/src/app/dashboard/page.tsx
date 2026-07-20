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

  const services = health?.checks ? Object.entries(health.checks).map(([name, status]) => ({ name, ok: status === "ok" })) : []

  const statCards = [
    { label: "Identities", value: identityStats?.total ?? "—", color: "#FBBF24", accent: "rgba(245,158,11,0.15)", sub: `Identity Fabric` },
    { label: "Connectors", value: connectorStats?.total_connectors ?? "—", color: "#60A5FA", accent: "rgba(59,130,246,0.15)", sub: `${connectorStats?.connected_count ?? 0} active` },
    { label: "Synced Users", value: connectorStats?.total_identities ?? "—", color: "#34D399", accent: "rgba(52,211,153,0.12)", sub: `from ${connectorStats?.total_groups ?? 0} groups` },
    { label: "Audit Events", value: auditStats?.total ?? "—", color: "#A78BFA", accent: "rgba(167,139,250,0.12)", sub: `buffer ${auditStats?.usage_pct ? Math.round(auditStats.usage_pct) + '%' : '—'}` },
  ]

  return (
    <div style={{ maxWidth: 1400, margin: '0 auto' }}>
      {/* Header */}
      <div style={{ marginBottom: 32 }}>
        <h1 style={{ fontSize: 28, fontWeight: 700, letterSpacing: '-0.02em', background: 'linear-gradient(135deg, #F0EFEC 30%, #A1A1AA)', WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent', marginBottom: 4 }}>
          Dashboard
        </h1>
        <p style={{ fontSize: 13, color: '#5C5C62', lineHeight: 1.5 }}>
          Identity Fabric overview — <span style={{ color: '#34D399' }}>all systems nominal</span>
        </p>
      </div>

      {loading ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16 }}>
          {[1,2,3,4].map(i => <div key={i} className="skeleton" style={{ height: 120, borderRadius: 16 }} />)}
        </div>
      ) : (
        <>
          {/* Stat Cards */}
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 16, marginBottom: 32 }}>
            {statCards.map((card, i) => (
              <div key={card.label} className="stat-card animate-slide-in" style={{ animationDelay: `${i * 0.08}s` }}>
                {/* Colored top accent */}
                <div style={{ position: 'absolute', top: 0, left: 0, right: 0, height: 3, background: `linear-gradient(90deg, ${card.accent}, ${card.color}40, transparent)`, borderTopLeftRadius: 16, borderTopRightRadius: 16 }} />
                {/* Ambient glow dot */}
                <div style={{ position: 'absolute', top: 20, right: 20, width: 40, height: 40, borderRadius: '50%', background: card.accent, filter: 'blur(20px)', pointerEvents: 'none' }} />
                <div style={{ position: 'relative', zIndex: 1 }}>
                  <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.08em', color: '#5C5C62', marginBottom: 8 }}>{card.label}</p>
                  <p style={{ fontSize: 36, fontWeight: 700, color: card.color, letterSpacing: '-0.03em', marginBottom: 4 }}>{card.value}</p>
                  <p style={{ fontSize: 12, color: '#5C5C62' }}>{card.sub}</p>
                </div>
              </div>
            ))}
          </div>

          {/* Service Health + System */}
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, marginBottom: 32 }}>
            {/* Service Health */}
            <div className="glass-card" style={{ padding: 24 }}>
              <h3 style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.08em', color: '#5C5C62', marginBottom: 16 }}>System Health</h3>
              <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
                {services.map(s => (
                  <div key={s.name} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '10px 16px', borderRadius: 10, background: s.ok ? 'rgba(52,211,153,0.04)' : 'rgba(239,68,68,0.04)', border: `1px solid ${s.ok ? 'rgba(52,211,153,0.10)' : 'rgba(239,68,68,0.10)'}` }}>
                    <span style={{ width: 8, height: 8, borderRadius: '50%', background: s.ok ? '#34D399' : '#EF4444', boxShadow: s.ok ? '0 0 8px rgba(52,211,153,0.4)' : '0 0 8px rgba(239,68,68,0.4)' }} />
                    <span style={{ fontSize: 13, color: '#F0EFEC', textTransform: 'capitalize' }}>{s.name === "neo4j" ? "Neo4j" : s.name === "postgres" ? "PostgreSQL" : s.name}</span>
                    <span style={{ fontSize: 11, color: s.ok ? '#34D399' : '#EF4444', fontWeight: 600, letterSpacing: '0.05em' }}>{s.ok ? "ACTIVE" : "DOWN"}</span>
                  </div>
                ))}
                {services.length === 0 && <span style={{ fontSize: 13, color: '#5C5C62' }}>Health check unavailable</span>}
              </div>
            </div>

            {/* System Architecture */}
            <div className="glass-card" style={{ padding: 24 }}>
              <h3 style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.08em', color: '#5C5C62', marginBottom: 16 }}>Data Layer</h3>
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                {[
                  ["PostgreSQL", "ACID Identity Store", "#60A5FA"],
                  ["Neo4j 5", "Identity Graph", "#34D399"],
                  ["Redis 7", "Cache Layer", "#FBBF24"],
                  ["Temporal", "Workflow Engine", "#A78BFA"],
                  ["OTel", "Distributed Tracing", "#F472B6"],
                  ["Grafana", "Visualization", "#F59E0B"],
                ].map(([name, desc, color]) => (
                  <div key={name as string} style={{ padding: '10px 14px', borderRadius: 8, background: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.04)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 2 }}>
                      <span style={{ width: 6, height: 6, borderRadius: '50%', background: color as string, boxShadow: `0 0 6px ${color}` }} />
                      <span style={{ fontSize: 13, fontWeight: 600, color: '#F0EFEC' }}>{name as string}</span>
                    </div>
                    <span style={{ fontSize: 11, color: '#5C5C62', marginLeft: 14 }}>{desc as string}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>

          {/* Architecture Map */}
          <div className="glass-card" style={{ padding: 24, position: 'relative', overflow: 'hidden' }}>
            <div style={{ position: 'absolute', top: 0, left: 0, right: 0, height: 1, background: 'linear-gradient(90deg, transparent, rgba(245,158,11,0.10), transparent)', pointerEvents: 'none' }} />
            <h3 style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.08em', color: '#5C5C62', marginBottom: 20 }}>Architecture</h3>
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12, fontSize: 12 }}>
              {[
                ["HTTP API", "gorilla/mux", "30+ handlers", 0, "#FBBF24"],
                ["Workflows", "Temporal", "9 workflows", 1, "#A78BFA"],
                ["Graph Query", "Cypher", "entities + paths", 2, "#34D399"],
                ["Frontend", "Next.js + TS", "14 pages", 3, "#60A5FA"],
                ["PostgreSQL", "identities, audit", "ACID + GIN indexes", 0, "#60A5FA"],
                ["Neo4j 5", "Resources, Roles", "HAS_ROLE *1..N", 1, "#34D399"],
                ["Redis 7", "Caches + TTLs", "30s decision cache", 2, "#FBBF24"],
                ["Temporal", "Durable Execution", "500 concurrent", 3, "#A78BFA"],
              ].map(([title, subtitle, desc, col]) => (
                <div key={title as string} style={{ padding: '12px 14px', borderRadius: 10, background: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.04)' }}>
                  <p style={{ fontSize: 13, fontWeight: 600, color: '#F0EFEC', marginBottom: 2 }}>{title as string}</p>
                  <p style={{ fontSize: 11, color: '#9C9CA0', marginBottom: 4 }}>{subtitle as string}</p>
                  <p style={{ fontSize: 10, color: '#5C5C62' }}>{desc as string}</p>
                </div>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  )
}
