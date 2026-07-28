"use client"

import { useState, useEffect } from "react"

// Sample Cedar policy set — shown when the policies API is unavailable.
// Matches the policies seeded into the cedar_policies table.
const SAMPLE_CEDAR = `// ObserveID Cedar Policy Set (v1)
// Evaluation: forbid always wins; otherwise at least one permit must match.

// Engineers may read production AWS resources
permit(
    principal in Role::"Engineering",
    action in [Action::"read"],
    resource in Resource::"res-aws-prod"
);

// Nobody may touch the HR database
forbid(
    principal,
    action,
    resource in Resource::"res-hr-db"
);

// Nobody may touch the finance database
forbid(
    principal,
    action,
    resource in Resource::"res-finance-db"
);

// AI agents may read telemetry only, never write
permit(
    principal in Role::"ai_agent",
    action in [Action::"read"],
    resource in Resource::"res-telemetry"
);

forbid(
    principal in Role::"ai_agent",
    action in [Action::"write", Action::"admin", Action::"delete"],
    resource
);

// Platform admins — full access (break-glass)
permit(
    principal in Role::"platform-admin",
    action,
    resource
);`

export default function PoliciesPage() {
  const [policySource, setPolicySource] = useState<string>(SAMPLE_CEDAR)
  const [source, setSource] = useState<"api" | "fallback">("fallback")

  useEffect(() => {
    // Try the REST endpoint first; fall back to the hardcoded sample.
    fetch("/api/v1/policies")
      .then(async (r) => {
        if (!r.ok) throw new Error(String(r.status))
        const data = await r.json()
        const policies = data.policies || []
        if (policies.length > 0) {
          setPolicySource(policies.map((p: any) => p.policy_source || "").join("\n\n"))
          setSource("api")
        }
      })
      .catch(() => {
        // Endpoint doesn't exist yet — keep hardcoded sample
        setPolicySource(SAMPLE_CEDAR)
        setSource("fallback")
      })
  }, [])

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Policy Engine</h1>
          <p className="text-sm text-gray-400 mt-1">
            AWS Cedar policy-as-code — evaluated in-process on every access check
          </p>
        </div>
        <div className="flex items-center gap-2">
          {source === "fallback" && (
            <span className="badge-neutral">sample policies</span>
          )}
          <span className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/30">
            <span className="dot-live" />
            Hot Reload: Active (30s)
          </span>
        </div>
      </div>

      {/* Policy code block */}
      <div className="glass-card overflow-hidden">
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-gray-800">
          <span className="text-xs text-gray-500 font-mono">cedar_policies.cedar</span>
          <span className="text-xs text-gray-600 font-mono">{policySource.split("\n").length} lines</span>
        </div>
        <pre className="p-5 overflow-x-auto text-[0.8rem] leading-relaxed font-mono text-emerald-400 bg-black/40">
          {policySource}
        </pre>
      </div>

      {/* Evaluation rules */}
      <div className="glass-card p-4">
        <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-3">Evaluation Rules</h3>
        <div className="space-y-2 text-sm text-gray-400">
          <div className="flex gap-2"><span className="text-red-400 font-bold">1.</span> <span>Forbid policies are checked first — forbid always wins</span></div>
          <div className="flex gap-2"><span className="text-amber-400 font-bold">2.</span> <span>Permit policies are checked second — at least one must match</span></div>
          <div className="flex gap-2"><span className="text-blue-400 font-bold">3.</span> <span>If no policy matches → default deny</span></div>
          <div className="flex gap-2"><span className="text-emerald-400 font-bold">4.</span> <span>Policies hot-reload from PostgreSQL every 30 seconds</span></div>
        </div>
      </div>
    </div>
  )
}
