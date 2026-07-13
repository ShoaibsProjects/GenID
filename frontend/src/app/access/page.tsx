"use client"

import { useState } from "react"
import { checkAccess as apiCheckAccess, requestJITAccess } from "@/lib/api"
import { PageHeader } from "@/components/ui/PageHeader"
import { Button } from "@/components/ui/Button"
import { Card, CardBody, CardHeader } from "@/components/ui/Card"
import { Input, Select } from "@/components/ui/Input"
import { Tabs } from "@/components/ui/Tabs"
import { Badge } from "@/components/ui/Badge"

const actions = ["read", "write", "admin", "delete"]

const DURATION_PRESETS = [
  { label: "15m", mins: 15 },
  { label: "30m", mins: 30 },
  { label: "1h", mins: 60 },
  { label: "4h", mins: 240 },
  { label: "24h", mins: 1440 },
]

function ShieldIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
    </svg>
  )
}

function ZapIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
    </svg>
  )
}

function CheckIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  )
}

function CopyIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  )
}

function ExternalLinkIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
      <polyline points="15 3 21 3 21 9" />
      <line x1="10" y1="14" x2="21" y2="3" />
    </svg>
  )
}

export default function AccessPage() {
  const [tab, setTab] = useState("jit")

  const tabs = [
    { id: "check", label: "Check Access" },
    { id: "jit", label: "Just-In-Time Access" },
    { id: "grant", label: "Grant / Revoke" },
  ]

  return (
    <div className="space-y-6">
      <PageHeader
        title="Access Control"
        description="Check, request, grant, and revoke access across the identity fabric"
      />

      <Card>
        <CardBody>
          <Tabs tabs={tabs} active={tab} onChange={setTab} />
        </CardBody>
      </Card>

      {tab === "check" && <CheckAccessTab />}
      {tab === "jit" && <JITAccessTab />}
      {tab === "grant" && <GrantRevokeTab />}
    </div>
  )
}

// ── Check Access Tab ────────────────────────────────────────

