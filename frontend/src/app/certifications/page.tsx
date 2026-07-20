"use client"

export default function CertificationsPage() {
  const campaigns = [
    { id: "1", name: "Q3 2026 Quarterly Review", status: "pending", due_date: "2026-09-30", reviewers: 3, entries: 42 },
    { id: "2", name: "Privileged Access Review", status: "in_progress", due_date: "2026-08-15", reviewers: 2, entries: 18 },
    { id: "3", name: "New Hire Access Validation", status: "completed", due_date: "2026-07-01", reviewers: 1, entries: 5 },
  ]

  const STATUS_COLORS: Record<string, string> = {
    pending: "text-gray-400 bg-gray-500/10 border-gray-500/30",
    in_progress: "text-amber-400 bg-amber-500/10 border-amber-500/30",
    completed: "text-emerald-400 bg-emerald-500/10 border-emerald-500/30",
    overdue: "text-red-400 bg-red-500/10 border-red-500/30",
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div><h1 className="text-2xl font-bold text-white">Access Certifications</h1><p className="text-sm text-gray-400 mt-1">Periodic access reviews and recertification campaigns</p></div>
        <button className="btn-primary text-xs px-3 py-1.5">+ New Campaign</button>
      </div>

      <div className="grid grid-cols-3 gap-3">
        {[
          ["Total", campaigns.length, "text-white"],
          ["In Progress", campaigns.filter(c => c.status === "in_progress").length, "text-amber-400"],
          ["Completed", campaigns.filter(c => c.status === "completed").length, "text-emerald-400"],
        ].map(([label, val, color]) => (
          <div key={label as string} className="glass-card p-4"><span className="text-xs text-gray-500 uppercase">{label}</span><div className={`text-2xl font-bold ${color} mt-1`}>{val}</div></div>
        ))}
      </div>

      <div className="space-y-3">
        {campaigns.map(c => (
          <div key={c.id} className="glass-card p-4">
            <div className="flex items-start justify-between">
              <div>
                <h3 className="text-base font-semibold text-white">{c.name}</h3>
                <div className="flex gap-2 mt-1">
                  <span className={`px-2 py-0.5 rounded-full text-xs border ${STATUS_COLORS[c.status] || ""}`}>{c.status.replace("_", " ")}</span>
                  <span className="text-xs text-gray-500">Due: {c.due_date}</span>
                </div>
              </div>
              <div className="flex gap-4 text-xs text-gray-500">
                <span>{c.reviewers} reviewers</span>
                <span>{c.entries} entries</span>
              </div>
            </div>
            <div className="mt-3 h-2 rounded-full bg-gray-800 overflow-hidden">
              <div className={`h-full rounded-full transition-all ${
                c.status === "completed" ? "bg-emerald-500 w-full" : c.status === "in_progress" ? "bg-amber-500 w-1/2" : "bg-gray-600 w-0"
              }`} />
            </div>
          </div>
        ))}
      </div>

      <div className="glass-card p-4">
        <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-2">About Certifications</h3>
        <div className="text-sm text-gray-400 space-y-1">
          <p>Access certifications ensure that all entitlements are reviewed periodically by managers and resource owners.</p>
          <p>Campaigns auto-assign certifiers, send email reminders, track decisions, and enforce deadlines with escalation policies.</p>
          <p>The <span className="font-mono text-brand-400">certification_campaigns</span> and <span className="font-mono text-brand-400">certification_entries</span> tables in PostgreSQL manage the full lifecycle.</p>
        </div>
      </div>
    </div>
  )
}
