"use client"

export default function PoliciesPage() {
  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-white">Policies</h1>
        <p className="text-sm text-gray-400 mt-1">Cedar-based authorization policies</p>
      </div>
      <div className="glass-card p-12 text-center text-gray-500">
        <p className="mb-2">Policy management coming soon</p>
        <p className="text-xs text-gray-600">Cedar policies are defined in the backend at /policies/*.cedar</p>
      </div>
    </div>
  )
}
