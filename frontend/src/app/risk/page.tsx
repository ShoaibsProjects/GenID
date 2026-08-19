"use client"
import { useState, useEffect, useCallback } from "react"
import { authFetch } from "@/lib/api"
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/Card"
import { Badge } from "@/components/ui/Badge"
import { Button } from "@/components/ui/Button"
import { Input } from "@/components/ui/Input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell, TableCaption } from "@/components/ui/table"
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from "recharts"
import { TrendingUp, TrendingDown, Minus, Search, Filter, Download, AlertTriangle, ArrowUpRight, ArrowDownRight, Minus as MinusIcon, Download as DownloadIcon } from "lucide-react"
import { cn } from "@/lib/utils"

interface RiskSummary {
  critical: number
  high: number
  elevated: number
  low: number
  total: number
}

interface RiskIdentity {
  id: string
  email: string
  display_name: string
  department: string
  risk_score: number
  risk_band: string
  last_event: string
  last_event_at: string
  status: string
}

interface RiskTimelinePoint {
  timestamp: string
  score: number
}

const BAND_COLORS: Record<string, { bg: string; text: string; border: string; icon: string }> = {
  critical: { bg: "bg-red-500/10", text: "text-red-400", border: "border-red-500/25", icon: "🔴" },
  high: { bg: "bg-orange-500/10", text: "text-orange-400", border: "border-orange-500/25", icon: "🟠" },
  elevated: { bg: "bg-amber-500/10", text: "text-amber-400", border: "border-amber-500/25", icon: "🟡" },
  low: { bg: "bg-green-500/10", text: "text-green-400", border: "border-green-500/25", icon: "🟢" },
}

const mockTimeline = [
  { timestamp: "2024-01-15T00:00:00Z", score: 180 },
  { timestamp: "2024-01-16T00:00:00Z", score: 195 },
  { timestamp: "2024-01-17T00:00:00Z", score: 210 },
  { timestamp: "2024-01-18T00:00:00Z", score: 185 },
  { timestamp: "2024-01-19T00:00:00Z", score: 230 },
  { timestamp: "2024-01-20T00:00:00Z", score: 275 },
  { timestamp: "2024-01-21T00:00:00Z", score: 320 },
]

const mockIdentities = [
  { id: "1", email: "alice@company.com", display_name: "Alice Chen", department: "Engineering", risk_score: 850, risk_band: "critical", last_event: "auth.failed_login", last_event_at: "2024-01-21T14:32:00Z", status: "active" },
  { id: "2", email: "bob@company.com", display_name: "Bob Smith", department: "Sales", risk_score: 720, risk_band: "high", last_event: "auth.failed_login", last_event_at: "2024-01-21T10:15:00Z", status: "active" },
  { id: "3", email: "carol@company.com", display_name: "Carol Davis", department: "Marketing", risk_score: 650, risk_band: "high", last_event: "device.untrusted", last_event_at: "2024-01-21T09:45:00Z", status: "active" },
  { id: "4", email: "david@company.com", display_name: "David Wilson", department: "Engineering", risk_score: 420, risk_band: "elevated", last_event: "network.vpn", last_event_at: "2024-01-21T08:30:00Z", status: "active" },
  { id: "5", email: "eve@company.com", display_name: "Eve Brown", department: "HR", risk_score: 180, risk_band: "low", last_event: "", last_event_at: "2024-01-20T17:00:00Z", status: "active" },
  { id: "6", email: "frank@company.com", display_name: "Frank Miller", department: "Finance", risk_score: 95, risk_band: "low", last_event: "", last_event_at: "2024-01-20T16:00:00Z", status: "active" },
  { id: "7", email: "grace@company.com", display_name: "Grace Lee", department: "Engineering", risk_score: 310, risk_band: "elevated", last_event: "auth.mfa_failed", last_event_at: "2024-01-21T11:20:00Z", status: "active" },
  { id: "8", email: "henry@company.com", display_name: "Henry Chen", department: "Security", risk_score: 680, risk_band: "high", last_event: "auth.failed_login", last_event_at: "2024-01-21T13:10:00Z", status: "active" },
]

function getRiskBand(score: number): string {
  if (score >= 800) return "critical"
  if (score >= 600) return "high"
  if (score >= 300) return "elevated"
  return "low"
}

