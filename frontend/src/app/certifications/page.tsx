"use client"

export default function CertificationsPage() {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Access Certifications</h1>
          <p className="text-sm text-gray-400 mt-1">Periodic access reviews and recertification campaigns</p>
        </div>
        <button className="btn-primary text-xs px-3 py-1.5">+ New Campaign</button>
      </div>

      <div className="grid grid-cols-3 gap-3">
        {[
          ["Total", 0, "text-white"],
          ["In Progress", 0, "text-amber-400"],
          ["Completed", 0, "text-emerald-400"],
        ].map(([label, val, color]) => (
          <div key={label as string} className="glass-card p-4">
            <span className="text-xs text-gray-500 uppercase">{label}</span>
            <div className={`text-2xl font-bold ${color} mt-1`}>{val}</div>
          </div>
        ))}
      </div>

      <div className="glass-card p-12 text-center">
        <div className="text-gray-500">
          <p className="text-sm">No certification campaigns yet.</p>
          <p className="text-xs text-gray-600 mt-1">Create a campaign to start periodic access reviews.</p>
        </div>
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
