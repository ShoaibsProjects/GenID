"use client"
import { useState, useEffect, useCallback } from "react"

const API = process.env.NEXT_PUBLIC_API_URL || ""

interface OIDCClient {
  id: string
  name: string
  client_id: string
  client_secret?: string
  redirect_uris: string[]
  grant_types: string[]
  scopes: string[]
  is_public: boolean
  created_at: string
}

interface DiscoveryDoc {
  issuer: string
  authorization_endpoint: string
  token_endpoint: string
  userinfo_endpoint: string
  jwks_uri: string
  scopes_supported: string[]
  grant_types_supported: string[]
  claims_supported: string[]
  code_challenge_methods_supported?: string[]
}

export default function IDPPage() {
  const [clients, setClients] = useState<OIDCClient[]>([])
  const [discovery, setDiscovery] = useState<DiscoveryDoc | null>(null)
  const [loading, setLoading] = useState(true)
  const [tab, setTab] = useState<"clients" | "discovery" | "endpoints" | "test">("clients")
  const [showRegister, setShowRegister] = useState(false)
  const [regForm, setRegForm] = useState({ name: "", redirect_uris: "", grant_types: "authorization_code,refresh_token" })
  const [regSecret, setRegSecret] = useState<string | null>(null)
  const [error, setError] = useState("")
  const [testResult, setTestResult] = useState<any>(null)
  const [testLoading, setTestLoading] = useState(false)

  const fetchData = useCallback(async () => {
    try {
      const [cRes, dRes] = await Promise.all([
        fetch(`${API}/api/v1/oidc/clients`).catch(() => null),
        fetch(`${API}/.well-known/openid-configuration`).catch(() => null),
      ])
      if (cRes?.ok) { const d = await cRes.json(); setClients(d.clients || []) }
      if (dRes?.ok) { const d = await dRes.json(); setDiscovery(d) }
    } catch {}
    setLoading(false)
  }, [])

  useEffect(() => { fetchData() }, [fetchData])

  const registerClient = async () => {
    setError("")
    try {
      const res = await fetch(`${API}/api/v1/oidc/clients`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: regForm.name,
          redirect_uris: regForm.redirect_uris.split(",").map(s => s.trim()).filter(Boolean),
          grant_types: regForm.grant_types.split(",").map(s => s.trim()).filter(Boolean),
          is_public: false,
        }),
      })
      if (!res.ok) { const e = await res.json(); setError(e.error || "Registration failed"); return }
      const data = await res.json()
      setRegSecret(data.client_secret || null)
      setShowRegister(false)
      setRegForm({ name: "", redirect_uris: "", grant_types: "authorization_code,refresh_token" })
      fetchData()
    } catch (e: any) { setError(e.message) }
  }

  const deleteClient = async (id: string) => {
    if (!confirm(`Delete client ${id}?`)) return
    await fetch(`${API}/api/v1/oidc/clients/${id}`, { method: "DELETE" })
    fetchData()
  }

  const runFlowTest = async () => {
    setTestLoading(true); setTestResult(null)
    try {
      // Step 1: authorize
      const authRes = await fetch(`${API}/authorize?response_type=code&client_id=observeid-demo&redirect_uri=http://localhost:8080/callback&scope=openid+profile+email&state=test`)
      let code = ""
      if (authRes.redirected) { code = new URL(authRes.url).searchParams.get("code") || "" }
      else { const d = await authRes.json(); code = d.code || "" }
      if (!code) { setTestResult({ error: "No authorization code", step: "authorize" }); return }

      // Step 2: token
      const tokenRes = await fetch(`${API}/token`, {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        body: new URLSearchParams({ grant_type: "authorization_code", code, redirect_uri: "http://localhost:8080/callback", client_id: "observeid-demo", client_secret: "observeid-demo-secret" }).toString(),
      })
      const tokens = await tokenRes.json()
      if (tokens.error) { setTestResult({ error: tokens.error_description || tokens.error, step: "token" }); return }

      // Step 3: userinfo
      const uiRes = await fetch(`${API}/userinfo`, { headers: { Authorization: `Bearer ${tokens.access_token}` } })
      const ui = await uiRes.json()

      setTestResult({
        success: true,
        code: code.substring(0, 16) + "...",
        tokens: { token_type: tokens.token_type, expires_in: tokens.expires_in, has_id_token: !!tokens.id_token, has_refresh_token: !!tokens.refresh_token },
        userinfo: ui,
      })
    } catch (e: any) { setTestResult({ error: e.message, step: "unknown" }) }
    finally { setTestLoading(false) }
  }

  if (loading) return <div style={{ padding: 40, color: '#5C5C62', textAlign: 'center' }}>Loading IDP configuration...</div>

  const tabs = [
    { id: "clients" as const, label: "Clients", count: clients.length },
    { id: "discovery" as const, label: "Discovery" },
    { id: "endpoints" as const, label: "Endpoints" },
    { id: "test" as const, label: "Flow Test" },
  ]

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <h1 style={{ fontSize: 24, fontWeight: 700, letterSpacing: '-0.02em' }}>Identity Provider (OIDC)</h1>
          <p style={{ fontSize: 13, color: '#5C5C62', marginTop: 4 }}>OpenID Connect provider &amp; OAuth2 client management</p>
        </div>
        <button onClick={() => setShowRegister(true)} style={{ padding: '10px 20px', borderRadius: 8, background: 'linear-gradient(135deg, rgba(245,158,11,0.15), rgba(217,119,6,0.20))', border: '1px solid rgba(245,158,11,0.25)', color: '#FBBF24', fontWeight: 600, fontSize: 13, cursor: 'pointer' }}>
          Register Client
        </button>
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: 4, padding: 4, borderRadius: 10, background: 'rgba(255,255,255,0.02)', border: '1px solid rgba(255,255,255,0.04)' }}>
        {tabs.map(t => (
          <button key={t.id} onClick={() => setTab(t.id)} style={{
            flex: 1, padding: '10px 16px', borderRadius: 7, fontSize: 13, fontWeight: tab === t.id ? 600 : 450, cursor: 'pointer', border: 'none',
            background: tab === t.id ? 'rgba(245,158,11,0.08)' : 'transparent',
            color: tab === t.id ? '#FBBF24' : '#5C5C62',
            borderBottom: tab === t.id ? '2px solid #F59E0B' : '2px solid transparent',
            transition: 'all 0.2s',
          }}>
            {t.label}{t.count !== undefined && <span style={{ marginLeft: 6, padding: '1px 6px', borderRadius: 4, fontSize: 10, background: 'rgba(255,255,255,0.04)' }}>{t.count}</span>}
          </button>
        ))}
      </div>

      {/* Secret display */}
      {regSecret && (
        <div style={{ padding: 16, borderRadius: 10, background: 'rgba(16,185,129,0.06)', border: '1px solid rgba(16,185,129,0.2)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div><p style={{ fontSize: 13, fontWeight: 600, color: '#34D399' }}>Client Registered</p><p style={{ fontSize: 11, color: '#5C5C62', marginTop: 2 }}>Copy this secret — it will not be shown again.</p></div>
            <button onClick={() => setRegSecret(null)} style={{ background: 'none', border: 'none', color: '#5C5C62', cursor: 'pointer', fontSize: 12 }}>Dismiss</button>
          </div>
          <div style={{ marginTop: 10, padding: 12, borderRadius: 6, background: 'rgba(0,0,0,0.3)', fontFamily: 'monospace', fontSize: 13, color: '#34D399', wordBreak: 'break-all' }}>{regSecret}</div>
        </div>
      )}

      {/* Clients Tab */}
      {tab === "clients" && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {clients.length === 0 ? (
            <div style={{ padding: 40, textAlign: 'center', borderRadius: 10, border: '1px solid rgba(255,255,255,0.04)', background: 'rgba(255,255,255,0.02)' }}>
              <p style={{ color: '#5C5C62', fontSize: 13 }}>No OIDC clients registered yet.</p>
            </div>
          ) : clients.map(c => (
            <div key={c.id} style={{ padding: 16, borderRadius: 10, border: '1px solid rgba(255,255,255,0.04)', background: 'rgba(255,255,255,0.02)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                    <span style={{ fontWeight: 600, fontSize: 14 }}>{c.name}</span>
                    <span style={{ padding: '2px 8px', borderRadius: 4, fontSize: 10, background: c.is_public ? 'rgba(251,146,60,0.12)' : 'rgba(96,165,250,0.12)', color: c.is_public ? '#FB923C' : '#60A5FA', border: `1px solid ${c.is_public ? 'rgba(251,146,60,0.2)' : 'rgba(96,165,250,0.2)'}` }}>
                      {c.is_public ? "Public" : "Confidential"}
                    </span>
                  </div>
                  <p style={{ fontFamily: 'monospace', fontSize: 12, color: '#F59E0B', marginTop: 4 }}>{c.client_id}</p>
                  <div style={{ display: 'flex', gap: 6, marginTop: 6, flexWrap: 'wrap' }}>
                    {(c.grant_types || []).map(g => <span key={g} style={{ padding: '2px 6px', borderRadius: 3, fontSize: 10, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.05)', color: '#5C5C62' }}>{g}</span>)}
                  </div>
                  {c.redirect_uris?.length > 0 && <p style={{ fontSize: 11, color: '#5C5C62', marginTop: 6 }}>Redirects: {c.redirect_uris.join(", ")}</p>}
                </div>
                <button onClick={() => deleteClient(c.client_id)} style={{ padding: '6px 12px', borderRadius: 6, fontSize: 11, background: 'rgba(239,68,68,0.08)', border: '1px solid rgba(239,68,68,0.15)', color: '#EF4444', cursor: 'pointer' }}>Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Discovery Tab */}
      {tab === "discovery" && discovery && (
        <div style={{ padding: 20, borderRadius: 10, border: '1px solid rgba(255,255,255,0.04)', background: 'rgba(255,255,255,0.02)' }}>
          <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 16, display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: '#34D399' }} />OpenID Provider Metadata
          </h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {Object.entries(discovery).map(([k, v]) => (
              <div key={k} style={{ display: 'flex', gap: 16, fontSize: 13, alignItems: 'flex-start' }}>
                <span style={{ fontFamily: 'monospace', fontSize: 12, color: '#F59E0B', minWidth: 280, flexShrink: 0 }}>{k}</span>
                <span style={{ fontFamily: 'monospace', fontSize: 12, color: '#5C5C62', wordBreak: 'break-all' }}>{Array.isArray(v) ? v.join(", ") : String(v)}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Endpoints Tab */}
      {tab === "endpoints" && (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(340px, 1fr))', gap: 12 }}>
          {[
            { name: "Discovery", method: "GET", path: "/.well-known/openid-configuration", desc: "OIDC provider metadata" },
            { name: "JWKS", method: "GET", path: "/.well-known/jwks.json", desc: "JSON Web Key Set for token verification" },
            { name: "Authorize", method: "GET/POST", path: "/authorize", desc: "Authorization endpoint with login form" },
            { name: "Token", method: "POST", path: "/token", desc: "Token exchange (form-urlencoded per RFC 6749)" },
            { name: "UserInfo", method: "GET", path: "/userinfo", desc: "Returns ID token claims for authenticated users" },
            { name: "List Clients", method: "GET", path: "/api/v1/oidc/clients", desc: "List all registered OIDC clients" },
            { name: "Register Client", method: "POST", path: "/api/v1/oidc/clients", desc: "Register a new OAuth2 client" },
          ].map(ep => (
            <div key={ep.path} style={{ padding: 14, borderRadius: 10, border: '1px solid rgba(255,255,255,0.04)', background: 'rgba(255,255,255,0.02)' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
                <span style={{ padding: '2px 6px', borderRadius: 3, fontSize: 10, fontWeight: 700, background: ep.method.includes("POST") ? 'rgba(52,211,153,0.12)' : 'rgba(96,165,250,0.12)', color: ep.method.includes("POST") ? '#34D399' : '#60A5FA' }}>{ep.method}</span>
                <span style={{ fontWeight: 600, fontSize: 13 }}>{ep.name}</span>
              </div>
              <p style={{ fontFamily: 'monospace', fontSize: 12, color: '#F59E0B', marginTop: 4 }}>{ep.path}</p>
              <p style={{ fontSize: 12, color: '#5C5C62', marginTop: 2 }}>{ep.desc}</p>
            </div>
          ))}
        </div>
      )}

      {/* Flow Test Tab */}
      {tab === "test" && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          <div style={{ padding: 16, borderRadius: 10, border: '1px solid rgba(255,255,255,0.04)', background: 'rgba(255,255,255,0.02)' }}>
            <h3 style={{ fontSize: 14, fontWeight: 600, marginBottom: 8 }}>OIDC Authorization Code Flow</h3>
            <p style={{ fontSize: 12, color: '#5C5C62', marginBottom: 12 }}>Executes: authorize → token → userinfo. Uses demo client.</p>
            <button onClick={runFlowTest} disabled={testLoading} style={{ padding: '10px 20px', borderRadius: 8, background: 'linear-gradient(135deg, rgba(245,158,11,0.15), rgba(217,119,6,0.20))', border: '1px solid rgba(245,158,11,0.25)', color: '#FBBF24', fontWeight: 600, fontSize: 13, cursor: 'pointer', opacity: testLoading ? 0.5 : 1 }}>
              {testLoading ? "Running..." : "Run Full OIDC Flow"}
            </button>
          </div>
          {testResult && (
            <div style={{ padding: 16, borderRadius: 10, border: `1px solid ${testResult.success ? 'rgba(52,211,153,0.2)' : 'rgba(239,68,68,0.2)'}`, background: testResult.success ? 'rgba(52,211,153,0.04)' : 'rgba(239,68,68,0.04)' }}>
              {testResult.error ? (
                <div><p style={{ fontSize: 13, fontWeight: 600, color: '#EF4444' }}>Error at: {testResult.step}</p><p style={{ fontSize: 12, color: '#F87171', fontFamily: 'monospace', marginTop: 4 }}>{testResult.error}</p></div>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                  <p style={{ fontSize: 13, fontWeight: 600, color: '#34D399' }}>Flow Completed Successfully</p>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                    <div style={{ padding: 10, borderRadius: 6, background: 'rgba(0,0,0,0.2)', border: '1px solid rgba(255,255,255,0.04)' }}>
                      <p style={{ fontSize: 10, color: '#5C5C62', marginBottom: 4 }}>Auth Code</p>
                      <p style={{ fontFamily: 'monospace', fontSize: 12, color: '#F59E0B' }}>{testResult.code}</p>
                    </div>
                    <div style={{ padding: 10, borderRadius: 6, background: 'rgba(0,0,0,0.2)', border: '1px solid rgba(255,255,255,0.04)' }}>
                      <p style={{ fontSize: 10, color: '#5C5C62', marginBottom: 4 }}>Token Response</p>
                      <pre style={{ fontFamily: 'monospace', fontSize: 11, color: '#5C5C62', whiteSpace: 'pre-wrap' }}>{JSON.stringify(testResult.tokens, null, 2)}</pre>
                    </div>
                  </div>
                  {testResult.userinfo && (
                    <div style={{ padding: 10, borderRadius: 6, background: 'rgba(0,0,0,0.2)', border: '1px solid rgba(255,255,255,0.04)' }}>
                      <p style={{ fontSize: 10, color: '#5C5C62', marginBottom: 4 }}>UserInfo Claims</p>
                      <pre style={{ fontFamily: 'monospace', fontSize: 11, color: '#5C5C62', whiteSpace: 'pre-wrap' }}>{JSON.stringify(testResult.userinfo, null, 2)}</pre>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* Register Modal */}
      {showRegister && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', backdropFilter: 'blur(8px)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50 }} onClick={() => setShowRegister(false)}>
          <div style={{ width: 420, padding: 28, borderRadius: 14, background: 'rgba(12,12,16,0.95)', border: '1px solid rgba(255,255,255,0.06)', boxShadow: '0 24px 64px rgba(0,0,0,0.5)' }} onClick={e => e.stopPropagation()}>
            <h3 style={{ fontSize: 16, fontWeight: 600, marginBottom: 16 }}>Register OIDC Client</h3>
            {error && <p style={{ fontSize: 12, color: '#EF4444', marginBottom: 10 }}>{error}</p>}
            {[
              { label: "Client Name", key: "name", placeholder: "My Application" },
              { label: "Redirect URIs (comma-separated)", key: "redirect_uris", placeholder: "http://localhost:3000/callback" },
              { label: "Grant Types (comma-separated)", key: "grant_types", placeholder: "authorization_code,refresh_token" },
            ].map(f => (
              <div key={f.key} style={{ marginBottom: 12 }}>
                <label style={{ fontSize: 11, color: '#5C5C62', textTransform: 'uppercase', letterSpacing: '0.06em', marginBottom: 4, display: 'block' }}>{f.label}</label>
                <input value={(regForm as any)[f.key]} onChange={e => setRegForm({ ...regForm, [f.key]: e.target.value })} placeholder={f.placeholder} style={{ width: '100%', padding: '10px 12px', borderRadius: 7, background: 'rgba(255,255,255,0.03)', border: '1px solid rgba(255,255,255,0.06)', color: '#F0EFEC', fontSize: 13, outline: 'none' }} />
              </div>
            ))}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16 }}>
              <button onClick={() => setShowRegister(false)} style={{ padding: '8px 16px', borderRadius: 7, fontSize: 13, background: 'transparent', border: '1px solid rgba(255,255,255,0.06)', color: '#5C5C62', cursor: 'pointer' }}>Cancel</button>
              <button onClick={registerClient} style={{ padding: '8px 16px', borderRadius: 7, fontSize: 13, fontWeight: 600, background: 'linear-gradient(135deg, rgba(245,158,11,0.15), rgba(217,119,6,0.20))', border: '1px solid rgba(245,158,11,0.25)', color: '#FBBF24', cursor: 'pointer' }}>Register</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
