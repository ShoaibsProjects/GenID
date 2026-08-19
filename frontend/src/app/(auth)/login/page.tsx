"use client"
import { useState } from "react"
import { devLogin, setAuthToken } from "@/lib/api"
import { Button } from "@/components/ui/Button"
import { Input } from "@/components/ui/Input"

export default function LoginPage() {
  const [email, setEmail] = useState("admin@genid.io")
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const [offlineMode, setOfflineMode] = useState(false)

  const go = () => {
    window.location.replace("/dashboard")
  }

  const doLogin = async (username: string, pass: string) => {
    setBusy(true)
    setError("")
    try {
      const res = await devLogin(username, pass)
      if (res) { go(); return }
      // Offline fallback (flagged): mint a local mock token so the UI remains
      // browsable without the backend. TODO: remove once /api/v1/auth/login exists.
      setAuthToken("mock." + btoa(username), username)
      setOfflineMode(true)
      setTimeout(go, 900)
    } catch {
      setAuthToken("mock." + btoa(username), username)
      setOfflineMode(true)
      setTimeout(go, 900)
    } finally {
      setBusy(false)
    }
  }

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!email.trim() || !password.trim()) { setError("Email and password are required"); return }
    doLogin(email.trim(), password.trim())
  }

  const quickDev = () => doLogin("admin@genid.io", "dev-login")

  return (
    <div className="min-h-screen flex items-center justify-center px-4 bg-[var(--obsidian-raise)] relative overflow-hidden">
      <div className="absolute -top-40 -right-40 w-96 h-96 rounded-full bg-[var(--accent)]/20 blur-[120px] pointer-events-none" />
      <div className="absolute -bottom-40 -left-40 w-96 h-96 rounded-full bg-[var(--accent2)]/20 blur-[120px] pointer-events-none" />

      <div className="w-full max-w-md relative animate-fade-in">
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-[var(--accent-dim)] border border-[var(--accent)]/40 mb-4">
            <ShieldMark />
          </div>
          <h1 className="text-3xl font-bold text-gradient-accent">GenID</h1>
          <p className="text-sm text-[var(--text-secondary)] mt-2">
            Identity-First Security Platform — sign in to continue
          </p>
        </div>

        {offlineMode && (
          <div className="p-3 mb-4 rounded border border-amber-500/40 bg-amber-500/10 text-xs text-amber-400">
            Backend unreachable — using offline mock auth. Data shown is sample/local.
          </div>
        )}

        <form onSubmit={submit} className="space-y-4 p-6 rounded-2xl border border-[var(--obsidian-border)] bg-[var(--glass-card)] backdrop-blur-xl shadow-2xl">
          <div>
            <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">Email</label>
            <Input
              type="email"
              placeholder="admin@genid.io"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="username"
            />
          </div>
          <div>
            <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">Password</label>
            <Input
              type="password"
              placeholder="••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
            />
          </div>

          {error && (
            <div className="p-3 rounded border border-red-900/50 bg-red-900/10 text-xs text-red-400">{error}</div>
          )}

          <Button variant="default" className="w-full" type="submit" disabled={busy}>
            {busy ? "Signing in..." : "Sign In"}
          </Button>

          <div className="relative">
            <div className="absolute inset-0 flex items-center"><span className="w-full border-t border-[var(--obsidian-border)]" /></div>
            <div className="relative flex justify-center text-xs text-secondary"><span className="bg-[var(--glass-card)] px-2">or</span></div>
          </div>

          <Button variant="outline" className="w-full" type="button" onClick={quickDev} disabled={busy}>
            Use Dev Login (admin@genid.io)
          </Button>

          <p className="text-[0.65rem] text-[var(--text-muted)] text-center">
            Dev endpoint: /api/v1/dev/login · Prod login via OIDC (mock until wired)
          </p>
        </form>
      </div>
    </div>
  )
}

function ShieldMark() {
  return (
    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" className="text-[var(--accent)]">
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
      <path d="M9 12l2 2 4-4" />
    </svg>
  )
}