function CheckAccessTab() {
  const [identityId, setIdentityId] = useState("")
  const [resourceId, setResourceId] = useState("")
  const [action, setAction] = useState("read")
  const [result, setResult] = useState<any>(null)
  const [loading, setLoading] = useState(false)

  async function check() {
    if (!identityId || !resourceId) return
    setLoading(true)
    try {
      const res = await apiCheckAccess({
        identity_id: identityId,
        resource_id: resourceId,
        action,
      })
      setResult(res)
    } catch (e: any) {
      setResult({ error: e.message })
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card>
      <CardHeader>
        <h2 className="text-sm font-semibold text-primary">Check Access</h2>
        <p className="text-xs text-secondary mt-0.5">Evaluate whether an identity has access to a resource</p>
      </CardHeader>
      <CardBody>
        <div className="grid grid-cols-3 gap-4 mb-4">
          <Input label="Identity ID" placeholder="e.g. user-001" value={identityId} onChange={(e) => setIdentityId(e.target.value)} />
          <Input label="Resource ID" placeholder="e.g. s3-bucket-prod" value={resourceId} onChange={(e) => setResourceId(e.target.value)} />
          <Select label="Action" value={action} onChange={(e) => setAction(e.target.value)} options={actions.map(a => ({ value: a, label: a.charAt(0).toUpperCase() + a.slice(1) }))} />
        </div>
        <Button variant="primary" size="sm" onClick={check} loading={loading} icon={<ShieldIcon />}>
          Check
        </Button>

        {result && (
          <div className="mt-4 p-4 rounded border border-border bg-white/[0.02]">
            <div className="flex items-center gap-2 mb-2">
              <Badge variant={result.allowed ? "success" : "danger"}>{result.allowed ? "ALLOWED" : "DENIED"}</Badge>
              {result.latency_ms && <span className="text-xs text-muted">{result.latency_ms}ms</span>}
            </div>
            <pre className="text-xs text-secondary font-mono whitespace-pre-wrap">{JSON.stringify(result, null, 2)}</pre>
          </div>
        )}
      </CardBody>
    </Card>
  )
}

// ── Just-In-Time Access Tab ─────────────────────────────────

function JITAccessTab() {
  const [identityId, setIdentityId] = useState("")
  const [resourceId, setResourceId] = useState("")
  const [resourceType, setResourceType] = useState("")
  const [action, setAction] = useState("read")
  const [durationMins, setDurationMins] = useState(60)
  const [reason, setReason] = useState("")
  const [result, setResult] = useState<any>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState("")

  async function submit() {
    setError("")
    if (!identityId.trim()) { setError("Identity ID is required"); return }
    if (!resourceId.trim()) { setError("Resource ID is required"); return }
    if (!reason.trim()) { setError("Reason is required"); return }

    setLoading(true)
    try {
      const res = await requestJITAccess({
        identity_id: identityId,
        resource_id: resourceId,
        resource_type: resourceType || undefined,
        action,
        duration_mins: durationMins,
        reason,
      })
      setResult(res)
    } catch (e: any) {
      setError(e.message || "Request failed")
    } finally {
      setLoading(false)
    }
  }

  function reset() {
    setResult(null)
    setError("")
  }

  return (
    <div className="grid grid-cols-5 gap-6">
      <div className="col-span-3">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <ZapIcon />
              <h2 className="text-sm font-semibold text-primary">Request Temporary Access</h2>
            </div>
            <p className="text-xs text-secondary mt-0.5">
              Request time-bounded, policy-evaluated access to a resource
            </p>
          </CardHeader>
          <CardBody>
            {result ? (
              <div className="space-y-4">
                <div className="flex items-center gap-2">
                  <CheckIcon />
                  <span className="text-sm font-semibold text-emerald-400">Access Request Submitted</span>
                  <Badge variant="success">{result.status}</Badge>
                </div>

                <div className="p-4 rounded border border-border bg-white/[0.02] space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-secondary uppercase tracking-wider">Workflow ID</span>
                    <div className="flex items-center gap-2">
                      <code className="text-xs font-mono text-primary">{result.workflow_id}</code>
                      <button
                        className="text-muted hover:text-primary transition-colors"
                        onClick={() => navigator.clipboard.writeText(result.workflow_id)}
                        title="Copy workflow ID"
                      >
                        <CopyIcon />
                      </button>
                    </div>
                  </div>
                  <div>
                    <span className="text-xs text-secondary uppercase tracking-wider block mb-1">Temporal UI</span>
                    <a
                      href={`http://localhost:8234/namespaces/critical-offboarding/workflows/${result.workflow_id}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="text-xs text-accent hover:text-accent/80 flex items-center gap-1 transition-colors"
                    >
                      View in Temporal UI <ExternalLinkIcon />
                    </a>
                  </div>
                </div>

                <Button variant="secondary" size="sm" onClick={reset}>
                  Request Another
                </Button>
              </div>
            ) : (
              <div className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <Input
                    label="Identity ID *"
                    placeholder="e.g. user-001"
                    value={identityId}
                    onChange={(e) => setIdentityId(e.target.value)}
                  />
                  <Input
                    label="Resource ID *"
                    placeholder="e.g. s3-bucket-prod"
                    value={resourceId}
                    onChange={(e) => setResourceId(e.target.value)}
                  />
                </div>

                <div className="grid grid-cols-2 gap-4">
                  <Input
                    label="Resource Type"
                    placeholder="e.g. s3_bucket, database, api"
                    value={resourceType}
                    onChange={(e) => setResourceType(e.target.value)}
                    hint="Optional"
                  />
                  <Select
                    label="Action"
                    value={action}
                    onChange={(e) => setAction(e.target.value)}
                    options={actions.map(a => ({ value: a, label: a.charAt(0).toUpperCase() + a.slice(1) }))}
                  />
                </div>

                <div>
                  <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-2">
                    Duration
                  </label>
                  <div className="flex gap-2">
                    {DURATION_PRESETS.map((p) => (
                      <button
                        key={p.mins}
                        type="button"
                        onClick={() => setDurationMins(p.mins)}
                        className={`px-3 py-1.5 rounded text-xs font-semibold border transition-all duration-150 ${
                          durationMins === p.mins
                            ? "bg-accent/10 border-accent text-accent"
                            : "bg-transparent border-border text-secondary hover:text-primary hover:border-muted"
                        }`}
                      >
                        {p.label}
                      </button>
                    ))}
                  </div>
                </div>

                <div>
                  <label className="text-xs font-semibold text-secondary uppercase tracking-wider block mb-1">
                    Reason *
                  </label>
                  <textarea
                    className="w-full h-20 px-3 py-2 bg-white/[0.03] border border-border rounded text-sm text-primary placeholder:text-muted outline-none transition-colors duration-150 focus:border-accent focus:bg-accent/5 resize-none"
                    placeholder="Why is this access needed? e.g. Investigating production incident INC-1234"
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                  />
                </div>

                {error && (
                  <div className="p-3 rounded border border-red-900/50 bg-red-900/10">
                    <p className="text-xs text-red-400">{error}</p>
                  </div>
                )}

                <div className="flex items-center gap-3 pt-2">
                  <Button variant="primary" size="sm" onClick={submit} loading={loading} icon={<ZapIcon />}>
                    Request Access
                  </Button>
                  <span className="text-xs text-muted">
                    Policy-checked · Time-bounded · Audited
                  </span>
                </div>
              </div>
            )}
          </CardBody>
        </Card>
      </div>

      <div className="col-span-2 space-y-4">
        <Card variant="accent">
          <CardBody>
            <h3 className="text-xs font-semibold text-primary uppercase tracking-wider mb-2">How JIT Works</h3>
            <ol className="space-y-2">
              {[
                ["Policy Check", "Access policy is evaluated against identity + resource"],
                ["Temporary Provision", "Access is granted for the requested duration"],
                ["Auto-Revocation", "Access is automatically revoked when time expires"],
                ["Audit Trail", "Every request is logged with full context"],
              ].map(([step, desc]) => (
                <li key={step} className="flex gap-2">
                  <span className="w-5 h-5 rounded-full bg-accent/10 border border-accent/30 flex items-center justify-center text-[0.6rem] font-bold text-accent flex-shrink-0 mt-0.5">
                    {step.charAt(0)}
                  </span>
                  <div>
                    <span className="text-xs font-semibold text-primary block">{step}</span>
                    <span className="text-[0.65rem] text-secondary">{desc}</span>
                  </div>
                </li>
              ))}
            </ol>
          </CardBody>
        </Card>

        <Card>
          <CardBody>
            <h3 className="text-xs font-semibold text-primary uppercase tracking-wider mb-2">Workflow Phases</h3>
            <div className="space-y-1.5">
              {[
                "Policy validation",
                "Temporary access grant",
                "Dual selector: timer / manual revoke",
                "Automatic cleanup",
              ].map((phase, i) => (
                <div key={phase} className="flex items-center gap-2">
                  <div className={`w-1.5 h-1.5 rounded-full ${i < 2 ? "bg-emerald-500" : "bg-amber-500"}`} />
                  <span className="text-xs text-secondary">{phase}</span>
                </div>
              ))}
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  )
}

// ── Grant / Revoke Tab ──────────────────────────────────────

function GrantRevokeTab() {
  const [tab, setTab] = useState<"grant" | "revoke">("grant")

  return (
    <div className="grid grid-cols-2 gap-6">
      <Card variant={tab === "grant" ? "accent" : "default"}>
        <CardHeader>
          <button
            className="w-full text-left"
            onClick={() => setTab("grant")}
          >
            <h2 className="text-sm font-semibold text-primary">Grant Access</h2>
            <p className="text-xs text-secondary mt-0.5">Permanently grant a role or entitlement to an identity</p>
          </button>
        </CardHeader>
        <CardBody>
          <code className="block p-3 rounded bg-white/[0.02] border border-border text-xs font-mono text-secondary">
            POST /api/v1/access/grant{"\n"}
            {"{"}
              "identity_id": "...",
              "resource_id": "...",
              "role_id": "...",
              "reason": "..."
            {"}"}
          </code>
        </CardBody>
      </Card>

      <Card variant={tab === "revoke" ? "accent" : "default"}>
        <CardHeader>
          <button
            className="w-full text-left"
            onClick={() => setTab("revoke")}
          >
            <h2 className="text-sm font-semibold text-primary">Revoke Access</h2>
            <p className="text-xs text-secondary mt-0.5">Immediately revoke access and trigger cascade revocation</p>
          </button>
        </CardHeader>
        <CardBody>
          <code className="block p-3 rounded bg-white/[0.02] border border-border text-xs font-mono text-secondary">
            POST /api/v1/access/revoke{"\n"}
            {"{"}
              "identity_id": "...",
              "entitlement_id": "...",
              "reason": "..."
            {"}"}
          </code>
        </CardBody>
      </Card>
    </div>
  )
}
