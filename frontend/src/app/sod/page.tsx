"use client"

export default function SodPage() {
  const rules = [
    { id: "1", name: "Sensitive Access Conflict", conflicting_roles: ["Finance Approver", "Finance Auditor"], risk_level: "critical", description: "Same person cannot both approve and audit financial transactions" },
    { id: "2", name: "Developer-Production Access", conflicting_roles: ["Developer", "Prod Deployer"], risk_level: "high", description: "Developers cannot have direct production deployment access" },
    { id: "3", name: "Security Admin Conflict", conflicting_roles: ["Security Admin", "Audit Log Reader"], risk_level: "high", description: "Security admins should not be able to read audit logs they generate" },
    { id: "4", name: "HR Data Access", conflicting_roles: ["HR Manager", "Payroll Admin"], risk_level: "medium", description: "HR managers should be separate from payroll administrators" },
  ]

  const LEVEL_COLORS: Record<string, string> = {
    critical: "text-red-400 bg-red-500/10 border-red-500/30",
    high: "text-amber-400 bg-amber-500/10 border-amber-500/30",
    medium: "text-yellow-400 bg-yellow-500/10 border-yellow-500/30",
    low: "text-blue-400 bg-blue-500/10 border-blue-500/30",
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div><h1 className="text-2xl font-bold text-white">Separation of Duties</h1><p className="text-sm text-gray-400 mt-1">SoD policies prevent toxic combinations of access that create fraud or security risks</p></div>
        <button className="btn-primary text-xs px-3 py-1.5">+ Add Rule</button>
      </div>

      <div className="grid grid-cols-4 gap-3">
        {[
          ["Total Rules", rules.length, "text-white"],
          ["Critical", rules.filter(r => r.risk_level === "critical").length, "text-red-400"],
          ["High", rules.filter(r => r.risk_level === "high").length, "text-amber-400"],
          ["Medium", rules.filter(r => r.risk_level === "medium").length, "text-yellow-400"],
        ].map(([label, val, color]) => (
          <div key={label as string} className="glass-card p-4"><span className="text-xs text-gray-500 uppercase">{label}</span><div className={`text-2xl font-bold ${color} mt-1`}>{val}</div></div>
        ))}
      </div>

      <div className="space-y-3">
        {rules.map(r => (
          <div key={r.id} className="glass-card p-4">
            <div className="flex items-start justify-between">
              <div>
                <h3 className="text-base font-semibold text-white">{r.name}</h3>
                <span className={`px-2 py-0.5 rounded-full text-xs border ${LEVEL_COLORS[r.risk_level] || ""}`}>{r.risk_level}</span>
                <p className="text-sm text-gray-400 mt-2">{r.description}</p>
              </div>
            </div>
            <div className="mt-3 flex gap-2 flex-wrap">
              {r.conflicting_roles.map(cr => (
                <span key={cr} className="px-2 py-0.5 rounded-full text-xs bg-red-500/10 text-red-400 border border-red-500/30">{cr}</span>
              ))}
            </div>
          </div>
        ))}
      </div>

      <div className="glass-card p-4">
        <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-2">Violation Detection</h3>
        <div className="text-sm text-gray-400 space-y-1">
          <p>The <span className="font-mono text-brand-400">DetectSoDViolationsWorkflow</span> Temporal cron workflow scans for conflicting access combinations across all identities.</p>
          <p>When a violation is detected, the system flags the identity with a risk factor, triggers a CAEP event, and can auto-revoke the conflicting access if the risk score exceeds thresholds.</p>
          <p>SoD rules are evaluated at both access request time (preventive) and periodically (detective) via the Temporal scheduler.</p>
        </div>
      </div>
    </div>
  )
}
