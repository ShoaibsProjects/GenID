"use client"

import { useState, useEffect } from "react"

export default function AuditPage() {
  const [events, setEvents] = useState<any[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetch("/api/v1/caep/events")
      .then((r) => r.json())
      .then((d) => setEvents(d.events || []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-white">Audit & CAEP</h1>
        <p className="text-sm text-gray-400 mt-1">Security event monitoring and CAEP broadcasts</p>
      </div>
      <div className="glass-card overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading events...</div>
        ) : events.length === 0 ? (
          <div className="p-12 text-center text-gray-500">
            <p className="mb-2">No CAEP events recorded</p>
            <p className="text-xs text-gray-600">Events will appear when access revocations or emergency actions are triggered</p>
          </div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-800">
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Event Type</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Identity</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Timestamp</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800/50">
              {events.map((e: any) => (
                <tr key={e.id} className="hover:bg-surface-100/50">
                  <td className="py-3 px-4 text-sm text-white">{e.event_type}</td>
                  <td className="py-3 px-4 text-sm text-gray-400">{e.identity_id}</td>
                  <td className="py-3 px-4"><span className="badge-neutral">{e.delivery_status || "pending"}</span></td>
                  <td className="py-3 px-4 text-sm text-gray-400">{e.created_at || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
