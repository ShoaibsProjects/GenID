"use client"
import { useState, useEffect, useRef, useCallback } from "react"
import { authFetch } from "@/lib/api"

// ─── Types ──────────────────────────────────────────────────

interface Identity { id: string; display_name: string; email: string; department?: string }

interface EventDef { type: string; weight: number; severity: string; desc: string }

const EVENT_DEFS: EventDef[] = [
  { type: "auth.failed_login", weight: 100, severity: "medium", desc: "Wrong password / unknown user" },
  { type: "auth.mfa_failure", weight: 75, severity: "medium", desc: "MFA challenge failed" },
  { type: "auth.impossible_travel", weight: 150, severity: "high", desc: "Two logins too far apart to be real" },
  { type: "auth.password_spray", weight: 125, severity: "high", desc: "Same password tried across many accounts" },
  { type: "auth.brute_force", weight: 175, severity: "high", desc: "Rapid repeated login attempts" },
  { type: "account.locked", weight: 50, severity: "low", desc: "Account auto-locked" },
  { type: "session.anomalous", weight: 100, severity: "medium", desc: "Session from unusual context" },
  { type: "entitlement.escalation", weight: 200, severity: "high", desc: "User granted privilege unexpectedly" },
  { type: "access.off_hours", weight: 50, severity: "low", desc: "Access outside business hours" },
  { type: "peer_deviation", weight: 80, severity: "medium", desc: "Access diverges from peer group" },
  { type: "dormant_account", weight: 60, severity: "low", desc: "Account unused for extended period" },
  { type: "privilege_escalation", weight: 250, severity: "high", desc: "Privilege boundary crossed" },
  { type: "credential_leaked", weight: 300, severity: "critical", desc: "Password appears in breach data" },
]

const BANDS = [
  { max: 1000, label: "Critical", color: "#EF4444" },
  { max: 800, label: "High", color: "#F97316" },
  { max: 600, label: "Elevated", color: "#F59E0B" },
  { max: 300, label: "Low", color: "#84CC16" },
  { max: 0, label: "Minimal", color: "#34D399" },
]

function bandFor(score: number) {
  return BANDS.find(b => score >= b.max) || BANDS[BANDS.length - 1]
}

function fmt(n?: number | null): string {
  if (n === undefined || n === null) return "—"
  return Number.isInteger(n) ? n.toString() : n.toFixed(1)
}

// ─── Main Page ──────────────────────────────────────────────

