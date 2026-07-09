"use client"

export default function CertificationsPage() {
  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-white">Certification Campaigns</h1>
        <p className="text-sm text-gray-400 mt-1">Access reviews and recertification</p>
      </div>
      <div className="glass-card p-12 text-center text-gray-500">
        <p className="mb-2">No active campaigns</p>
        <p className="text-xs text-gray-600">Use the API to create certification campaigns when needed</p>
      </div>
    </div>
  )
}
