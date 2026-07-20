"use client"
import { useState, useEffect, useCallback } from "react"

interface Role {
  id: string; name: string; description: string; role_type: string
  is_active: boolean; is_auto_assigned: boolean; approval_required: boolean
  member_count: number; entitlement_count: number; created_at: string
}

interface Member { identity_id: string; display_name: string; email: string; assigned_at: string; source: string }
interface Entitlement { app_name: string; permission_level: string; entitlement_type: string }

const ROLE_TYPES: Record<string, {color: string; label: string}> = {
  technical: {color: "#F59E0B", label: "Technical"},
  business: {color: "#60A5FA", label: "Business"},
  admin: {color: "#EF4444", label: "Administrative"},
  built_in: {color: "#34D399", label: "Built-in"},
  custom: {color: "#A78BFA", label: "Custom"},
}

export default function GroupsPage() {
  const [roles, setRoles] = useState<Role[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState("")
  const [roleFilter, setRoleFilter] = useState("")
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ name: "", description: "", role_type: "custom", is_auto_assigned: false, approval_required: false })

  // Detail
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [detail, setDetail] = useState<any>(null)
  const [detailTab, setDetailTab] = useState<"members" | "entitlements">("members")
  const [detailLoad, setDetailLoad] = useState(false)

  // Assign role
  const [showAssign, setShowAssign] = useState(false)
  const [assignSearch, setAssignSearch] = useState("")
  const [assignResults, setAssignResults] = useState<any[]>([])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams()
      if (search) params.set("search", search)
      if (roleFilter) params.set("role_type", roleFilter)
      const res = await fetch("/api/v1/groups?" + params.toString())
      const d = await res.json()
      setRoles(d.groups || [])
    } catch { setRoles([]) } finally { setLoading(false) }
  }, [search, roleFilter])

  useEffect(() => { load() }, [load])

  // Load role detail
  async function openDetail(id: string) {
    setSelectedId(id); setDetailTab("members"); setDetailLoad(true)
    try {
      const res = await fetch(`/api/v1/groups/${id}`)
      const d = await res.json()
      setDetail(d)
    } catch { setDetail(null) } finally { setDetailLoad(false) }
  }

  // Create role
  async function handleCreate() {
    if (!form.name.trim()) return
    try {
      await fetch("/api/v1/groups", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(form) })
      setShowAdd(false); load()
    } catch (e: any) { alert("Create failed: " + e.message) }
  }

  // Delete role
  async function handleDelete(id: string) {
    if (!confirm("Delete this role?")) return
    try { await fetch(`/api/v1/groups/${id}`, { method: "DELETE" }); if (selectedId === id) setSelectedId(null); load() } catch (e: any) { alert(e.message) }
  }

  // Assign role to identity
  async function handleAssign(identityId: string) {
    if (!selectedId) return
    try {
      await fetch("/api/v1/roles/assign", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ identity_id: identityId, role_id: selectedId, assigned_by: "admin", source: "direct" }) })
      setShowAssign(false)
      openDetail(selectedId) // refresh detail
    } catch (e: any) { alert("Assign failed: " + e.message) }
  }

  // Remove role from identity
  async function handleRemoveMember(identityId: string) {
    if (!selectedId) return
    try {
      await fetch("/api/v1/roles/remove", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ identity_id: identityId, role_id: selectedId }) })
      openDetail(selectedId) // refresh
    } catch (e: any) { alert("Remove failed: " + e.message) }
  }

  // Search identities for assignment
  async function searchIdentities() {
    try {
      const params = new URLSearchParams({ search: assignSearch, limit: "20" })
      const res = await fetch("/api/v1/identities?" + params.toString())
      const d = await res.json()
      setAssignResults(d.identities || [])
    } catch { setAssignResults([]) }
  }

  useEffect(() => { if (showAssign && assignSearch) { searchIdentities() } }, [assignSearch, showAssign])

  const filtered = roles

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Roles & Groups</h1>
          <p className="text-sm text-gray-400 mt-1">{roles.length} roles · manage access through role-based assignments</p>
        </div>
        <div className="flex gap-2">
          <button className="btn-secondary text-xs px-3 py-1.5" onClick={load}>Refresh</button>
          <button className="btn-primary text-xs px-3 py-1.5" onClick={() => setShowAdd(true)}>+ Create Role</button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex gap-3 items-center flex-wrap">
        <div className="relative flex-1 max-w-xs">
          <input className="input text-sm py-1.5 pl-8 w-full" placeholder="Search roles..." value={search} onChange={e => setSearch(e.target.value)} />
          <svg className="absolute left-2.5 top-2 w-3.5 h-3.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg>
        </div>
        <span className="text-xs text-gray-500">Type:</span>
        {Object.entries(ROLE_TYPES).map(([k, v]) => (
          <button key={k} onClick={() => setRoleFilter(roleFilter === k ? "" : k)} className={`px-2 py-0.5 rounded text-xs font-medium ${roleFilter === k ? "text-amber-400 bg-amber-500/10 ring-1 ring-amber-500/30" : "text-gray-400 bg-gray-800 hover:text-gray-200"}`}>{v.label}</button>
        ))}
      </div>

      {/* Role Cards */}
      <div className="grid grid-cols-1 gap-3">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading roles...</div>
        ) : filtered.length === 0 ? (
          <div className="p-12 text-center text-gray-500">No roles found</div>
        ) : (
          filtered.map(r => {
            const typeStyle = ROLE_TYPES[r.role_type] || ROLE_TYPES.custom
            return (
              <div key={r.id} className="glass-card p-4 cursor-pointer card-hover" onClick={() => openDetail(r.id)}>
                <div className="flex items-start justify-between">
                  <div className="flex items-start gap-3">
                    {/* Role icon */}
                    <div style={{ width: 40, height: 40, borderRadius: 10, background: `${typeStyle.color}15`, display: 'flex', alignItems: 'center', justifyContent: 'center', border: `1px solid ${typeStyle.color}30`, fontSize: 18 }}>
                      {r.role_type === "admin" ? "🛡" : r.role_type === "technical" ? "⚙" : r.role_type === "business" ? "📋" : "🔧"}
                    </div>
                    <div>
                      <h3 className="text-base font-semibold text-white">{r.name}</h3>
                      <div className="flex gap-2 mt-1 flex-wrap">
                        <span style={{ fontSize: 11, padding: '2px 8px', borderRadius: 100, background: `${typeStyle.color}10`, color: typeStyle.color, border: `1px solid ${typeStyle.color}30` }}>{typeStyle.label}</span>
                        {r.is_auto_assigned && <span style={{ fontSize: 11, padding: '2px 8px', borderRadius: 100, background: 'rgba(52,211,153,0.08)', color: '#34D399', border: '1px solid rgba(52,211,153,0.2)' }}>Auto</span>}
                        {r.approval_required && <span style={{ fontSize: 11, padding: '2px 8px', borderRadius: 100, background: 'rgba(239,68,68,0.08)', color: '#EF4444', border: '1px solid rgba(239,68,68,0.2)' }}>Approval</span>}
                        {!r.is_active && <span style={{ fontSize: 11, padding: '2px 8px', borderRadius: 100, background: 'rgba(255,255,255,0.03)', color: '#5C5C62', border: '1px solid rgba(255,255,255,0.06)' }}>Inactive</span>}
                      </div>
                      {r.description && <p className="text-sm text-gray-400 mt-1 line-clamp-2">{r.description}</p>}
                    </div>
                  </div>
                  <div className="flex items-center gap-4 text-xs text-gray-500" onClick={e => e.stopPropagation()}>
                    <div className="text-center"><div className="text-lg font-bold text-white">{r.member_count}</div><div>Members</div></div>
                    <div className="text-center"><div className="text-lg font-bold text-white">{r.entitlement_count}</div><div>Entitlements</div></div>
                    <button className="btn-secondary text-xs px-2 py-0.5" onClick={() => handleDelete(r.id)}>Delete</button>
                  </div>
                </div>
              </div>
            )
          })
        )}
      </div>

      {/* Role Detail Panel */}
      {selectedId && (
        <div className="fixed inset-0 z-50 flex justify-end">
          <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={() => { setSelectedId(null); setDetail(null) }} />
          <div className="relative z-10 w-full max-w-xl h-full overflow-y-auto card-obsidian" style={{ borderLeft: '1px solid rgba(255,255,255,0.06)' }} onClick={e => e.stopPropagation()}>
            <button className="absolute top-4 right-4 text-gray-400 hover:text-white text-xl z-20" onClick={() => { setSelectedId(null); setDetail(null) }}>&times;</button>

            {detailLoad ? <div className="p-8 text-center text-gray-500">Loading...</div> : detail ? (
              <div className="p-6 pt-12 space-y-5">
                {/* Header */}
                <div>
                  <h2 className="text-xl font-bold text-white">{detail.role.name}</h2>
                  <div className="flex gap-2 mt-2">
                    <span style={{ fontSize: 11, padding: '2px 8px', borderRadius: 100, background: `${(ROLE_TYPES[detail.role.role_type] || ROLE_TYPES.custom).color}10`, color: (ROLE_TYPES[detail.role.role_type] || ROLE_TYPES.custom).color, border: `1px solid ${(ROLE_TYPES[detail.role.role_type] || ROLE_TYPES.custom).color}30` }}>{(ROLE_TYPES[detail.role.role_type] || ROLE_TYPES.custom).label}</span>
                    {detail.role.is_auto_assigned && <span className="px-2 py-0.5 rounded-full text-xs bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">Auto-Assigned</span>}
                    {detail.role.approval_required && <span className="px-2 py-0.5 rounded-full text-xs bg-red-500/10 text-red-400 border border-red-500/20">Approval Required</span>}
                  </div>
                  {detail.role.description && <p className="text-sm text-gray-400 mt-2">{detail.role.description}</p>}
                </div>

                {/* Tabs */}
                <div className="flex border-b" style={{ borderColor: 'rgba(255,255,255,0.06)' }}>
                  {(["members", "entitlements"] as const).map(t => (
                    <button key={t} onClick={() => setDetailTab(t)} className={`px-4 py-2 text-xs font-medium border-b-2 transition-colors ${detailTab === t ? "border-amber-500 text-amber-400" : "border-transparent text-gray-400 hover:text-gray-300"}`}>
                      {t === "members" ? `Members (${detail.role.member_count})` : `Entitlements (${detail.role.entitlement_count})`}
                    </button>
                  ))}
                </div>

                {/* Members Tab */}
                {detailTab === "members" && (
                  <div>
                    <div className="flex justify-between items-center mb-3">
                      <span className="text-xs text-gray-500">{detail.members.length} members</span>
                      <button className="btn-primary text-xs px-3 py-1" onClick={() => { setShowAssign(true); setAssignSearch(""); setAssignResults([]) }}>
                        + Assign Identity
                      </button>
                    </div>
                    {detail.members.length === 0 ? (
                      <p className="text-sm text-gray-500 py-4">No identities assigned to this role</p>
                    ) : (
                      <div className="space-y-1">
                        {detail.members.map((m: Member) => (
                          <div key={m.identity_id} className="flex items-center justify-between p-2 rounded hover:bg-white/[0.02]">
                            <div className="flex items-center gap-2">
                              <div className="w-7 h-7 rounded-full bg-amber-500/10 flex items-center justify-center text-xs font-bold text-amber-400">{(m.display_name || m.email)[0]?.toUpperCase()}</div>
                              <div>
                                <div className="text-sm text-gray-200">{m.display_name}</div>
                                <div className="text-xs text-gray-500 font-mono">{m.email}</div>
                              </div>
                            </div>
                            <div className="flex items-center gap-3">
                              <span className="text-xs text-gray-500">{m.source}</span>
                              <button className="text-xs text-red-400 hover:text-red-300" onClick={() => handleRemoveMember(m.identity_id)}>Remove</button>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}

                {/* Entitlements Tab */}
                {detailTab === "entitlements" && (
                  <div>
                    <span className="text-xs text-gray-500 block mb-3">{detail.entitlements.length} entitlements granted by this role</span>
                    {detail.entitlements.length === 0 ? (
                      <p className="text-sm text-gray-500 py-4">No entitlements linked to this role. Add entitlements to grant access when this role is assigned.</p>
                    ) : (
                      <div className="space-y-1">
                        {detail.entitlements.map((e: Entitlement, i: number) => (
                          <div key={i} className="flex items-center justify-between p-2 rounded hover:bg-white/[0.02]">
                            <span className="text-sm text-gray-200 font-mono">{e.app_name}</span>
                            <div className="flex gap-2">
                              <span className="text-xs text-gray-400 font-mono">{e.permission_level}</span>
                              <span className="px-1.5 py-0.5 rounded text-xs bg-purple-500/10 text-purple-400">{e.entitlement_type}</span>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            ) : null}
          </div>
        </div>
      )}

      {/* Create Role Modal */}
      {showAdd && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setShowAdd(false)}>
          <div className="w-full max-w-md glass-card p-6 space-y-4" onClick={e => e.stopPropagation()}>
            <div className="flex justify-between"><h2 className="text-lg font-semibold text-white">Create Role</h2><button className="text-gray-400 hover:text-white text-xl" onClick={() => setShowAdd(false)}>&times;</button></div>
            <div className="space-y-3">
              <div><label className="text-xs text-gray-400 block mb-0.5">Name *</label><input className="input text-sm py-1.5" value={form.name} onChange={e => setForm({...form, name: e.target.value})} /></div>
              <div><label className="text-xs text-gray-400 block mb-0.5">Description</label><input className="input text-sm py-1.5" value={form.description} onChange={e => setForm({...form, description: e.target.value})} /></div>
              <div><label className="text-xs text-gray-400 block mb-0.5">Role Type</label>
                <select className="input text-sm py-1.5" value={form.role_type} onChange={e => setForm({...form, role_type: e.target.value})}>
                  {Object.entries(ROLE_TYPES).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
                </select>
              </div>
              <div className="flex gap-3">
                <label className="flex items-center gap-2 text-sm text-gray-400 cursor-pointer">
                  <input type="checkbox" checked={form.is_auto_assigned} onChange={e => setForm({...form, is_auto_assigned: e.target.checked})} />
                  Auto-assign by attribute
                </label>
                <label className="flex items-center gap-2 text-sm text-gray-400 cursor-pointer">
                  <input type="checkbox" checked={form.approval_required} onChange={e => setForm({...form, approval_required: e.target.checked})} />
                  Requires approval
                </label>
              </div>
            </div>
            <div className="flex gap-2 justify-end pt-2"><button className="btn-secondary text-xs px-4 py-2" onClick={() => setShowAdd(false)}>Cancel</button><button className="btn-primary text-xs px-4 py-2" onClick={handleCreate}>Create</button></div>
          </div>
        </div>
      )}

      {/* Assign Identity Modal */}
      {showAssign && (
        <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setShowAssign(false)}>
          <div className="w-full max-w-lg glass-card p-6 space-y-4" onClick={e => e.stopPropagation()}>
            <div className="flex justify-between"><h2 className="text-lg font-semibold text-white">Assign Identity to Role</h2><button className="text-gray-400 hover:text-white text-xl" onClick={() => setShowAssign(false)}>&times;</button></div>
            <input className="input text-sm py-1.5" placeholder="Search by name or email..." value={assignSearch} onChange={e => setAssignSearch(e.target.value)} autoFocus />
            <div className="max-h-64 overflow-y-auto space-y-1">
              {assignResults.map((id: any) => (
                <div key={id.id} className="flex items-center justify-between p-2 rounded hover:bg-white/[0.02] cursor-pointer" onClick={() => handleAssign(id.id)}>
                  <div className="flex items-center gap-2">
                    <div className="w-7 h-7 rounded-full bg-amber-500/10 flex items-center justify-center text-xs font-bold text-amber-400">{(id.display_name || id.email)[0]?.toUpperCase()}</div>
                    <div><div className="text-sm text-gray-200">{id.display_name}</div><div className="text-xs text-gray-500">{id.email}</div></div>
                  </div>
                  <span className="text-xs text-amber-400">+ Assign</span>
                </div>
              ))}
              {assignSearch && assignResults.length === 0 && <p className="text-sm text-gray-500 py-4 text-center">No identities found</p>}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
