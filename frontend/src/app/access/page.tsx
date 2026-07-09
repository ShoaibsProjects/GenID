"use client"

import { useState } from "react"
import { checkAccess as apiCheckAccess } from "@/lib/api"

export default function AccessPage() {
  const [identityId, setIdentityId] = useState("")
  const [resourceId, setResourceId] = useState("")
  const [result, setResult] = useState<any>(null)

  async function checkAccess() {
    try {
      const res = await apiCheckAccess({ identity_id: identityId, resource_id: resourceId, action: "read" })
      setResult(res)
    } catch (e: any) {
      setResult({ error: e.message })
    }
  }

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-white">Access Control</h1>
        <p className="text-sm text-gray-400 mt-1">Check, grant, and revoke access permissions</p>
      </div>

      <div className="grid grid-cols-2 gap-6">
        <div className="glass-card p-6">
          <h2 className="text-sm font-semibold text-gray-200 mb-4">Check Access</h2>
          <div className="space-y-3">
            <input className="input w-full" placeholder="Identity ID" value={identityId} onChange={(e) => setIdentityId(e.target.value)} />
            <input className="input w-full" placeholder="Resource ID" value={resourceId} onChange={(e) => setResourceId(e.target.value)} />
            <button className="btn-primary text-sm" onClick={checkAccess}>Check</button>
          </div>
          {result && (
            <div className="mt-4 p-3 rounded-lg bg-surface-100/50">
              <pre className="text-xs text-gray-300">{JSON.stringify(result, null, 2)}</pre>
            </div>
          )}
        </div>

        <div className="glass-card p-6">
          <h2 className="text-sm font-semibold text-gray-200 mb-4">Quick Actions</h2>
          <div className="space-y-3">
            <p className="text-xs text-gray-500">Use the API to grant or revoke access:</p>
            <code className="block p-3 rounded bg-gray-900 text-xs text-gray-300">
              POST /api/v1/access/grant {"{"}"identity_id":"...","resource_id":"...","role_id":"...","reason":"..."{"}"}
            </code>
            <code className="block p-3 rounded bg-gray-900 text-xs text-gray-300 mt-2">
              POST /api/v1/access/revoke {"{"}"identity_id":"...","entitlement_id":"...","reason":"..."{"}"}
            </code>
          </div>
        </div>
      </div>
    </div>
  )
}
