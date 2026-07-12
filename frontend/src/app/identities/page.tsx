"use client"

import { useState, useEffect, useCallback } from "react"
import { fetchIdentities, fetchAgents, createIdentity, deleteIdentity, Identity, Agent } from "@/lib/api"
import { PageHeader } from "@/components/ui/PageHeader"
import { Badge } from "@/components/ui/Badge"
import { Button } from "@/components/ui/Button"
import { Card, CardHeader, CardBody } from "@/components/ui/Card"
import { Input, Select } from "@/components/ui/Input"
import { Modal } from "@/components/ui/Modal"
import { Tabs } from "@/components/ui/Tabs"
import { DataTable } from "@/components/ui/DataTable"
import { EmptyState } from "@/components/ui/EmptyState"

export default function IdentitiesPage() {
  const [tab, setTab] = useState("people")
  const [people, setPeople] = useState<Identity[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [synced, setSynced] = useState<any[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState("")
  const [showAddModal, setShowAddModal] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [p, a] = await Promise.all([
        fetchIdentities().catch(() => null),
        fetchAgents().catch(() => null),
      ])
      setPeople(p?.identities || [])
      setAgents(a?.agents || [])

      // Load synced identities
      const connsRes = await fetch("/api/v1/connectors").then(r => r.json()).catch(() => null)
      const all: any[] = []
      if (connsRes?.connectors) {
        for (const c of connsRes.connectors) {
          try {
            const d = await fetch(`/api/v1/connectors/${c.id}/identities`).then(r => r.json())
            if (d?.identities) {
              for (const id of d.identities) { all.push({ ...id, _connName: c.name }) }
            }
          } catch (_) {}
        }
      }
      setSynced(all)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  async function handleCreate(data: any) {
    try {
      await createIdentity(data)
      setShowAddModal(false)
      load()
    } catch (e: any) { alert("Failed: " + e.message) }
  }

  async function handleDelete(id: string) {
    if (!confirm("Delete this identity? The record will be deactivated.")) return
    try { await deleteIdentity(id); load() } catch (e: any) { alert("Failed: " + e.message) }
  }

  const filtered = people.filter(p =>
    (!search || p.name?.toLowerCase().includes(search.toLowerCase()) || p.email?.toLowerCase().includes(search.toLowerCase()))
  )

  const tabs = [
    { id: "people", label: "People", count: people.length },
    { id: "machines", label: "Machines", count: agents.length },
    { id: "synced", label: "Directory", count: synced.length },
  ]

  return (
    <div className="space-y-6">
      <PageHeader
        title="Identities"
        description={`${people.length + agents.length + synced.length} total identities across all sources`}
        actions={
          <Button variant="primary" size="sm" onClick={() => setShowAddModal(true)}>
            <PlusIcon /> Add Identity
          </Button>
        }
      />

      <Card>
        <CardBody className="flex items-center gap-4">
          <Input
            placeholder="Search by name or email..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="max-w-xs"
          />
          <Tabs tabs={tabs} active={tab} onChange={setTab} />
        </CardBody>
      </Card>

      <Card>
        {loading ? (
          <div className="p-12 text-center animate-pulse space-y-3">
            {[1,2,3,4].map(i => <div key={i} className="h-10 bg-white/[0.03] rounded" />)}
          </div>
        ) : tab === "people" ? (
          filtered.length === 0 ? (
            <EmptyState
              title="No identities yet"
              description="Add your first identity manually or import from HR/CSV."
              action={{ label: "Add Identity", onClick: () => setShowAddModal(true) }}
              icon={<UsersIcon />}
            />
          ) : (
            <DataTable
              columns={[
                { key: "name", header: "Identity", render: (row) => (
                  <div className="flex items-center gap-2.5">
                    <div className="w-7 h-7 rounded-full bg-accent/10 border border-accent/30 flex items-center justify-center text-xs font-bold text-accent">
                      {(row.name || row.email || "?").charAt(0).toUpperCase()}
                    </div>
                    <span className="font-medium">{row.name || "Unnamed"}</span>
                  </div>
                )},
                { key: "email", header: "Email", render: (row) => <span className="text-secondary">{row.email || "-"}</span> },
                { key: "department", header: "Dept", render: (row) => <span className="text-secondary">{row.department || "-"}</span> },
                { key: "status", header: "Status", render: (row) => {
                  const v = (row.status || "active") as string
                  const vars: Record<string, "success"|"warning"|"danger"|"neutral"> = {
                    active: "success", inactive: "neutral", suspended: "warning", terminated: "danger"
                  }
                  return <Badge variant={vars[v] || "neutral"}>{v}</Badge>
                }},
                { key: "type", header: "Type", render: (row) => <span className="text-secondary capitalize">{(row.type || "human").replace("_"," ")}</span> },
                { key: "actions", header: "", align: "right", render: (row) => (
                  <Button variant="ghost" size="sm" onClick={(e) => { e?.stopPropagation?.(); handleDelete(row.id) }}>
                    Delete
                  </Button>
                )},
              ]}
              data={filtered}
              keyField="id"
              emptyMessage="No matching identities"
            />
          )
        ) : tab === "machines" ? (
          agents.length === 0 ? (
            <EmptyState title="No machine identities" description="Register an AI agent or service account." icon={<BotIcon />} />
          ) : (
            <DataTable
              columns={[
                { key: "name", header: "Agent", render: (row) => <span className="font-medium">{row.name}</span> },
                { key: "type", header: "Type", render: (row) => <span className="text-secondary capitalize">{row.type?.replace("_"," ")}</span> },
                { key: "status", header: "Status", render: (row) => <Badge variant={(row.status === "active") ? "success" : "neutral"}>{row.status}</Badge> },
                { key: "governed", header: "Governed", render: (row) => <Badge variant={row.is_governed ? "success" : "danger"}>{row.is_governed ? "Yes" : "No"}</Badge> },
              ]}
              data={agents}
              keyField="id"
            />
          )
        ) : (
          synced.length === 0 ? (
            <EmptyState
              title="No directory-synced identities"
              description="Connect a directory (Entra ID, LDAP, Okta) and sync to import identities."
            />
          ) : (
            <DataTable
              columns={[
                { key: "name", header: "User", render: (row) => <span className="font-medium">{row.display_name || row.email || "-"}</span> },
                { key: "email", header: "Email", render: (row) => <span className="text-secondary">{row.email || "-"}</span> },
                { key: "source", header: "Source", render: (row) => <Badge variant="info">{row._connName || "Directory"}</Badge> },
                { key: "enabled", header: "Status", render: (row) => <Badge variant={row.enabled ? "success" : "neutral"}>{row.enabled ? "Active" : "Disabled"}</Badge> },
              ]}
              data={synced}
              keyField="id"
            />
          )
        )}
      </Card>

      <AddIdentityModal
        open={showAddModal}
        onClose={() => setShowAddModal(false)}
        onSubmit={handleCreate}
      />
    </div>
  )
}

/* ─── Add Identity Modal ──────────────────────────────────── */
function AddIdentityModal({ open, onClose, onSubmit }: {
  open: boolean; onClose: () => void; onSubmit: (data: any) => void
}) {
  const [form, setForm] = useState({
    email: "", display_name: "", first_name: "", last_name: "",
    type: "human", department: "", title: "", employee_id: "",
    source: "manual", phone: "", tenant_id: ""
  })
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit() {
    if (!form.email || !form.display_name) return
    setSubmitting(true)
    try { await onSubmit(form) } finally { setSubmitting(false) }
  }

  function set(key: string, value: string) { setForm(f => ({ ...f, [key]: value })) }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Add Identity"
      description="Create a new identity in the Identity Fabric"
      footer={
        <div className="flex justify-end gap-2">
          <Button variant="secondary" onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={handleSubmit} loading={submitting}>Create Identity</Button>
        </div>
      }
    >
      <div className="grid grid-cols-2 gap-3">
        <div className="col-span-2">
          <Input label="Email *" placeholder="user@company.com" value={form.email} onChange={e => set("email", e.target.value)} />
        </div>
        <Input label="Display Name *" placeholder="Jane Doe" value={form.display_name} onChange={e => set("display_name", e.target.value)} />
        <Input label="First Name" placeholder="Jane" value={form.first_name} onChange={e => set("first_name", e.target.value)} />
        <Input label="Last Name" placeholder="Doe" value={form.last_name} onChange={e => set("last_name", e.target.value)} />
        <Input label="Phone" placeholder="+1 555-0000" value={form.phone} onChange={e => set("phone", e.target.value)} />
        <Select label="Type" value={form.type} onChange={e => set("type", e.target.value)} options={[
          { value: "human", label: "Human" },
          { value: "service_account", label: "Service Account" },
          { value: "ai_agent", label: "AI Agent" },
          { value: "iot_device", label: "IoT Device" },
        ]} />
        <Input label="Department" placeholder="Engineering" value={form.department} onChange={e => set("department", e.target.value)} />
        <Input label="Title" placeholder="Software Engineer" value={form.title} onChange={e => set("title", e.target.value)} />
        <Input label="Employee ID" placeholder="EMP-001" value={form.employee_id} onChange={e => set("employee_id", e.target.value)} />
        <Select label="Source" value={form.source} onChange={e => set("source", e.target.value)} options={[
          { value: "manual", label: "Manual" }, { value: "hris", label: "HRIS Import" },
          { value: "scim", label: "SCIM" }, { value: "ldap", label: "LDAP/AD" },
        ]} />
      </div>
    </Modal>
  )
}

/* ─── Icons ────────────────────────────────────────────────── */
function PlusIcon() {
  return <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 5v14M5 12h14"/></svg>
}
function UsersIcon() {
  return <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1" className="text-muted"><path d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197m13.5-9a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"/></svg>
}
function BotIcon() {
  return <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1" className="text-muted"><path d="M9.75 3.104v5.714a2.25 2.25 0 01-.659 1.591L5 14.5M9.75 3.104c-.251.023-.501.05-.75.082m.75-.082a24.301 24.301 0 014.5 0m0 0v5.714c0 .597.237 1.17.659 1.591L19.8 15.3"/></svg>
}
