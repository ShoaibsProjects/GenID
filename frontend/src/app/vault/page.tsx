"use client"
import { useState, useEffect, useCallback } from "react"

interface SecretEntry { id: string; name: string; secret_type: string; created_at: string; updated_at: string }

export default function VaultPage() {
  const [secrets, setSecrets] = useState<SecretEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [showAdd, setShowAdd] = useState(false)
  const [showRetrieve, setShowRetrieve] = useState<string | null>(null)
  const [retrieved, setRetrieved] = useState<any>(null)
  const [form, setForm] = useState({ name: "", value: "", secret_type: "api_key" })
  const [search, setSearch] = useState("")

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await fetch("/api/v1/vault/secrets")
      const data = await res.json()
      setSecrets(data.secrets || [])
    } catch { setSecrets([]) } finally { setLoading(false) }
  }, [])

  useEffect(() => { load() }, [load])

  const filtered = search ? secrets.filter(s => s.name?.toLowerCase().includes(search.toLowerCase())) : secrets

  async function handleStore() {
    try {
      await fetch("/api/v1/vault/secrets", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(form) })
      setShowAdd(false)
      setForm({ name: "", value: "", secret_type: "api_key" })
      load()
    } catch (e: any) { alert("Store failed: " + e.message) }
  }

  async function handleRetrieve(id: string) {
    setShowRetrieve(id)
    try {
      const res = await fetch(`/api/v1/vault/secrets/${id}`)
      const data = await res.json()
      setRetrieved(data)
    } catch { setRetrieved({ error: "Retrieve failed" }) }
  }

  async function handleDelete(id: string) {
    if (!confirm("Permanently delete this secret?")) return
    try { await fetch(`/api/v1/vault/secrets/${id}`, { method: "DELETE" }); load() } catch (e: any) { alert(e.message) }
  }

  const SECRET_TYPES = ["api_key", "database_password", "tls_cert", "oauth_token", "ssh_key", "encryption_key", "other"]

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div><h1 className="text-2xl font-bold text-white">Vault</h1><p className="text-sm text-gray-400 mt-1">{secrets.length} secrets encrypted with AES-256-GCM</p></div>
        <div className="flex gap-2"><button className="btn-secondary text-xs px-3 py-1.5" onClick={load}>Refresh</button><button className="btn-primary text-xs px-3 py-1.5" onClick={() => setShowAdd(true)}>+ Store Secret</button></div>
      </div>

      <div className="flex gap-3 items-center">
        <div className="relative flex-1 max-w-md"><input className="input text-sm py-1.5 pl-8 w-full" placeholder="Search secrets..." value={search} onChange={e => setSearch(e.target.value)} /><svg className="absolute left-2.5 top-2 w-3.5 h-3.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg></div>
      </div>

      <div className="glass-card overflow-hidden">
        {loading ? <div className="p-12 text-center text-gray-500">Loading...</div> : filtered.length === 0 ? <div className="p-12 text-center text-gray-500">
          <p className="mb-2">No secrets stored</p><p className="text-xs text-gray-600">Store API keys, passwords, certificates, and tokens encrypted with AES-256-GCM</p></div> : (
          <table className="w-full"><thead><tr className="border-b border-gray-800">
            <th className="text-left py-2.5 px-3 text-xs text-gray-500 uppercase">Name</th><th className="text-left py-2.5 px-3 text-xs text-gray-500 uppercase">Type</th><th className="text-left py-2.5 px-3 text-xs text-gray-500 uppercase">Created</th><th className="text-right py-2.5 px-3 text-xs text-gray-500 uppercase">Actions</th>
          </tr></thead><tbody className="divide-y divide-gray-800/50">
            {filtered.map(s => (
              <tr key={s.id} className="hover:bg-surface-100/30">
                <td className="py-2 px-3 text-sm text-gray-200 font-mono">{s.name}</td>
                <td className="py-2 px-3"><span className="px-2 py-0.5 rounded-full text-xs border bg-purple-500/10 text-purple-400 border-purple-500/30">{(s.secret_type || "api_key").replace("_", " ")}</span></td>
                <td className="py-2 px-3 text-xs text-gray-400 font-mono">{s.created_at ? new Date(s.created_at).toLocaleDateString() : "-"}</td>
                <td className="py-2 px-3 text-right space-x-1">
                  <button className="text-xs text-brand-400 hover:text-brand-300" onClick={() => handleRetrieve(s.id)}>View</button>
                  <button className="text-xs text-red-400 hover:text-red-300" onClick={() => handleDelete(s.id)}>Del</button>
                </td>
              </tr>
            ))}
          </tbody></table>
        )}
      </div>

      <div className="glass-card p-4"><h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-2">Encryption</h3><div className="text-sm text-gray-400 space-y-1">
        <p><span className="text-brand-400">AES-256-GCM</span> authenticated encryption with random nonces via crypto/rand</p>
        <p>All secrets encrypted at rest — decryption requires the master key</p>
        <p>Vault persisted to disk with <span className="font-mono">0600</span> permissions</p>
      </div></div>

      {showAdd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setShowAdd(false)}>
          <div className="w-full max-w-md glass-card p-6 space-y-4" onClick={e => e.stopPropagation()}>
            <div className="flex justify-between"><h2 className="text-lg font-semibold text-white">Store Secret</h2><button className="text-gray-400 hover:text-white text-xl" onClick={() => setShowAdd(false)}>&times;</button></div>
            <div className="space-y-3">
              <div><label className="text-xs text-gray-400 block mb-0.5">Name</label><input className="input text-sm py-1.5 font-mono" value={form.name} onChange={e => setForm({...form, name: e.target.value})} /></div>
              <div><label className="text-xs text-gray-400 block mb-0.5">Type</label><select className="input text-sm py-1.5" value={form.secret_type} onChange={e => setForm({...form, secret_type: e.target.value})}>{SECRET_TYPES.map(t => <option key={t} value={t}>{t.replace("_", " ")}</option>)}</select></div>
              <div><label className="text-xs text-gray-400 block mb-0.5">Value</label><textarea className="input text-sm py-1.5 h-24 font-mono" value={form.value} onChange={e => setForm({...form, value: e.target.value})} /></div>
            </div>
            <div className="flex gap-2 justify-end pt-2"><button className="btn-secondary text-xs px-4 py-2" onClick={() => setShowAdd(false)}>Cancel</button><button className="btn-primary text-xs px-4 py-2" onClick={handleStore}>Encrypt & Store</button></div>
          </div>
        </div>
      )}

      {showRetrieve && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => { setShowRetrieve(null); setRetrieved(null) }}>
          <div className="w-full max-w-2xl glass-card p-6 space-y-4" onClick={e => e.stopPropagation()}>
            <div className="flex justify-between"><h2 className="text-lg font-semibold text-white">Retrieved Secret</h2><button className="text-gray-400 hover:text-white text-xl" onClick={() => { setShowRetrieve(null); setRetrieved(null) }}>&times;</button></div>
            {retrieved?.error ? <p className="text-red-400">{retrieved.error}</p> : (
              <div className="space-y-3">
                <div className="p-3 rounded bg-surface-100/30"><span className="text-gray-500 block text-xs uppercase tracking-wider">ID</span><span className="text-gray-200 font-mono text-sm">{retrieved?.id}</span></div>
                <div className="p-3 rounded bg-surface-100/30"><span className="text-gray-500 block text-xs uppercase tracking-wider">Name</span><span className="text-gray-200 font-mono text-sm">{retrieved?.name}</span></div>
                <div className="p-3 rounded bg-surface-100/30"><span className="text-gray-500 block text-xs uppercase tracking-wider">Value (Decrypted)</span><pre className="text-amber-400 text-sm font-mono whitespace-pre-wrap break-all mt-1">{retrieved?.value}</pre></div>
                <div className="p-3 rounded bg-amber-900/10 border border-amber-900/30 text-amber-400 text-xs">This value was decrypted using AES-256-GCM. Do not share or log it.</div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
