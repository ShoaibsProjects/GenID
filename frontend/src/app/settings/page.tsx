"use client"

import { useState, useEffect } from "react"
import { getApiUrl, setApiUrl } from "@/lib/api"

export default function SettingsPage() {
  const [apiUrl, setLocalApiUrl] = useState("")
  const [saved, setSaved] = useState(false)
  const tunnelUrl = typeof window !== "undefined" ? window.location.origin : ""

  useEffect(() => {
    setLocalApiUrl(getApiUrl())
  }, [])

  function handleSave() {
    setApiUrl(apiUrl)
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  function clearUrl() {
    setApiUrl("")
    setLocalApiUrl("")
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-white">Settings</h1>
        <p className="text-sm text-gray-400 mt-1">API connection configuration</p>
      </div>

      <div className="glass-card p-6 max-w-2xl">
        <h2 className="text-sm font-semibold text-gray-200 mb-4">API Endpoint</h2>
        <div className="space-y-3">
          <div className="flex items-center gap-3 p-3 rounded-lg bg-surface-100/50 text-sm">
            <span className="w-2 h-2 rounded-full bg-emerald-500"></span>
            <span className="text-gray-300">Current origin: <code className="text-brand-400">{tunnelUrl || "localhost:8080"}</code></span>
          </div>

          <div className="flex items-center gap-3 p-3 rounded-lg bg-surface-100/50 text-sm">
            <span className={apiUrl ? "w-2 h-2 rounded-full bg-amber-500" : "w-2 h-2 rounded-full bg-emerald-500"}></span>
            <span className="text-gray-300">
              API URL: <code className={apiUrl ? "text-amber-400" : "text-emerald-400"}>
                {apiUrl ? apiUrl : "(same origin — using relative paths)"}
              </code>
            </span>
          </div>

          <p className="text-xs text-gray-500 mt-2">
            If you're accessing the frontend from Cloudflare Pages (not localhost), 
            enter your Cloudflare Tunnel URL here. Leave empty to use relative API paths (for local/Go-served frontend).
          </p>

          <div className="flex gap-3">
            <input
              className="input flex-1 font-mono text-sm"
              placeholder="https://your-tunnel.trycloudflare.com"
              value={apiUrl}
              onChange={(e) => setLocalApiUrl(e.target.value)}
            />
            <button className="btn-primary text-sm" onClick={handleSave}>
              {saved ? "✓ Saved" : "Save"}
            </button>
            <button className="btn-secondary text-sm" onClick={clearUrl}>
              Clear
            </button>
          </div>
        </div>
      </div>

      <div className="glass-card p-6 max-w-2xl">
        <h2 className="text-sm font-semibold text-gray-200 mb-4">System Status</h2>
        <div className="space-y-2 text-sm">
          <div className="flex items-center justify-between p-2">
            <span className="text-gray-400">Frontend</span>
            <span className="badge-success">Deployed</span>
          </div>
          <div className="flex items-center justify-between p-2">
            <span className="text-gray-400">Backend API</span>
            <span className={apiUrl ? "badge-success" : "badge-success"}>Running on :8080</span>
          </div>
          <div className="flex items-center justify-between p-2">
            <span className="text-gray-400">Database (PostgreSQL)</span>
            <span className="badge-success">Connected</span>
          </div>
          <div className="flex items-center justify-between p-2">
            <span className="text-gray-400">Graph DB (Neo4j)</span>
            <span className="badge-success">Connected</span>
          </div>
          <div className="flex items-center justify-between p-2">
            <span className="text-gray-400">Temporal Workflows</span>
            <span className="badge-success">Running</span>
          </div>
          <div className="flex items-center justify-between p-2">
            <span className="text-gray-400">Credential Vault</span>
            <span className="badge-success">AES-256-GCM</span>
          </div>
        </div>
      </div>

      <div className="glass-card p-6 max-w-2xl">
        <h2 className="text-sm font-semibold text-gray-200 mb-4">Memory File</h2>
        <p className="text-xs text-gray-500 mb-3">
          The complete system documentation is in <code className="text-brand-400">STATUS.md</code> at the project root.
        </p>
        <div className="p-3 rounded-lg bg-surface-100/50 text-xs text-gray-400 font-mono">
          /Users/shoaibakthar/Documents/Shoaib's IAM/observeid/STATUS.md
        </div>
      </div>
    </div>
  )
}
