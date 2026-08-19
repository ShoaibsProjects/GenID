"use client"
import { useState, useEffect } from "react"
import Link from "next/link"
import { authFetch } from "@/lib/api"
import { useEventStream } from "@/hooks/use-event-stream"
import { FirecallCard } from "@/components/FirecallCard"
import { Badge } from "@/components/ui/Badge"
import { ArrowRight, KeyRound, UserPlus, FilePlus, Zap, ShieldAlert, Radio, Users } from "lucide-react"

export default function DashboardPage() {
  const [identityStats, setIdentityStats] = useState<any>(null)
  const [pendingApprovals, setPendingApprovals] = useState<number>(0)
  const [activeFirecall, setActiveFirecall] = useState<number>(0)
  const [policyViolations, setPolicyViolations] = useState<number>(0)
  const [health, setHealth] = useState<any>(null)
  const [loading, setLoading] = useState(true)
  const { events, live } = useEventStream({ limit: 12 })

  useEffect(() => {
    async function load() {
      try {
        const [conn, audit, h, reqs] = await Promise.all([
          authFetch("/api/v1/connectors/stats").then(r => r.json()).catch(() => null),
          authFetch("/api/v1/audit/stats").then(r => r.json()).catch(() => null),
          fetch("/healthz").then(r => r.json()).catch(() => null),
          authFetch("/api/v1/requests?status=pending&limit=1").then(r => r.json()).catch(() => null),
        ])
        const idRes = await authFetch("/api/v1/identities?limit=1").then(r => r.json()).catch(() => null)
        setIdentityStats(idRes)
        setActiveFirecall(audit?.active_jit ?? 0)
        setPolicyViolations(audit?.critical_revocations ?? 0)
        setPendingApprovals(reqs?.total ?? 0)
        setHealth(h)
      } catch { } finally { setLoading(false) }
    }
    load()
    const interval = setInterval(load, 15000)
    return () => clearInterval(interval)
  }, [])

  const services = health?.checks ? Object.entries(health.checks).map(([name, status]) => ({ name, ok: status === "ok" })) : []

  const kpis = [
    { label: "Total Identities", value: identityStats?.total ?? "—", color: "#FBBF24", sub: "People & machines", href: "/identities" },
    { label: "Pending Approvals", value: pendingApprovals, color: "#A78BFA", sub: "Awaiting decision", href: "/inbox" },
    { label: "Active JIT / Firecall", value: activeFirecall, color: "#34D399", sub: "Time-bounded access", href: "/access" },
    { label: "Policy Violations", value: policyViolations, color: "#EF4444", sub: "Last 24 hours", href: "/risk" },
  ]

  const quickActions = [
    { label: "Request Access", desc: "JIT, policy-evaluated", icon: KeyRound, href: "/access", color: "#FBBF24" },
    { label: "Register Identity", desc: "Person or NHI", icon: UserPlus, href: "/identities", color: "#60A5FA" },
    { label: "New Policy", desc: "Author & enforce", icon: FilePlus, href: "/policies", color: "#A78BFA" },
    { label: "Open Simulator", desc: "Dry-run policy", icon: Zap, href: "/policies/simulator", color: "#34D399" },
  ]

  return (
    <div className="max-w-[1400px] mx-auto space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between flex-wrap gap-4">
        <div>
          <h1 className="text-[28px] font-bold tracking-tight text-gradient-accent">
            Dashboard
          </h1>
          <p className="text-[13px] text-[var(--text-secondary)]">
            Identity Fabric overview —{" "}
            <span className="text-emerald-400">all systems nominal</span>
          </p>
        </div>
        <div className="flex items-center gap-2 text-xs">
          {live ? (
            <Badge className="border-emerald-500/40 bg-emerald-500/10 text-emerald-400">
              <Radio className="w-3 h-3 mr-1" /> Live
            </Badge>
          ) : (
            <Badge className="border-amber-500/40 bg-amber-500/10 text-amber-400">Polling</Badge>
          )}
          <span className="text-[var(--text-muted)]">
            {events.length > 0 ? `${events.length} recent events` : "no events yet"}
          </span>
        </div>
      </div>

      {loading ? (
        <div className="grid grid-cols-4 gap-4">
          {[1,2,3,4].map(i => <div key={i} className="skeleton h-[120px] rounded-2xl" />)}
        </div>
      ) : (
        <>
          {/* KPI Cards */}
          <div className="grid grid-cols-4 gap-4 max-md:grid-cols-2 max-sm:grid-cols-1">
            {kpis.map((card, i) => (
              <Link key={card.label} href={card.href} className="group">
                <div className="stat-card animate-slide-in" style={{ animationDelay: `${i * 0.08}s` }}>
                  <div style={{ position: 'absolute', top: 0, left: 0, right: 0, height: 3, background: `linear-gradient(90deg, ${card.color}30, ${card.color}40, transparent)`, borderTopLeftRadius: 16, borderTopRightRadius: 16 }} />
                  <div style={{ position: 'absolute', top: 20, right: 20, width: 40, height: 40, borderRadius: '50%', background: `${card.color}18`, filter: 'blur(20px)', pointerEvents: 'none' }} />
                  <div style={{ position: 'relative', zIndex: 1 }}>
                    <div className="flex items-center justify-between">
                      <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.08em', color: '#5C5C62', marginBottom: 8 }}>{card.label}</p>
                      <ArrowRight className="w-4 h-4 text-[var(--text-muted)] opacity-0 group-hover:opacity-100 transition-opacity" />
                    </div>
                    <p style={{ fontSize: 36, fontWeight: 700, color: card.color, letterSpacing: '-0.03em', marginBottom: 4 }}>{card.value}</p>
                    <p style={{ fontSize: 12, color: '#5C5C62' }}>{card.sub}</p>
                  </div>
                </div>
              </Link>
            ))}
          </div>

          {/* Quick Actions */}
          <div className="grid grid-cols-4 gap-4 max-md:grid-cols-2">
            {quickActions.map((qa) => (
              <Link
                key={qa.label}
                href={qa.href}
                className="glass-card p-4 flex items-center gap-3 hover:border-[var(--accent)]/40 hover:bg-[var(--glass-2)] transition-all duration-200 group"
              >
                <div className="w-10 h-10 rounded-xl flex items-center justify-center" style={{ background: `${qa.color}15`, border: `1px solid ${qa.color}30`, color: qa.color }}>
                  <qa.icon className="w-5 h-5" />
                </div>
                <div className="flex-1">
                  <p className="text-sm font-semibold text-[var(--text-primary)] group-hover:text-[var(--accent)] transition-colors">{qa.label}</p>
                  <p className="text-xs text-[var(--text-muted)]">{qa.desc}</p>
                </div>
              </Link>
            ))}
          </div>

          {/* System Health + Live Activity */}
          <div className="grid grid-cols-5 gap-4">
            {/* System Health */}
            <div className="glass-card p-6 col-span-3">
              <h3 className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)] mb-4">System Health</h3>
              <div className="flex gap-4 flex-wrap">
                {services.map(s => (
                  <div key={s.name} style={{ display: 'flex', alignItems: 'center', gap: 10, padding: '10px 16px', borderRadius: 10, background: s.ok ? 'rgba(52,211,153,0.04)' : 'rgba(239,68,68,0.04)', border: `1px solid ${s.ok ? 'rgba(52,211,153,0.10)' : 'rgba(239,68,68,0.10)'}` }}>
                    <span style={{ width: 8, height: 8, borderRadius: '50%', background: s.ok ? '#34D399' : '#EF4444', boxShadow: s.ok ? '0 0 8px rgba(52,211,153,0.4)' : '0 0 8px rgba(239,68,68,0.4)' }} />
                    <span style={{ fontSize: 13, color: '#F0EFEC', textTransform: 'capitalize' }}>{s.name === "neo4j" ? "Neo4j" : s.name === "postgres" ? "PostgreSQL" : s.name}</span>
                    <span style={{ fontSize: 11, color: s.ok ? '#34D399' : '#EF4444', fontWeight: 600, letterSpacing: '0.05em' }}>{s.ok ? "ACTIVE" : "DOWN"}</span>
                  </div>
                ))}
                {services.length === 0 && <span className="text-[13px] text-[var(--text-muted)]">Health check unavailable</span>}
              </div>
            </div>

            {/* Live Activity Feed */}
            <div className="glass-card p-6 col-span-2">
              <div className="flex items-center justify-between mb-4">
                <h3 className="text-[11px] font-bold uppercase tracking-wider text-[var(--text-muted)]">Activity Feed</h3>
                {live && (
                  <span className="flex items-center gap-1.5 text-[10px] text-emerald-400">
                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" /> SSE
                  </span>
                )}
              </div>
              {events.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-8 text-[var(--text-muted)]">
                  <ShieldAlert className="w-6 h-6 mb-2 opacity-50" />
                  <p className="text-xs">No events streamed yet.</p>
                </div>
              ) : (
                <div className="space-y-2 max-h-64 overflow-y-auto">
                  {events.map((ev, i) => (
                    <div key={ev.id ?? i} className="flex items-start gap-2 p-2 rounded-lg bg-[var(--glass-2)]">
                      <span className={`mt-1.5 w-1.5 h-1.5 rounded-full flex-shrink-0 ${severityColor(ev.level || ev.severity)}`} />
                      <div className="min-w-0">
                        <p className="text-xs text-[var(--text-primary)] truncate">{ev.summary || ev.type}</p>
                        <p className="text-[10px] text-[var(--text-muted)] font-mono">
                          {ev.type}
                          {ev.timestamp ? ` · ${new Date(ev.timestamp).toLocaleTimeString()}` : ""}
                        </p>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Firecall (Break-Glass) Emergency Access */}
          <FirecallCard />
        </>
      )}
    </div>
  )
}

function severityColor(level?: string): string {
  switch ((level || "").toLowerCase()) {
    case "error": case "critical": case "fatal": return "bg-red-500"
    case "warn": case "warning": return "bg-amber-400"
    case "info": return "bg-sky-400"
    default: return "bg-emerald-400"
  }
}