export default function EventSimulatorPage() {
  const [identities, setIdentities] = useState<Identity[]>([])
  const [selectedId, setSelectedId] = useState("test-identity-001")
  const [customId, setCustomId] = useState("")
  const [useCustom, setUseCustom] = useState(false)

  const [eventType, setEventType] = useState("auth.failed_login")
  const [metadata, setMetadata] = useState("")

  const [feed, setFeed] = useState<{ time: string; type: string; delta: number; ok: boolean; msg: string }[]>([])
  const [sending, setSending] = useState(false)
  const [liveScore, setLiveScore] = useState<{ score: number | null; band: string | null; staticScore: number | null; dynamicScore: number | null; peerScore: number | null; eventCount: number | null }>({ score: null, band: null, staticScore: null, dynamicScore: null, peerScore: null, eventCount: null })
  const [pulse, setPulse] = useState(0)

  const feedRef = useRef<HTMLDivElement>(null)

  const activeId = useCustom ? customId : selectedId

  useEffect(() => {
    authFetch("/api/v1/identities?limit=100").then(r => r.json()).then(d => {
      const list = (d?.identities || []).map((i: any) => ({ id: i.id, display_name: i.display_name, email: i.email, department: i.department }))
      setIdentities(list)
      if (list.length && !list.find((i: Identity) => i.id === "test-identity-001")) setSelectedId(list[0].id)
    }).catch(() => {})
  }, [])

  useEffect(() => {
    feedRef.current?.scrollTo({ top: feedRef.current.scrollHeight, behavior: "smooth" })
  }, [feed])

  const pollScore = useCallback(async (id: string) => {
    try {
      const res = await authFetch(`/api/v1/risk/score/${id}`)
      if (!res.ok) return
      const d = await res.json()
      setLiveScore({
        score: d.riskScore ?? null,
        band: d.riskBand ?? null,
        staticScore: d.staticScore ?? null,
        dynamicScore: d.dynamicScore ?? null,
        peerScore: d.peerScore ?? null,
        eventCount: d.eventCount ?? null,
      })
    } catch { }
  }, [])

  useEffect(() => {
    if (!activeId) return
    pollScore(activeId)
    const interval = setInterval(() => pollScore(activeId), 3000)
    return () => clearInterval(interval)
  }, [activeId, pollScore])

  const send = async () => {
    if (!activeId) return
    setSending(true)
    const def = EVENT_DEFS.find(e => e.type === eventType)!
    let meta: any = {}
    if (metadata.trim()) {
      try { meta = JSON.parse(metadata) } catch { meta = { note: metadata } }
    }
    const t0 = Date.now()
    try {
      const res = await authFetch("/api/v1/events/ingest", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          event_type: eventType,
          identity_id: activeId,
          source: "simulator",
          severity: def.severity,
          metadata: meta,
        }),
      })
      const latency = Date.now() - t0
      const ok = res.ok
      const body = ok ? {} : await res.json().catch(() => ({}))
      setFeed(f => [...f.slice(-24), { time: new Date().toLocaleTimeString(), type: eventType, delta: def.weight, ok, msg: ok ? `+${def.weight} · ${latency}ms` : (body as any).error || "failed" }])
      setPulse(p => p + 1)
      setTimeout(() => pollScore(activeId), 300)
    } catch (e: any) {
      setFeed(f => [...f.slice(-24), { time: new Date().toLocaleTimeString(), type: eventType, delta: def.weight, ok: false, msg: e.message || "network error" }])
    } finally {
      setSending(false)
    }
  }

  const sendRapid = async (count: number) => {
    if (!activeId) return
    setSending(true)
    const def = EVENT_DEFS.find(e => e.type === eventType)!
    for (let i = 0; i < count; i++) {
      try {
        const res = await authFetch("/api/v1/events/ingest", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ event_type: eventType, identity_id: activeId, source: "simulator", severity: def.severity }),
        })
        const ok = res.ok
        setFeed(f => [...f.slice(-24), { time: new Date().toLocaleTimeString(), type: eventType, delta: def.weight, ok, msg: ok ? `+${def.weight}` : "failed" }])
      } catch { }
    }
    setPulse(p => p + 1)
    setTimeout(() => pollScore(activeId), 500)
    setSending(false)
  }

  const currentDef = EVENT_DEFS.find(e => e.type === eventType)!
  const liveBandColor = liveScore.band
    ? (BANDS.find(b => b.label.toLowerCase() === (liveScore.band || "").toLowerCase()) || bandFor(liveScore.score ?? 0)).color
    : bandFor(liveScore.score ?? 0).color

  return (
    <div style={{ maxWidth: 1400, margin: '0 auto' }}>
      {/* Header */}
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: 28, fontWeight: 700, letterSpacing: '-0.02em', background: 'linear-gradient(135deg, #F0EFEC 30%, #A1A1AA)', WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent', marginBottom: 4 }}>
          Event Simulator
        </h1>
        <p style={{ fontSize: 13, color: '#5C5C62', lineHeight: 1.5 }}>
          Fire real risk events through the pipeline (UI → API → NATS → Processor → Neo4j) and watch the score move live.
        </p>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1.4fr', gap: 16, marginBottom: 24 }}>
        {/* Control panel */}
        <div className="glass-card" style={{ padding: 24 }}>
          <h3 style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.08em', color: '#5C5C62', marginBottom: 16 }}>Simulate Event</h3>

          {/* Identity selection */}
          <label style={{ fontSize: 12, color: '#9C9CA0', display: 'block', marginBottom: 6 }}>Target Identity</label>
          <div style={{ display: 'flex', gap: 8, marginBottom: 14 }}>
            <button onClick={() => setUseCustom(false)}
              style={{ flex: 1, padding: '8px 12px', borderRadius: 10, fontSize: 12, cursor: 'pointer', border: `1px solid ${!useCustom ? 'rgba(245,158,11,0.3)' : 'rgba(255,255,255,0.08)'}`, background: !useCustom ? 'rgba(245,158,11,0.08)' : 'rgba(255,255,255,0.02)', color: '#F0EFEC' }}>
              Pick from list
            </button>
            <button onClick={() => setUseCustom(true)}
              style={{ flex: 1, padding: '8px 12px', borderRadius: 10, fontSize: 12, cursor: 'pointer', border: `1px solid ${useCustom ? 'rgba(245,158,11,0.3)' : 'rgba(255,255,255,0.08)'}`, background: useCustom ? 'rgba(245,158,11,0.08)' : 'rgba(255,255,255,0.02)', color: '#F0EFEC' }}>
              Manual ID
            </button>
          </div>

          {useCustom ? (
            <input
              value={customId}
              onChange={e => setCustomId(e.target.value)}
              placeholder="e.g. demo-dave"
              style={{ width: '100%', padding: '10px 12px', borderRadius: 10, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.08)', color: '#F0EFEC', fontSize: 13, marginBottom: 14, outline: 'none' }}
            />
          ) : (
            <select
              value={selectedId}
              onChange={e => setSelectedId(e.target.value)}
              style={{ width: '100%', padding: '10px 12px', borderRadius: 10, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.08)', color: '#F0EFEC', fontSize: 13, marginBottom: 14, outline: 'none' }}
            >
              {identities.filter(i => i.id === "test-identity-001").map(i => (
                <option key={i.id} value={i.id}>{i.display_name || i.email} (test)</option>
              ))}
              {identities.filter(i => i.id !== "test-identity-001").map(i => (
                <option key={i.id} value={i.id}>{i.display_name || i.email}{i.department ? ` · ${i.department}` : ""}</option>
              ))}
            </select>
          )}

          {/* Event type */}
          <label style={{ fontSize: 12, color: '#9C9CA0', display: 'block', marginBottom: 6 }}>Event Type</label>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8, marginBottom: 14, maxHeight: 240, overflowY: 'auto', paddingRight: 4 }}>
            {EVENT_DEFS.map(def => {
              const selected = def.type === eventType
              return (
                <button key={def.type} onClick={() => setEventType(def.type)} title={def.desc}
                  style={{ padding: '8px 10px', borderRadius: 10, fontSize: 12, cursor: 'pointer', textAlign: 'left', border: `1px solid ${selected ? 'rgba(245,158,11,0.35)' : 'rgba(255,255,255,0.06)'}`, background: selected ? 'rgba(245,158,11,0.08)' : 'rgba(255,255,255,0.02)', color: selected ? '#FBBF24' : '#9C9CA0' }}>
                  <div style={{ fontWeight: 600, fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{def.type.replace("auth.", "").replace(".", " ")}</div>
                  <div style={{ fontSize: 10, color: '#5C5C62' }}>+{def.weight}</div>
                </button>
              )
            })}
          </div>

          {/* Metadata */}
          <label style={{ fontSize: 12, color: '#9C9CA0', display: 'block', marginBottom: 6 }}>Metadata (optional JSON)</label>
          <textarea
            value={metadata}
            onChange={e => setMetadata(e.target.value)}
            placeholder={'{"ip":"185.220.101.4","geo":"Moscow","previous_geo":"San Francisco"}'}
            style={{ width: '100%', padding: '10px 12px', borderRadius: 10, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.08)', color: '#F0EFEC', fontSize: 12, marginBottom: 16, outline: 'none', fontFamily: 'monospace', minHeight: 48, resize: 'vertical' }}
          />

          {/* Actions */}
          <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr 1fr', gap: 8 }}>
            <button onClick={send} disabled={sending || !activeId}
              style={{ background: 'linear-gradient(135deg, #F59E0B, #F97316)', border: 'none', color: '#111', padding: '12px 16px', borderRadius: 12, fontSize: 14, fontWeight: 700, cursor: sending ? 'wait' : 'pointer', opacity: sending || !activeId ? 0.5 : 1 }}>
              {sending ? "Sending…" : "▶ Send Event"}
            </button>
            <button onClick={() => sendRapid(5)} disabled={sending}
              style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.25)', color: '#F87171', borderRadius: 12, fontSize: 12, fontWeight: 600, cursor: 'pointer' }}>
              ×5 Rapid
            </button>
            <button onClick={() => sendRapid(8)} disabled={sending}
              style={{ background: 'rgba(239,68,68,0.1)', border: '1px solid rgba(239,68,68,0.25)', color: '#F87171', borderRadius: 12, fontSize: 12, fontWeight: 600, cursor: 'pointer' }}>
              ×8 Attack
            </button>
          </div>

          <p style={{ fontSize: 11, color: '#5C5C62', marginTop: 12, lineHeight: 1.5 }}>
            {currentDef.desc}. Severity: <span style={{ color: currentDef.weight >= 250 ? "#F87171" : currentDef.weight >= 100 ? "#FBBF24" : "#9C9CA0" }}>{currentDef.severity}</span>. Risk decays −5/hr after the burst.
          </p>
        </div>

        {/* Live score */}
        <div className="glass-card" style={{ padding: 24, display: 'flex', flexDirection: 'column' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
            <h3 style={{ fontSize: 11, fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.08em', color: '#5C5C62' }}>
              Live Risk Score
            </h3>
            <span style={{ fontSize: 11, color: liveScore.band ? bandFor(liveScore.score ?? 0).color : '#5C5C62' }}>
              {liveScore.eventCount !== null ? `${liveScore.eventCount} events` : "no data"}
            </span>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: 28, flex: 1 }}>
            {/* Gauge */}
            <div style={{ position: 'relative', width: 180, height: 180, flexShrink: 0 }}>
              <div style={{
                position: 'absolute', inset: 0, borderRadius: '50%',
                background: `conic-gradient(${liveBandColor} ${Math.min(100, ((liveScore.score ?? 0) / 1000) * 100)}%, rgba(255,255,255,0.06) 0)`,
                WebkitMask: 'radial-gradient(farthest-side, transparent calc(100% - 14px), black calc(100% - 14px))',
                mask: 'radial-gradient(farthest-side, transparent calc(100% - 14px), black calc(100% - 14px))',
                transition: 'all 0.5s ease',
              }} />
              <div style={{ position: 'absolute', inset: 0, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
                <div key={pulse} style={{ fontSize: 42, fontWeight: 800, letterSpacing: '-0.04em', color: liveBandColor, textShadow: `0 0 30px ${liveBandColor}60`, animation: pulse ? 'float 1s ease' : undefined }}>
                  {fmt(liveScore.score)}
                </div>
                <div style={{ fontSize: 12, color: '#5C5C62', textTransform: 'uppercase', letterSpacing: '0.08em' }}>
                  {liveScore.band ? liveScore.band.toUpperCase() : "NO DATA"}
                </div>
              </div>
            </div>

            {/* Breakdown */}
            <div style={{ flex: 1 }}>
              {[
                ["Static (30%)", liveScore.staticScore, "#60A5FA"],
                ["Dynamic (50%)", liveScore.dynamicScore, "#F472B6"],
                ["Peer (20%)", liveScore.peerScore, "#34D399"],
              ].map(([label, score, color]) => {
                const v = (score as number) ?? 0
                return (
                  <div key={label as string} style={{ marginBottom: 12 }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 4 }}>
                      <span style={{ color: '#9C9CA0' }}>{label as string}</span>
                      <span style={{ color: color as string, fontWeight: 600 }}>{fmt(v as number)}</span>
                    </div>
                    <div style={{ height: 6, borderRadius: 3, background: 'rgba(255,255,255,0.04)', overflow: 'hidden' }}>
                      <div style={{ height: '100%', width: `${Math.min(100, (v / 1000) * 100)}%`, background: color as string, borderRadius: 3, boxShadow: `0 0 8px ${color}80`, transition: 'width 0.5s ease' }} />
                    </div>
                  </div>
                )
              })}
              <div style={{ marginTop: 12, padding: '10px 12px', borderRadius: 8, background: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.04)' }}>
                <div style={{ fontSize: 11, color: '#5C5C62' }}>
                  Band thresholds: <span style={{ color: '#EF4444' }}>800+ critical</span> · <span style={{ color: '#F97316' }}>600 high</span> · <span style={{ color: '#F59E0B' }}>300 elevated</span>
                </div>
                <div style={{ fontSize: 11, color: '#5C5C62', marginTop: 4 }}>
                  Critical → auto session terminate + review · High → step-up MFA · Elevated → micro-review
                </div>
              </div>
            </div>
          </div>

          {/* Event feed */}
          <div ref={feedRef} style={{ marginTop: 16, maxHeight: 200, overflowY: 'auto', borderTop: '1px solid rgba(255,255,255,0.06)', paddingTop: 12 }}>
            {feed.length === 0 ? (
              <p style={{ fontSize: 12, color: '#5C5C62' }}>No events sent yet. Hit <span style={{ color: '#FBBF24' }}>Send Event</span> to fire one through the pipeline.</p>
            ) : feed.map((f, i) => (
              <div key={i} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '6px 0', fontSize: 12, borderBottom: '1px solid rgba(255,255,255,0.03)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span style={{ width: 6, height: 6, borderRadius: '50%', background: f.ok ? '#34D399' : '#EF4444', boxShadow: `0 0 6px ${f.ok ? '#34D399' : '#EF4444'}` }} />
                  <span style={{ color: '#F0EFEC', fontFamily: 'monospace' }}>{f.type}</span>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <span style={{ color: f.ok ? '#34D399' : '#F87171', fontWeight: 600 }}>{f.msg}</span>
                  <span style={{ color: '#5C5C62', fontSize: 11 }}>{f.time}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
