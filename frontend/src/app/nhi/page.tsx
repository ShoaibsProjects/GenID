"use client"
import { useState, useEffect, useCallback } from "react"
import {
  fetchNHI,
  registerNHI,
  fetchPassports,
  issuePassport,
  revokePassports,
  type NHIRecord,
  type NHIPassport,
} from "@/lib/api"
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/Card"
import { Badge } from "@/components/ui/Badge"
import { Button } from "@/components/ui/Button"
import { Input } from "@/components/ui/Input"
import {
  Table, TableHeader, TableBody, TableRow, TableHead, TableCell,
} from "@/components/ui/table"
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription,
  DialogFooter, DialogClose,
} from "@/components/ui/dialog"
import { AlertTriangle, Shield, KeyRound, Plus, RefreshCw, Bot } from "lucide-react"

const TYPE_OPTIONS = ["service_account", "workload", "api_agent", "cron", "ci_runner", "other"]

function riskBand(score: number | undefined): string {
  if (score == null) return "unknown"
  if (score >= 800) return "critical"
  if (score >= 600) return "high"
  if (score >= 300) return "elevated"
  return "low"
}

function bandBadge(score: number | undefined) {
  const band = riskBand(score)
  const cls =
    band === "critical" || band === "high"
      ? "border-red-500/40 bg-red-500/10 text-red-400"
      : band === "elevated"
        ? "border-amber-500/40 bg-amber-500/10 text-amber-400"
        : "border-emerald-500/40 bg-emerald-500/10 text-emerald-400"
  return <Badge className={cls}>{band}</Badge>
}

