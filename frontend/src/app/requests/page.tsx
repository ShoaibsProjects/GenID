"use client"
import { useEffect, useState } from "react"
import { createElement as h } from "react"
import { listRequests, type WorkflowRequest } from "@/lib/api"

export default function RequestsPage() {
  const [rows, setRows] = useState<WorkflowRequest[]>([])
  const [loading, setLoading] = useState(true)

  const load = async () => {
    try {
      const data = await listRequests({ limit: 100 })
      setRows(data.requests || [])
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

  const fmt = (s: string | undefined): string => {
    if (!s) return "-"
    const ms = Date.now() - new Date(s).getTime()
    const sec = Math.floor(ms / 1000)
    if (sec < 60) return sec + "s"
    const m = Math.floor(sec / 60)
    if (m < 60) return m + "m"
    return Math.floor(m / 60) + "h"
  }

  const tdStyle = { padding: "10px 12px", fontFamily: "monospace" }
  const tdStyleRight = { padding: "10px 12px", color: "#5C5C62" }
  const tdStyleStatus = { padding: "10px 12px", color: "#34D399" }
  const tdStyleType = { padding: "10px 12px", color: "#FBBF24" }
  const thStyle = { textAlign: "left" as const, padding: "10px 12px", color: "#9C9CA0", fontWeight: 600, borderBottom: "1px solid rgba(255,255,255,0.1)" }
  const trStyle = { borderBottom: "1px solid rgba(255,255,255,0.04)" }

  if (loading) {
    return h("div", { style: { maxWidth: 1400, margin: "0 auto" } },
      h("h1", { style: { fontSize: 28, fontWeight: 700, color: "#F0EFEC", marginBottom: 4 } }, "Identity Automation"),
      h("p", { style: { color: "#5C5C62" } }, "Loading")
    )
  }

  if (rows.length === 0) {
    return h("div", { style: { maxWidth: 1400, margin: "0 auto" } },
      h("h1", { style: { fontSize: 28, fontWeight: 700, color: "#F0EFEC", marginBottom: 4 } }, "Identity Automation"),
      h("p", { style: { fontSize: 13, color: "#5C5C62", marginBottom: 24 } }, "Workflow request lifecycle."),
      h("p", { style: { color: "#5C5C62", padding: 40, textAlign: "center", border: "1px dashed rgba(255,255,255,0.08)", borderRadius: 12 } },
        "No workflow requests yet. Click Firecall on the Dashboard or send an event from the Event Simulator."
      )
    )
  }

  return h("div", { style: { maxWidth: 1400, margin: "0 auto" } },
    h("h1", { style: { fontSize: 28, fontWeight: 700, color: "#F0EFEC", marginBottom: 4 } }, "Identity Automation"),
    h("p", { style: { fontSize: 13, color: "#5C5C62", marginBottom: 24 } },
      "Workflow request lifecycle. Every Temporal workflow writes through workflow_requests and workflow_audit."
    ),
    h("table", { style: { width: "100%", borderCollapse: "collapse", fontSize: 13 } },
      h("thead", null,
        h("tr", null,
          h("th", { style: thStyle }, "Type"),
          h("th", { style: thStyle }, "Status"),
          h("th", { style: thStyle }, "ID"),
          h("th", { style: thStyle }, "Target"),
          h("th", { style: thStyle }, "Created"),
          h("th", { style: thStyle }, "Expires")
        )
      ),
      h("tbody", null,
        ...rows.map((r) =>
          h("tr", { key: r.id, style: trStyle },
            h("td", { style: tdStyleType }, r.type),
            h("td", { style: tdStyleStatus }, r.status),
            h("td", { style: tdStyle }, r.id.slice(0, 8)),
            h("td", { style: tdStyle }, (r.target_id || "-").slice(0, 8)),
            h("td", { style: tdStyleRight }, fmt(r.created_at) + " ago"),
            h("td", { style: tdStyleRight }, r.expires_at ? fmt(r.expires_at) : "-")
          )
        )
      )
    )
  )
}
