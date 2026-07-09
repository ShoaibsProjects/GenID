"use client"

import { useState, useEffect } from "react"
import { fetchSecrets, storeSecret, deleteSecret } from "@/lib/api"

export default function VaultPage() {
  const [secrets, setSecrets] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: "", type: "connector_password", reference: "", value: "" })

  function load() {
    setLoading(true)
    fetchSecrets()
      .then((d) => setSecrets(d.secrets || []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  async function handleSave() {
    try {
      await storeSecret(form)
      setShowForm(false)
      setForm({ name: "", type: "connector_password", reference: "", value: "" })
      load()
    } catch (e: any) {
      alert("Error: " + e.message)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Permanently delete this secret?")) return
    try {
      await deleteSecret(id)
      load()
    } catch (e: any) {
      alert("Error: " + e.message)
    }
  }

  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Credential Vault</h1>
          <p className="text-sm text-gray-400 mt-1">Encrypted secret storage for connector credentials</p>
        </div>
        <button className="btn-primary text-sm" onClick={() => setShowForm(!showForm)}>
          {showForm ? "Cancel" : "Add Secret"}
        </button>
      </div>

      {showForm && (
        <div className="glass-card p-6 border border-brand-500/20">
          <div className="flex items-center gap-2 mb-4">
            <div className="w-6 h-6 rounded-full bg-emerald-600/20 flex items-center justify-center">
              <svg className="w-3.5 h-3.5 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
              </svg>
            </div>
            <h2 className="text-sm font-semibold text-gray-200">New Secret</h2>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <input className="input" placeholder="Secret Name" value={form.name} onChange={(e) => setForm({...form, name: e.target.value})} />
            <select className="input" value={form.type} onChange={(e) => setForm({...form, type: e.target.value})}>
              <option value="connector_password">Connector Password</option>
              <option value="client_secret">OAuth Client Secret</option>
              <option value="api_key">API Key</option>
              <option value="tls_cert">TLS Certificate</option>
              <option value="ssh_key">SSH Private Key</option>
              <option value="generic">Generic Secret</option>
            </select>
            <input className="input" placeholder="Reference ID (e.g. connector_123)" value={form.reference} onChange={(e) => setForm({...form, reference: e.target.value})} />
            <input className="input col-span-2" type="password" placeholder="Secret Value" value={form.value} onChange={(e) => setForm({...form, value: e.target.value})} />
          </div>
          <button className="btn-primary text-sm mt-4" onClick={handleSave}>
            Encrypt & Store
          </button>
        </div>
      )}

      <div className="glass-card overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading vault...</div>
        ) : secrets.length === 0 ? (
          <div className="p-12 text-center text-gray-500">
            <p className="mb-2 text-gray-400">No secrets stored</p>
            <p className="text-xs text-gray-600">AES-256-GCM encrypted vault for connector credentials</p>
          </div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-800">
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Name</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Type</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Reference</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Encrypted</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Created</th>
                <th className="text-right py-3 px-4 text-xs font-medium text-gray-500 uppercase">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800/50">
              {secrets.map((s: any) => (
                <tr key={s.id} className="hover:bg-surface-100/50 transition-colors">
                  <td className="py-3 px-4 text-sm font-medium text-white">{s.name}</td>
                  <td className="py-3 px-4 text-sm text-gray-400">{s.type}</td>
                  <td className="py-3 px-4 text-sm text-gray-400 max-w-[150px] truncate">{s.reference || "-"}</td>
                  <td className="py-3 px-4">
                    <span className="badge-success text-[10px]">AES-256-GCM</span>
                  </td>
                  <td className="py-3 px-4 text-sm text-gray-400">{new Date(s.created_at).toLocaleDateString()}</td>
                  <td className="py-3 px-4 text-right">
                    <button className="btn-ghost text-xs text-rose-400" onClick={() => handleDelete(s.id)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="glass-card p-4 bg-surface-100/30">
        <div className="flex items-center gap-2 text-xs text-gray-500">
          <svg className="w-4 h-4 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
          </svg>
          All secrets encrypted with AES-256-GCM before storage. Master key derived via SHA-256 from VAULT_MASTER_KEY environment variable.
        </div>
      </div>
    </div>
  )
}
