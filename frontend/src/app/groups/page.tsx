"use client"

import { useState, useEffect } from "react"
import { fetchGroups, createGroup, deleteGroup, assignRole, removeRole } from "@/lib/api"

export default function GroupsPage() {
  const [groups, setGroups] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [form, setForm] = useState({ name: "", description: "", role_type: "custom" })

  function load() {
    setLoading(true)
    fetchGroups()
      .then((d) => setGroups(d.groups || []))
      .catch(() => {})
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  async function handleCreate() {
    try {
      await createGroup({ ...form, tenant_id: "default" })
      setShowForm(false)
      setForm({ name: "", description: "", role_type: "custom" })
      load()
    } catch (e: any) {
      alert("Create failed: " + e.message)
    }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this group?")) return
    try {
      await deleteGroup(id)
      load()
    } catch (e: any) {
      alert("Delete failed: " + e.message)
    }
  }

  return (
    <div className="space-y-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Groups & Roles</h1>
          <p className="text-sm text-gray-400 mt-1">Manage RBAC groups and role assignments</p>
        </div>
        <button className="btn-primary text-sm" onClick={() => setShowForm(!showForm)}>
          {showForm ? "Cancel" : "Create Group"}
        </button>
      </div>

      {showForm && (
        <div className="glass-card p-6">
          <h2 className="text-sm font-semibold text-gray-200 mb-4">New Group</h2>
          <div className="grid grid-cols-3 gap-4">
            <input className="input" placeholder="Group Name" value={form.name} onChange={(e) => setForm({...form, name: e.target.value})} />
            <input className="input" placeholder="Description" value={form.description} onChange={(e) => setForm({...form, description: e.target.value})} />
            <select className="input" value={form.role_type} onChange={(e) => setForm({...form, role_type: e.target.value})}>
              <option value="team">Team</option>
              <option value="department">Department</option>
              <option value="custom">Custom</option>
            </select>
          </div>
          <button className="btn-primary text-sm mt-4" onClick={handleCreate}>Create</button>
        </div>
      )}

      <div className="glass-card overflow-hidden">
        {loading ? (
          <div className="p-12 text-center text-gray-500">Loading groups...</div>
        ) : groups.length === 0 ? (
          <div className="p-12 text-center text-gray-500">No groups created yet</div>
        ) : (
          <table className="w-full">
            <thead>
              <tr className="border-b border-gray-800">
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Name</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Type</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Entitlements</th>
                <th className="text-left py-3 px-4 text-xs font-medium text-gray-500 uppercase">Active</th>
                <th className="text-right py-3 px-4 text-xs font-medium text-gray-500 uppercase">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-800/50">
              {groups.map((g: any) => (
                <tr key={g.uuid} className="hover:bg-surface-100/50 transition-colors">
                  <td className="py-3 px-4">
                    <p className="text-sm font-medium text-white">{g.name}</p>
                    <p className="text-xs text-gray-500">{g.description || "-"}</p>
                  </td>
                  <td className="py-3 px-4 text-sm text-gray-400">{g.role_type || "-"}</td>
                  <td className="py-3 px-4 text-sm text-gray-400">{g.entitlement_count || "0"}</td>
                  <td className="py-3 px-4">
                    <span className={g.is_active === "true" ? "badge-success" : "badge-danger"}>
                      {g.is_active === "true" ? "Active" : "Inactive"}
                    </span>
                  </td>
                  <td className="py-3 px-4 text-right">
                    <button className="btn-ghost text-xs" onClick={() => handleDelete(g.uuid)}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  )
}