export default function NHIPage() {
  const [rows, setRows] = useState<NHIRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [msg, setMsg] = useState("")
  const [registerOpen, setRegisterOpen] = useState(false)
  const [passportTarget, setPassportTarget] = useState<NHIRecord | null>(null)
  const [busyId, setBusyId] = useState("")

  const load = useCallback(async () => {
    setLoading(true)
    setError("")
    try {
      const res = await fetchNHI()
      setRows(res.nhi || [])
    } catch (e: any) {
      setError(e.message || "Failed to load NHI registry")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  const revoke = async (r: NHIRecord) => {
    setBusyId(r.id)
    setMsg("")
    try {
      const res = await revokePassports(r.id)
      setMsg(`Revoked ${res.revoked} passport${res.revoked === 1 ? "" : "s"} for ${r.name}`)
    } catch (e: any) {
      setMsg(`Revoke failed: ${e.message}`)
    } finally {
      setBusyId("")
    }
  }

  return (
    <div className="max-w-7xl mx-auto space-y-6 animate-fade-in">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gradient-accent">NHI Registry</h1>
          <p className="text-[var(--text-secondary)] mt-1">
            Non-human identities, agent cards, and JIT passports
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            <RefreshCw className="w-4 h-4 mr-2" />
            Refresh
          </Button>
          <Button variant="default" size="sm" onClick={() => setRegisterOpen(true)}>
            <Plus className="w-4 h-4 mr-2" />
            Register Agent
          </Button>
        </div>
      </div>

      {msg && (
        <div className="p-3 rounded border border-[var(--obsidian-border)] bg-[var(--glass-2)] text-sm text-[var(--text-secondary)]">
          {msg}
        </div>
      )}
      {error && (
        <div className="p-3 rounded border border-red-900/50 bg-red-900/10 text-sm text-red-400">
          {error}
        </div>
      )}

      <Card className="glass-card">
        <CardHeader>
          <CardTitle className="text-xl">Identities</CardTitle>
          <CardDescription>All registered non-human identities, sorted by risk</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Owner</TableHead>
                  <TableHead>Risk</TableHead>
                  <TableHead>Environment</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading ? (
                  <TableRow>
                    <TableCell colSpan={7} className="text-center py-8 text-[var(--text-muted)]">
                      Loading NHI registry...
                    </TableCell>
                  </TableRow>
                ) : rows.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} className="text-center py-8">
                      <div className="flex flex-col items-center gap-2 text-[var(--text-muted)]">
                        <Bot className="w-8 h-8" />
                        No non-human identities registered yet
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  rows.map((r) => (
                    <TableRow key={r.id} className="hover:bg-[var(--glass-2)]">
                      <TableCell>
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-full bg-[var(--accent-dim)] flex items-center justify-center text-[var(--accent)]">
                            <Bot className="w-4 h-4" />
                          </div>
                          <div>
                            <p className="font-medium text-sm text-[var(--text-primary)]">{r.name}</p>
                            <p className="text-xs text-[var(--text-muted)] font-mono">{r.id.slice(0, 8)}</p>
                          </div>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">{r.type || "service_account"}</Badge>
                      </TableCell>
                      <TableCell className="text-sm text-[var(--text-secondary)]">
                        {r.owner_id ? r.owner_id.slice(0, 8) : "—"}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <span className="font-mono text-sm">{r.risk_score ?? "—"}</span>
                          {bandBadge(r.risk_score)}
                        </div>
                      </TableCell>
                      <TableCell className="text-sm text-[var(--text-secondary)]">
                        {r.deployment_environment || "—"}
                      </TableCell>
                      <TableCell>
                        <Badge className={r.status === "active" ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-400" : "border-red-500/40 bg-red-500/10 text-red-400"}>
                          {r.status || "unknown"}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <Button variant="ghost" size="sm" onClick={() => setPassportTarget(r)} disabled={r.status !== "active"}>
                            <KeyRound className="w-3.5 h-3.5 mr-1" />
                            Issue Passport
                          </Button>
                          <Button variant="ghost" size="sm" onClick={() => revoke(r)} disabled={busyId === r.id}>
                            <Shield className="w-3.5 h-3.5 mr-1" />
                            {busyId === r.id ? "Revoking..." : "Revoke"}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>

      <RegisterModal open={registerOpen} onOpenChange={setRegisterOpen} onDone={() => { setRegisterOpen(false); load() }} />

      {passportTarget && (
        <PassportModal
          nhi={passportTarget}
          onClose={() => setPassportTarget(null)}
        />
      )}
    </div>
  )
}

// ─── Register Agent Modal ──────────────────────────────────────

function RegisterModal({ open, onOpenChange, onDone }: { open: boolean; onOpenChange: (v: boolean) => void; onDone: () => void }) {
  const [name, setName] = useState("")
  const [type, setType] = useState("service_account")
  const [ownerId, setOwnerId] = useState("")
  const [env, setEnv] = useState("production")
  const [protocols, setProtocols] = useState("")
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState("")

  const submit = async () => {
    if (!name.trim()) { setErr("Name is required"); return }
    setBusy(true)
    setErr("")
    try {
      await registerNHI({
        name: name.trim(),
        type,
        owner_id: ownerId.trim() || undefined,
        deployment_environment: env,
        protocols: protocols.split(",").map((s) => s.trim()).filter(Boolean),
      })
      setName(""); setOwnerId(""); setProtocols("")
      onDone()
    } catch (e: any) {
      setErr(e.message || "Registration failed")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Register Non-Human Identity</DialogTitle>
          <DialogDescription>
            Create an agent card + governed NHI record. Risk is scored automatically.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">Name *</label>
            <Input placeholder="e.g. payments-worker" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div>
            <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">Type</label>
            <div className="flex flex-wrap gap-2">
              {TYPE_OPTIONS.map((t) => (
                <button
                  key={t}
                  type="button"
                  onClick={() => setType(t)}
                  className={`px-3 py-1.5 rounded text-xs font-semibold border transition-all duration-150 ${
                    type === t
                      ? "bg-accent/10 border-accent text-accent"
                      : "bg-transparent border-border text-secondary hover:text-primary hover:border-muted"
                  }`}
                >
                  {t}
                </button>
              ))}
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">Owner ID</label>
              <Input placeholder="e.g. user-001" value={ownerId} onChange={(e) => setOwnerId(e.target.value)} />
            </div>
            <div>
              <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">Environment</label>
              <Input placeholder="production" value={env} onChange={(e) => setEnv(e.target.value)} />
            </div>
          </div>
          <div>
            <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">
              Protocols (comma separated)
            </label>
            <Input placeholder="oauth2, mtls, jwt" value={protocols} onChange={(e) => setProtocols(e.target.value)} />
          </div>
          {err && <div className="p-3 rounded border border-red-900/50 bg-red-900/10 text-xs text-red-400">{err}</div>}
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline" size="sm">Cancel</Button>
          </DialogClose>
          <Button variant="default" size="sm" onClick={submit} disabled={busy}>
            {busy ? "Registering..." : "Register Agent"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ─── Issue Passport Modal ──────────────────────────────────────

function PassportModal({ nhi, onClose }: { nhi: NHIRecord; onClose: () => void }) {
  const [passports, setPassports] = useState<NHIPassport[]>([])
  const [issued, setIssued] = useState<{ token: string; scope: string; expires_at: string } | null>(null)
  const [ttl, setTtl] = useState(120)
  const [scope, setScope] = useState("access:grant")
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState("")

  const load = useCallback(async () => {
    try {
      const res = await fetchPassports(nhi.id)
      setPassports(res.passports || [])
    } catch { /* keep empty */ }
  }, [nhi.id])

  useEffect(() => { load() }, [load])

  const issue = async () => {
    setBusy(true)
    setErr("")
    try {
      const res = await issuePassport(nhi.id, { scope, ttl_minutes: ttl })
      setIssued(res)
      load()
    } catch (e: any) {
      setErr(e.message || "Issue failed")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose() }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyRound className="w-4 h-4 text-[var(--accent)]" />
            JIT Passports — {nhi.name}
          </DialogTitle>
          <DialogDescription>
            Mint time-bounded passports. The raw token is shown exactly once.
          </DialogDescription>
        </DialogHeader>

        {issued && (
          <div className="p-4 rounded border border-emerald-500/40 bg-emerald-500/10">
            <p className="text-xs font-semibold text-emerald-400 uppercase tracking-wider mb-2">Passport Issued — copy token now</p>
            <code className="block text-xs font-mono text-[var(--text-primary)] bg-[var(--obsidian-raise)] rounded p-2 break-all select-all">
              {issued.token}
            </code>
            <p className="text-xs text-[var(--text-muted)] mt-2">
              Scope: {issued.scope} · Expires: {new Date(issued.expires_at).toLocaleString()}
            </p>
          </div>
        )}

        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">Scope</label>
            <Input value={scope} onChange={(e) => setScope(e.target.value)} />
          </div>
          <div>
            <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">TTL (minutes, ≤1440)</label>
            <Input type="number" min={1} max={1440} value={ttl} onChange={(e) => setTtl(Number(e.target.value))} />
          </div>
        </div>
        {err && <div className="p-3 rounded border border-red-900/50 bg-red-900/10 text-xs text-red-400">{err}</div>}

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={onClose}>Close</Button>
          <Button variant="default" size="sm" onClick={issue} disabled={busy}>
            <KeyRound className="w-3.5 h-3.5 mr-1" />
            {busy ? "Issuing..." : "Issue Passport"}
          </Button>
        </DialogFooter>

        <div className="mt-2">
          <h4 className="text-xs font-semibold text-secondary uppercase tracking-wider mb-2">Active passports</h4>
          {passports.length === 0 ? (
            <p className="text-xs text-[var(--text-muted)]">No passports issued for this NHI.</p>
          ) : (
            <div className="space-y-2 max-h-48 overflow-y-auto">
              {passports.map((p) => (
                <div key={p.id} className="flex items-center justify-between p-3 rounded border border-[var(--obsidian-border)] bg-[var(--glass-2)]">
                  <div>
                    <p className="text-xs font-mono text-[var(--text-primary)]">{p.id.slice(0, 8)} · {p.scope}</p>
                    <p className="text-[0.65rem] text-[var(--text-muted)]">
                      Expires {new Date(p.expires_at).toLocaleString()}
                    </p>
                  </div>
                  <Badge className={
                    p.status === "active"
                      ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-400"
                      : p.status === "expired"
                        ? "border-amber-500/40 bg-amber-500/10 text-amber-400"
                        : "border-slate-500/40 bg-slate-500/10 text-slate-400"
                  }>
                    {p.status}
                  </Badge>
                </div>
              ))}
            </div>
          )}
        </div>

        {passports.some((p) => p.status === "active") && (
          <p className="flex items-center gap-1 text-xs text-amber-400/80 mt-2">
            <AlertTriangle className="w-3.5 h-3.5" />
            Active passports are held by external consumers. Revoke them from the registry if compromised.
          </p>
        )}
      </DialogContent>
    </Dialog>
  )
}
