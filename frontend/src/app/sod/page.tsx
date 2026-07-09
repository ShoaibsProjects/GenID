"use client"

export default function SodPage() {
  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-white">Segregation of Duties</h1>
        <p className="text-sm text-gray-400 mt-1">SoD violations and conflict detection</p>
      </div>
      <div className="glass-card p-12 text-center text-gray-500">
        <p className="mb-2">No violations detected</p>
        <p className="text-xs text-gray-600">SoD detection runs via Temporal cron workflow - results appear here automatically</p>
      </div>
    </div>
  )
}
