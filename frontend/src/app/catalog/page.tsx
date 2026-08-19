"use client"
import { useEffect, useState } from "react"
import { createElement as h } from "react"
import { useRouter } from "next/navigation"
import { listRoleCatalog, type CatalogRole, requestRoleAccess } from "@/lib/api"

export default function CatalogPage() {
  const [roles, setRoles] = useState<CatalogRole[]>([])
  const [loading, setLoading] = useState(true)
  const [msg, setMsg] = useState<string>("")
  const [reqIdentity, setReqIdentity] = useState<string>("")
  const [reqDuration, setReqDuration] = useState<string>("")
  const router = useRouter()

  const load = async () => {
    try {
      const data = await listRoleCatalog()
      setRoles(data.roles || [])
    } catch (e) {
      console.error(e)
    }
    setLoading(false)
  }

  useEffect(() => { load() }, [])

  const request = async (role: CatalogRole) => {
    if (!reqIdentity) { setMsg("Enter identity_id above"); return; }
    setMsg("Submitting...")
    try {
      await requestRoleAccess({
        identity_id: reqIdentity,
        role_id: role.id,
        requested_by: reqIdentity,
        reason: `Self-service request for ${role.name}`,
        duration_hours: reqDuration ? parseInt(reqDuration) : undefined,
      })
      setMsg(`Requested ${role.name} — check Inbox for approval`)
      setReqIdentity("")
      setReqDuration("")
    } catch (e) {
      setMsg("Failed: " + (e as Error).message)
    }
  }

  const td = { padding: "10px 12px", fontFamily: "monospace" as const }
  const tdR = { padding: "10px 12px", color: "#5C5C62" }
  const th = { textAlign: "left" as const, padding: "10px 12px", color: "#9C9CA0", fontWeight: 600, borderBottom: "1px solid rgba(255,255,255,0.1)" }
  const tr = { borderBottom: "1px solid rgba(255,255,255,0.04)" }
  const inp = { padding: "6px 10px", borderRadius: 8, border: "1px solid rgba(255,255,255,0.15)", background: "#16161B", color: "#F0EFEC", width: 240, marginRight: 12 }
  const btn = { padding: "6px 14px", borderRadius: 8, border: "none", cursor: "pointer" as const, background: "#34D399", color: "#0A0A0F", fontWeight: 600 }

  if (loading) {
    return h("div", { style: { maxWidth: 1000, margin: "0 auto" } },
      h("h1", { style: { fontSize: 28, fontWeight: 700, color: "#F0EFEC", marginBottom: 4 } }, "Requestable Roles"),
      h("p", { style: { color: "#5C5C62" } }, "Loading")
    )
  }

  return h("div", { style: { maxWidth: 1000, margin: "0 auto" } },
    h("h1", { style: { fontSize: 28, fontWeight: 700, color: "#F0EFEC", marginBottom: 4 } }, "Requestable Roles"),
    h("p", { style: { fontSize: 13, color: "#5C5C62", marginBottom: 16 } },
      "Roles that require approval. Enter your identity_id and optionally duration, then click Request."
    ),
    h("div", { style: { display: "flex", gap: 12, marginBottom: 24 } },
      h("input", { placeholder: "Your identity_id (UUID)", value: reqIdentity, style: inp, onChange: (e: React.ChangeEvent<HTMLInputElement>) => setReqIdentity(e.target.value) }),
      h("input", { placeholder: "Duration hours (optional)", value: reqDuration, style: { ...inp, width: 180 }, onChange: (e: React.ChangeEvent<HTMLInputElement>) => setReqDuration(e.target.value) }),
    ),
    msg ? h("p", { style: { fontSize: 13, color: "#34D399", marginBottom: 16 } }, msg) : null,
    roles.length === 0
      ? h("p", { style: { color: "#5C5C62", padding: 40, textAlign: "center", border: "1px dashed rgba(255,255,255,0.08)", borderRadius: 12 } }, "No requestable roles configured.")
      : h("table", { style: { width: "100%", borderCollapse: "collapse", fontSize: 13 } },
          h("thead", null,
            h("tr", null,
              h("th", { style: th }, "Role"),
              h("th", { style: th }, "Type"),
              h("th", { style: th }, "Max Duration"),
              h("th", { style: th }, "Action")
            )
          ),
          h("tbody", null,
            ...roles.map((r) =>
              h("tr", { key: r.id, style: tr },
                h("td", { style: td }, r.name),
                h("td", { style: td }, r.role_type),
                h("td", { style: tdR }, r.max_duration_hours ? r.max_duration_hours + "h" : "unlimited"),
                h("td", { style: td },
                  h("button", { style: btn, onClick: () => request(r) }, "Request")
                )
              )
            )
          )
        )
  )
}