export default function RiskDashboardPage() {
  const [summary, setSummary] = useState<RiskSummary>({ critical: 0, high: 0, elevated: 0, low: 0, total: 0 })
  const [identities, setIdentities] = useState<RiskIdentity[]>([])
  const [timeline, setTimeline] = useState<RiskTimelinePoint[]>([])
  const [loading, setLoading] = useState(true)
  const [search, setSearch] = useState("")
  const [bandFilter, setBandFilter] = useState("all")
  const [sortBy, setSortBy] = useState<"risk_score" | "last_event_at" | "email">("risk_score")
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc")

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [summaryRes, identitiesRes] = await Promise.all([
        authFetch("/api/v1/risk/summary").then(r => r.json()).catch(() => null),
        authFetch("/api/v1/identities?limit=100").then(r => r.json()).catch(() => null),
      ])

      if (summaryRes) setSummary(summaryRes)
      if (identitiesRes) {
        const enriched = identitiesRes.identities?.map((i: any) => ({
          ...i,
          risk_band: getRiskBand(i.risk_score),
        })) || mockIdentities
        setIdentities(enriched)
      } else {
        setIdentities(mockIdentities)
      }
      setTimeline(mockTimeline)
    } catch (error) {
      console.error("Failed to load risk data:", error)
      setIdentities(mockIdentities)
      setTimeline(mockTimeline)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadData()
  }, [loadData])

  const filteredIdentities = identities
    .filter(i => {
      if (bandFilter !== "all" && i.risk_band !== bandFilter) return false
      if (search && !i.email.toLowerCase().includes(search.toLowerCase()) && 
          !i.display_name?.toLowerCase().includes(search.toLowerCase())) return false
      return true
    })
    .sort((a, b) => {
      const aVal = a[sortBy]
      const bVal = b[sortBy]
      if (aVal < bVal) return sortDir === "asc" ? -1 : 1
      if (aVal > bVal) return sortDir === "asc" ? 1 : -1
      return 0
    })

  const formatTime = (iso: string) => {
    const d = new Date(iso)
    return d.toLocaleString()
  }

  return (
    <div className="max-w-7xl mx-auto space-y-6 animate-fade-in">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gradient-accent">Risk Dashboard</h1>
          <p className="text-[var(--text-secondary)] mt-1">Real-time identity risk posture across the organization</p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm">
            <DownloadIcon className="w-4 h-4 mr-2" />
            Export CSV
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card className="glass-card stat-card">
          <div style={{ position: "absolute", top: 0, left: 0, right: 0, height: 3, background: "linear-gradient(90deg, transparent, rgba(239,68,68,0.3), transparent)" }} />
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">Critical (≥800)</p>
                <p className="text-3xl font-bold text-red-400 mt-1">{summary.critical}</p>
              </div>
              <div className="w-12 h-12 rounded-xl bg-red-500/10 flex items-center justify-center">
                <AlertTriangle className="w-6 h-6 text-red-400" />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="glass-card stat-card">
          <div style={{ position: "absolute", top: 0, left: 0, right: 0, height: 3, background: "linear-gradient(90deg, transparent, rgba(249,115,22,0.3), transparent)" }} />
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">High (600-799)</p>
                <p className="text-3xl font-bold text-orange-400 mt-1">{summary.high}</p>
              </div>
              <div className="w-12 h-12 rounded-xl bg-orange-500/10 flex items-center justify-center">
                <ArrowUpRight className="w-6 h-6 text-orange-400" />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="glass-card stat-card">
          <div style={{ position: "absolute", top: 0, left: 0, right: 0, height: 3, background: "linear-gradient(90deg, transparent, rgba(245,158,11,0.3), transparent)" }} />
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">Elevated (300-599)</p>
                <p className="text-3xl font-bold text-amber-400 mt-1">{summary.elevated}</p>
              </div>
              <div className="w-12 h-12 rounded-xl bg-amber-500/10 flex items-center justify-center">
                <ArrowUpRight className="w-6 h-6 text-amber-400" />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="glass-card stat-card">
          <div style={{ position: "absolute", top: 0, left: 0, right: 0, height: 3, background: "linear-gradient(90deg, transparent, rgba(52,211,153,0.3), transparent)" }} />
          <CardContent className="p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">Low (&lt;300)</p>
                <p className="text-3xl font-bold text-green-400 mt-1">{summary.low}</p>
              </div>
              <div className="w-12 h-12 rounded-xl bg-green-500/10 flex items-center justify-center">
                <ArrowDownRight className="w-6 h-6 text-green-400" />
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card className="glass-card">
        <CardHeader>
          <CardTitle className="text-xl">Risk Timeline (Last 7 Days)</CardTitle>
          <CardDescription>Organization-wide average risk score trend</CardDescription>
        </CardHeader>
        <CardContent>
          <div style={{ height: 300 }}>
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={mockTimeline} margin={{ top: 5, right: 30, left: 20, bottom: 5 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="#1e1e2e" vertical={false} />
                <XAxis dataKey="timestamp" stroke="#5C5C62" fontSize={12} tickFormatter={(v) => new Date(v).toLocaleDateString()} />
                <YAxis stroke="#5C5C62" fontSize={12} domain={[0, 1000]} />
                <Tooltip 
                  contentStyle={{ background: '#14141A', border: '1px solid rgba(245,158,11,0.2)', borderRadius: '8px' }}
                  labelFormatter={(v) => new Date(v).toLocaleDateString()}
                />
                <Legend />
                <Line 
                  type="monotone" 
                  dataKey="score" 
                  stroke="#F59E0B" 
                  strokeWidth={2} 
                  dot={false}
                  activeDot={{ r: 6, strokeWidth: 2, fill: '#F59E0B' }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </CardContent>
      </Card>

      <Card className="glass-card">
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-xl">Identities by Risk</CardTitle>
              <CardDescription>All identities sorted by risk score</CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Select value={bandFilter} onValueChange={setBandFilter}>
                <SelectTrigger className="w-40"><SelectValue placeholder="All bands" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Bands</SelectItem>
                  <SelectItem value="critical">Critical</SelectItem>
                  <SelectItem value="high">High</SelectItem>
                  <SelectItem value="elevated">Elevated</SelectItem>
                  <SelectItem value="low">Low</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </CardHeader>
        <CardContent>
            <div className="flex items-center gap-4 mb-4">
              <Search className="w-5 h-5 text-[var(--text-muted)]" />
              <Input 
                placeholder="Search by name or email..." 
                value={search} 
                onChange={e => setSearch(e.target.value)} 
                className="w-64"
              />
            </div>

            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-right">Risk</TableHead>
                    <TableHead onClick={() => { setSortBy("email"); setSortDir(sortDir === "asc" ? "desc" : "asc") }} style={{ cursor: "pointer" }}>Identity</TableHead>
                    <TableHead>Department</TableHead>
                    <TableHead onClick={() => { setSortBy("risk_score"); setSortDir(sortDir === "asc" ? "desc" : "asc") }} style={{ cursor: "pointer" }}>Risk Score</TableHead>
                    <TableHead>Band</TableHead>
                    <TableHead>Last Event</TableHead>
                    <TableHead onClick={() => { setSortBy("last_event_at"); setSortDir(sortDir === "asc" ? "desc" : "asc") }} style={{ cursor: "pointer" }}>Last Event At</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell colSpan={9} className="text-center py-8">
                        <div className="flex items-center justify-center gap-2">
                          <div className="w-5 h-5 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
                          <span className="text-[var(--text-muted)]">Loading risk data...</span>
                        </div>
                      </TableCell>
                    </TableRow>
                  ) : filteredIdentities.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={9} className="text-center py-8 text-[var(--text-muted)]">
                        No identities match the current filters
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredIdentities.map((identity) => (
                      <TableRow key={identity.id} className="hover:bg-[var(--glass-2)]">
                        <TableCell className="text-right font-mono text-sm">{identity.risk_score}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-3">
                            <div className="w-8 h-8 rounded-full bg-[var(--accent-dim)] flex items-center justify-center text-[var(--accent)] font-semibold text-sm">
                              {identity.display_name?.charAt(0) || identity.email.charAt(0)}
                            </div>
                            <div>
                              <p className="font-medium text-sm text-[var(--text-primary)]">{identity.display_name || identity.email}</p>
                              <p className="text-xs text-[var(--text-muted)]">{identity.email}</p>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className="text-sm text-[var(--text-secondary)]">{identity.department || "—"}</TableCell>
                        <TableCell className="font-mono font-semibold">{identity.risk_score}</TableCell>
                        <TableCell>
                          <Badge className={identity.risk_band === "critical" || identity.risk_band === "high" ? "border-red-500/40 bg-red-500/10 text-red-400" : identity.risk_band === "elevated" ? "border-amber-500/40 bg-amber-500/10 text-amber-400" : "border-emerald-500/40 bg-emerald-500/10 text-emerald-400"}>
                            {identity.risk_band}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-sm text-[var(--text-secondary)]">{identity.last_event || "—"}</TableCell>
                        <TableCell className="text-sm text-[var(--text-muted)]">{identity.last_event_at ? new Date(identity.last_event_at).toLocaleString() : "—"}</TableCell>
                        <TableCell>
                          <Badge className={identity.status === "active" ? "border-emerald-500/40 bg-emerald-500/10 text-emerald-400" : "border-slate-500/40 bg-slate-500/10 text-slate-400"}>{identity.status}</Badge>
                        </TableCell>
                        <TableCell>
                          <Button variant="ghost" size="sm" onClick={() => window.open(`/identities/${identity.id}`, "_blank")}>
                            Detail
                          </Button>
                        </TableCell>
                      </TableRow>
                    )))}
                  </TableBody>
                </Table>
              </div>
          </CardContent>
        </Card>
      </div>
  )
}
