"use client"
import { useState, useEffect, useCallback } from "react"
import Link from "next/link"
import { authFetch } from "@/lib/api"

interface Role {
  id: string; name: string; description: string; role_type: string
  is_active: boolean; is_auto_assigned: boolean; approval_required: boolean
  member_count: number; entitlement_count: number; created_at: string
}

const ROLE_STYLES: Record<string, { color: string; icon: string }> = {
  birthright: { color: "#34D399", icon: "✦" },
  it: { color: "#60A5FA", icon: "⚙" },
  admin: { color: "#EF4444", icon: "🛡" },
  technical: { color: "#F59E0B", icon: "⚙" },
  business: { color: "#60A5FA", icon: "📋" },
  built_in: { color: "#34D399", icon: "✦" },
  custom: { color: "#A78BFA", icon: "🔧" },
}

export default function PoliciesPage() {
  const [roles, setRoles] = useState<Role[]>([])
  const [loading, setLoading] = useState(true)
  const [activeTab, setActiveTab] = useState<"birthright" | "it">("birthright")

  const loadRoles = useCallback(async () => {
    setLoading(true)
    try {
      const res = await authFetch("/api/v1/groups")
      const d = await res.json()
      setRoles(d.groups || [])
    } catch { setRoles([]) } finally { setLoading(false) }
  }, [])

  useEffect(() => { loadRoles() }, [loadRoles])

  const birthrightRoles = roles.filter(r => r.is_auto_assigned || r.role_type === "built_in" || r.role_type === "business")
  const itRoles = roles.filter(r => !r.is_auto_assigned && r.role_type !== "built_in" && r.role_type !== "business")

  const displayRoles = activeTab === "birthright" ? birthrightRoles : itRoles

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-[var(--text-primary)]">Roles</h1>
          <p className="text-sm text-[var(--text-secondary)] mt-1">
            Birthright roles auto-assign by attribute. IT roles are manually managed.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Link href="/groups" className="btn-secondary text-xs px-3 py-1.5 no-underline">
            Manage All Roles →
          </Link>
        </div>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 p-1 rounded-xl bg-[var(--glass-1)] border border-[var(--obsidian-border)] w-fit">
        <button
          onClick={() => setActiveTab("birthright")}
          className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
            activeTab === "birthright"
              ? "bg-[var(--accent-dim)] text-[var(--accent)] border border-[var(--accent)]/30"
              : "text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
          }`}
        >
          <span className="mr-1.5">✦</span>
          Birthright Roles
          <span className="ml-2 px-1.5 py-0.5 rounded-full text-[0.65rem] bg-[var(--glass-2)]">{birthrightRoles.length}</span>
        </button>
        <button
          onClick={() => setActiveTab("it")}
          className={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
            activeTab === "it"
              ? "bg-blue-500/10 text-blue-400 border border-blue-500/30"
              : "text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
          }`}
        >
          <span className="mr-1.5">⚙</span>
          IT Roles
          <span className="ml-2 px-1.5 py-0.5 rounded-full text-[0.65rem] bg-[var(--glass-2)]">{itRoles.length}</span>
        </button>
      </div>

      {/* Description */}
      <div className="glass-card p-4">
        {activeTab === "birthright" ? (
          <div className="flex gap-3">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-sm flex-shrink-0">✦</div>
            <div>
              <h3 className="text-sm font-semibold text-[var(--text-primary)]">Birthright Roles</h3>
              <p className="text-xs text-[var(--text-secondary)] mt-1">
                Automatically assigned based on employee attributes (department, title, location).
                When a new identity is created via connector sync, these roles are granted instantly —
                no approval needed. Revoked automatically when attributes change.
              </p>
            </div>
          </div>
        ) : (
          <div className="flex gap-3">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center bg-blue-500/10 border border-blue-500/20 text-blue-400 text-sm flex-shrink-0">⚙</div>
            <div>
              <h3 className="text-sm font-semibold text-[var(--text-primary)]">IT Roles</h3>
              <p className="text-xs text-[var(--text-secondary)] mt-1">
                Manually assigned by IT admins or requested via access workflows.
                Requires approval. Used for elevated access, project-based permissions,
                and temporary access grants with auto-expiry.
              </p>
            </div>
          </div>
        )}
      </div>

      {/* Role Cards */}
      {loading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {[1, 2, 3].map(i => (
            <div key={i} className="glass-card p-4 animate-pulse">
              <div className="h-5 bg-[var(--glass-3)] rounded w-1/3 mb-3" />
              <div className="h-3 bg-[var(--glass-2)] rounded w-2/3" />
            </div>
          ))}
        </div>
      ) : displayRoles.length === 0 ? (
        <div className="glass-card p-12 text-center">
          <p className="text-[var(--text-muted)] text-sm">No {activeTab === "birthright" ? "birthright" : "IT"} roles found.</p>
          <Link href="/groups" className="text-xs text-[var(--accent)] mt-2 inline-block no-underline">
            Create roles in Role Management →
          </Link>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {displayRoles.map(r => {
            const style = ROLE_STYLES[r.role_type] || ROLE_STYLES.custom
            return (
              <div key={r.id} className="glass-card p-4 hover:border-[var(--obsidian-border-accent)] transition-all">
                <div className="flex items-start justify-between">
                  <div className="flex items-start gap-3">
                    <div
                      className="w-10 h-10 rounded-xl flex items-center justify-center text-sm flex-shrink-0"
                      style={{ background: `${style.color}12`, border: `1px solid ${style.color}25`, color: style.color }}
                    >
                      {style.icon}
                    </div>
                    <div>
                      <h3 className="text-sm font-semibold text-[var(--text-primary)]">{r.name}</h3>
                      {r.description && (
                        <p className="text-xs text-[var(--text-secondary)] mt-1 line-clamp-2">{r.description}</p>
                      )}
                      <div className="flex gap-2 mt-2 flex-wrap">
                        {r.is_auto_assigned && (
                          <span className="px-2 py-0.5 rounded-full text-[0.65rem] font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                            Auto-assigned
                          </span>
                        )}
                        {r.approval_required && (
                          <span className="px-2 py-0.5 rounded-full text-[0.65rem] font-medium bg-amber-500/10 text-amber-400 border border-amber-500/20">
                            Approval required
                          </span>
                        )}
                        {!r.is_active && (
                          <span className="px-2 py-0.5 rounded-full text-[0.65rem] font-medium bg-[var(--glass-2)] text-[var(--text-muted)] border border-[var(--obsidian-border)]">
                            Inactive
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                  <div className="flex gap-3 text-center">
                    <div>
                      <div className="text-base font-bold text-[var(--text-primary)]">{r.member_count}</div>
                      <div className="text-[0.6rem] text-[var(--text-muted)]">Members</div>
                    </div>
                    <div>
                      <div className="text-base font-bold text-[var(--text-primary)]">{r.entitlement_count}</div>
                      <div className="text-[0.6rem] text-[var(--text-muted)]">Entitlements</div>
                    </div>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {/* Summary Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        <div className="glass-card p-3 text-center">
          <div className="text-lg font-bold text-[var(--text-primary)]">{birthrightRoles.length}</div>
          <div className="text-[0.65rem] text-[var(--text-muted)]">Birthright Roles</div>
        </div>
        <div className="glass-card p-3 text-center">
          <div className="text-lg font-bold text-[var(--text-primary)]">{itRoles.length}</div>
          <div className="text-[0.65rem] text-[var(--text-muted)]">IT Roles</div>
        </div>
        <div className="glass-card p-3 text-center">
          <div className="text-lg font-bold text-emerald-400">{roles.filter(r => r.is_auto_assigned).length}</div>
          <div className="text-[0.65rem] text-[var(--text-muted)]">Auto-assigned</div>
        </div>
        <div className="glass-card p-3 text-center">
          <div className="text-lg font-bold text-amber-400">{roles.filter(r => r.approval_required).length}</div>
          <div className="text-[0.65rem] text-[var(--text-muted)]">Require Approval</div>
        </div>
      </div>
    </div>
  )
}
