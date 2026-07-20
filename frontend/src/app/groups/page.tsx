"use client"
import { useState, useEffect, useCallback } from "react"

export default function GroupsPage() {
  const [groups, setGroups] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState("")
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ name: "", description: "", role_type: "custom" })

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await fetch("/graphql", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ query: "{ roles { id tenantId name description roleType isAutoAssigned approvalRequired maxDurationHours isActive } groups { id tenantId name description } }" }) })
      const d = await res.json()
      const roles = d?.data?.roles || []
      const grps = d?.data?.groups || []
      setGroups([...roles.map((r: any) => ({...r, kind: "role"})), ...grps.map((g: any) => ({...g, kind: "group"}))])
    } catch { setGroups([]) } finally { setLoading(false) }
  }, [])

  useEffect(() => { load() }, [load])

  const filtered = search ? groups.filter((g: any) => g.name?.toLowerCase().includes(search.toLowerCase()) || g.description?.toLowerCase().includes(search.toLowerCase())) : groups

  async function handleCreate() {
    try {
      const endpoint = "/api/v1/groups"
      await fetch(endpoint, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(form) })
      setShowAdd(false)
      load()
    } catch (e: any) { alert("Create failed: " + e.message) }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this group?")) return
    try { await fetch(`/api/v1/groups/${id}`, { method: "DELETE" }); load() } catch (e: any) { alert(e.message) }
  }

  const ROLE_TYPES: Record<string, string> = { custom: "Custom", built_in: "Built-in", business: "Business", technical: "Technical", admin: "Administrative" }
  const TYPE_COLORS: Record<string, string> = { custom: "text-purple-400 bg-purple-500/10", built_in: "text-blue-400 bg-blue-500/10", business: "text-emerald-400 bg-emerald-500/10", technical: "text-amber-400 bg-amber-500/10", admin: "text-red-400 bg-red-500/10" }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div><h1 className="text-2xl font-bold text-white">Roles & Groups</h1><p className="text-sm text-gray-400 mt-1">{groups.length} roles and groups</p></div>
        <div className="flex gap-2"><button className="btn-secondary text-xs px-3 py-1.5" onClick={load}>Refresh</button><button className="btn-primary text-xs px-3 py-1.5" onClick={() => setShowAdd(true)}>+ Add Group</button></div>
      </div>

      <div className="flex gap-3 items-center">
        <div className="relative flex-1 max-w-md"><input className="input text-sm py-1.5 pl-8 w-full" placeholder="Search roles and groups..." value={search} onChange={e => setSearch(e.target.value)} /><svg className="absolute left-2.5 top-2 w-3.5 h-3.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg></div>
      </div>

      <div className="glass-card overflow-hidden">
        {loading ? <div className="p-12 text-center text-gray-500">Loading...</div> : filtered.length === 0 ? <div className="p-12 text-center text-gray-500">No roles or groups found</div> : (
          <table className="w-full"><thead><tr className="border-b border-gray-800">
            <th className="text-left py-2.5 px-3 text-xs text-gray-500 uppercase">Name</th><th className="text-left py-2.5 px-3 text-xs text-gray-500 uppercase">Kind</th><th className="text-left py-2.5 px-3 text-xs text-gray-500 uppercase">Type</th><th className="text-left py-2.5 px-3 text-xs text-gray-500 uppercase">Active</th><th className="text-left py-2.5 px-3 text-xs text-gray-500 uppercase">Description</th><th className="text-right py-2.5 px-3 text-xs text-gray-500 uppercase w-20">Actions</th>
          </tr></thead><tbody className="divide-y divide-gray-800/50">
            {filtered.map((g: any, i: number) => (
              <tr key={g.id || i} className="hover:bg-surface-100/30">
                <td className="py-2 px-3 text-sm text-gray-200 font-medium">{g.name}</td>
                <td className="py-2 px-3"><span className={`px-2 py-0.5 rounded-full text-xs border ${g.kind === "role" ? "text-blue-400 bg-blue-500/10 border-blue-500/30" : "text-emerald-400 bg-emerald-500/10 border-emerald-500/30"}`}>{g.kind}</span></td>
                <td className="py-2 px-3">{(g.roleType || g.role_type) ? <span className={`px-2 py-0.5 rounded text-xs ${TYPE_COLORS[g.roleType || g.role_type] || "text-gray-400 bg-gray-500/10"}`}>{ROLE_TYPES[g.roleType || g.role_type] || g.roleType || g.role_type}</span> : <span className="text-xs text-gray-500">-</span>}</td>
                <td className="py-2 px-3"><span className={`px-2 py-0.5 rounded text-xs ${(g.isActive !== false && g.status !== "inactive") ? "text-emerald-400 bg-emerald-500/10" : "text-gray-400 bg-gray-500/10"}`}>{(g.isActive !== false && g.status !== "inactive") ? "Yes" : "No"}</span></td>
                <td className="py-2 px-3 text-sm text-gray-400 truncate max-w-[300px]">{g.description || "-"}</td>
                <td className="py-2 px-3 text-right"><button className="text-xs text-red-400 hover:text-red-300" onClick={() => handleDelete(g.id)}>Del</button></td>
              </tr>
            ))}
          </tbody></table>
        )}
      </div>

      {showAdd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setShowAdd(false)}>
          <div className="w-full max-w-md glass-card p-6 space-y-4" onClick={e => e.stopPropagation()}>
            <div className="flex justify-between"><h2 className="text-lg font-semibold text-white">Add Group</h2><button className="text-gray-400 hover:text-white text-xl" onClick={() => setShowAdd(false)}>&times;</button></div>
            <div className="space-y-3">
              <div><label className="text-xs text-gray-400 block mb-0.5">Name</label><input className="input text-sm py-1.5" value={form.name} onChange={e => setForm({...form, name: e.target.value})} /></div>
              <div><label className="text-xs text-gray-400 block mb-0.5">Description</label><input className="input text-sm py-1.5" value={form.description} onChange={e => setForm({...form, description: e.target.value})} /></div>
            </div>
            <div className="flex gap-2 justify-end pt-2"><button className="btn-secondary text-xs px-4 py-2" onClick={() => setShowAdd(false)}>Cancel</button><button className="btn-primary text-xs px-4 py-2" onClick={handleCreate}>Create</button></div>
          </div>
        </div>
      )}
    </div>
  )
}
