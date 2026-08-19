"use client"
import { useEffect, useState } from "react"
import { createElement as h } from "react"
import { listPendingApprovals, decideApproval, type Approval } from "@/lib/api"

export default function InboxPage() {
  const [rows, setRows] = useState<Approval[]>([])
  const [loading, setLoading] = useState(true)
  const [comment, setComment] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState<string>("")
  const [msg, setMsg] = useState<string>("")

  const load = async () => {
    try {
      const data = await listPendingApprovals()
      setRows(data.approvals || [])
    } catch (e) {
      console.error(e)
    }
    setLoading(false)
  }

  useEffect(() => {
    load()
    const t = setInterval(load, 5000)
    return () => clearInterval(t)
  }, [])

  const decide = async (a: Approval, approved: boolean) => {
    setBusy(a.id)
    setMsg("")
    try {
      await decideApproval(a.id, {
        approver_id: "00000000-0000-0000-0000-000000000c0c",
        approved,
        comment: comment[a.id] || "",
      })
      setMsg(approved ? `Approved ${a.id.slice(0, 8)}` : `Denied ${a.id.slice(0, 8)}`)
      load()
    } catch (e) {
      setMsg("Failed: " + (e as Error).message)
    }
    setBusy("")
  }

  const fmt = (s: string | undefined): string => {
    if (!s) return "-"
    const ms = Date.now() - new Date(s).getTime()
    const sec = Math.floor(ms / 1000)
    if (sec < 60) return sec + "s"
    const m = Math.floor(sec / 60)
    if (m < 60) return m + "m"
    return Math.floor(m / 60) + "h"
  }

  const tdStyle = { padding: "10px 12px", fontFamily: "monospace" as const }
  const tdStyleRight = { padding: "10px 12px", color: "#5C5C62" }
  const tdStyleType = { padding: "10px 12px", color: "#FBBF24" }
  const tdStyleRole = { padding: "10px 12px", color: "#7DD3FC" }
  const thStyle = { textAlign: "left" as const, padding: "10px 12px", color: "#9C9CA0", fontWeight: 600, borderBottom: "1px solid rgba(255,255,255,0.1)" }
  const trStyle = { borderBottom: "1px solid rgba(255,255,255,0.04)" }
  const btnApprove = { marginRight: 8, padding: "6px 14px", borderRadius: 8, border: "none", cursor: "pointer" as const, background: "#34D399", color: "#0A0A0F", fontWeight: 600 }
  const btnDeny = { padding: "6px 14px", borderRadius: 8, border: "none", cursor: "pointer" as const, background: "#F87171", color: "#0A0A0F", fontWeight: 600 }
  const input = { padding: "6px 10px", borderRadius: 8, border: "1px solid rgba(255,255,255,0.15)", background: "#16161B", color: "#F0EFEC", width: 220 }

  if (loading) {
    return h("div", { style: { maxWidth: 1200, margin: "0 auto" } },
      h("h1", { style: { fontSize: 28, fontWeight: 700, color: "#F0EFEC", marginBottom: 4 } }, "Approval Inbox"),
      h("p", { style: { color: "#5C5C62" } }, "Loading")
    )
  }

  return h("div", { style: { maxWidth: 1200, margin: "0 auto" } },
    h("h1", { style: { fontSize: 28, fontWeight: 700, color: "#F0EFEC", marginBottom: 4 } }, "Approval Inbox"),
    h("p", { style: { fontSize: 13, color: "#5C5C62", marginBottom: 16 } },
      "Pending approval decisions from the routing engine (workflow_approvals)."
    ),
    msg ? h("p", { style: { fontSize: 13, color: "#34D399", marginBottom: 16 } }, msg) : null,
    rows.length === 0
      ? h("p", { style: { color: "#5C5C62", padding: 40, textAlign: "center", border: "1px dashed rgba(255,255,255,0.08)", borderRadius: 12 } },
          "No pending approvals. High-risk grants and critical firecalls will appear here."
        )
      : h("table", { style: { width: "100%", borderCollapse: "collapse", fontSize: 13 } },
          h("thead", null,
            h("tr", null,
              h("th", { style: thStyle }, "Type"),
              h("th", { style: thStyle }, "Level"),
              h("th", { style: thStyle }, "Approver"),
              h("th", { style: thStyle }, "Request"),
              h("th", { style: thStyle }, "Due"),
              h("th", { style: thStyle }, "Decide")
            )
          ),
          h("tbody", null,
            ...rows.map((a) =>
              h("tr", { key: a.id, style: trStyle },
                h("td", { style: tdStyleType }, a.request_id.slice(0, 8) || "-"),
                h("td", { style: tdStyle }, "L" + a.level),
                h("td", { style: tdStyleRole }, a.approver_role + (a.approver_email ? " (" + a.approver_email + ")" : "")),
                h("td", { style: tdStyleRight }, a.request_id.slice(0, 8)),
                h("td", { style: tdStyleRight }, a.due_at ? fmt(a.due_at) + " left" : "-"),
                h("td", { style: tdStyle },
                  h("input", {
                    placeholder: "Comment (optional)",
                    value: comment[a.id] || "",
                    style: input,
                    onChange: (e: any) => setComment((c) => ({ ...c, [a.id]: e.target.value })),
                  }),
                  h("button", {
                    style: btnApprove,
                    disabled: busy === a.id,
                    onClick: () => decide(a, true),
                  }, "Approve"),
                  h("button", {
                    style: btnDeny,
                    disabled: busy === a.id,
                    onClick: () => decide(a, false),
                  }, "Deny")
                )
              )
            )
          )
        )
  )
}