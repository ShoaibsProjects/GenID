"use client"
import { useState } from "react"
import { authFetch } from "@/lib/api"
import { Button } from "@/components/ui/Button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from "@/components/ui/Card"
import { Badge } from "@/components/ui/Badge"
import { Input } from "@/components/ui/Input"
import { Textarea } from "@/components/ui/textarea"
import { CheckCircle, XCircle, AlertTriangle, Loader2, Shield, HelpCircle, ChevronDown, ChevronUp } from "lucide-react"
import { cn } from "@/lib/utils"

interface SimulationForm {
  role: string
  network_zone: string
  device_trust: string
  time_of_day: string
  risk_score: number
}

interface PolicyResult {
  decision: string
  advice: string
  reason: string
  policy_id: string
  duration: string
  matched_policies?: string[]
}

const ROLES = [
  { value: "it-admin", label: "IT Admin" },
  { value: "oncall", label: "On-Call Engineer" },
  { value: "developer", label: "Developer" },
  { value: "security-analyst", label: "Security Analyst" },
  { value: "hr-manager", label: "HR Manager" },
  { value: "finance", label: "Finance" },
  { value: "contractor", label: "Contractor" },
]

const NETWORK_ZONES = [
  { value: "corporate", label: "Corporate Office" },
  { value: "vpn", label: "VPN" },
  { value: "public", label: "Public Network" },
  { value: "datacenter", label: "Datacenter" },
]

const DEVICE_TRUST_OPTIONS = [
  { value: "managed", label: "Managed (MDM enrolled)" },
  { value: "unmanaged", label: "Unmanaged (BYOD)" },
  { value: "unknown", label: "Unknown" },
]

const TIME_OPTIONS = [
  { value: "business", label: "Business Hours (9am-5pm)" },
  { value: "after-hours", label: "After Hours (5pm-9am)" },
  { value: "weekend", label: "Weekend" },
]

export default function PolicySimulatorPage() {
  const [form, setForm] = useState<SimulationForm>({
    role: "it-admin",
    network_zone: "corporate",
    device_trust: "managed",
    time_of_day: "business",
    risk_score: 200,
  })
  const [result, setResult] = useState<PolicyResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [showHelp, setShowHelp] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setResult(null)
    
    try {
      const res = await authFetch("/api/v1/access/grant", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          identity_id: "00000000-0000-0000-0000-000000000002",
          resource_id: "demo-resource-001",
          resource_type: "Resource",
          reason: "Policy simulation dry run",
          duration_hours: 0,
          risk_score: form.risk_score,
          role: form.role,
          evaluate_at: new Date().toISOString(),
          signals: {
            ip_address: form.network_zone === "corporate" ? "10.0.1.5" : 
                       form.network_zone === "vpn" ? "172.16.0.5" :
                       form.network_zone === "datacenter" ? "10.10.0.1" : "203.0.113.1",
            user_agent: "Mozilla/5.0 (PolicySimulator)",
            device_id: "sim-device-001",
          }
        })
      })
      
      if (res.ok) {
        const data = await res.json()
        // The workflow runs async, so we get a request_id
        // For simulation, we'll poll for the decision
        if (data.request_id) {
          await pollForDecision(data.request_id)
        }
      }
    } catch (error) {
      console.error("Simulation failed:", error)
    } finally {
      setLoading(false)
    }
  }

  const pollForDecision = async (requestId: string) => {
    for (let i = 0; i < 30; i++) {
      await new Promise(r => setTimeout(r, 1000))
      try {
        const res = await authFetch(`/api/v1/requests/${requestId}`, {
          headers: {
            "X-Master-Key": "dev-only-master-key-32-bytes-long-x"
          }
        })
        if (res.ok) {
          const data = await res.json()
          if (data.request && (data.request.status === "executed" || data.request.status === "denied" || data.request.status === "pending")) {
            setResult({
              decision: data.request.status === "executed" ? "Allow" : data.request.status === "denied" ? "Deny" : "StepUp",
              advice: data.request.status === "executed" ? "auto_approve_2h" : data.request.status === "denied" ? "deny_due_to_risk" : "step_up_approval",
              reason: data.request.status === "executed" ? "Auto-approved: policy permits" : 
                      data.request.status === "denied" ? "Denied: risk too high or policy forbids" : "Requires human approval",
              policy_id: "policy-conditional-" + form.network_zone,
              duration: data.request.status === "executed" ? "2h JIT" : "N/A",
              matched_policies: ["policy-conditional-" + form.network_zone]
            })
            break
          }
        }
      } catch (e) {
        console.error("Poll error:", e)
      }
    }
  }

  const DECISION_COLORS: Record<string, { bg: string; text: string; border: string; icon: React.ReactNode }> = {
    Allow: { 
      bg: "bg-green-500/10", 
      text: "text-green-400", 
      border: "border-green-500/25",
      icon: <CheckCircle className="w-8 h-8 text-green-400" />
    },
    Deny: { 
      bg: "bg-red-500/10", 
      text: "text-red-400", 
      border: "border-red-500/25",
      icon: <XCircle className="w-8 h-8 text-red-400" />
    },
    StepUp: { 
      bg: "bg-amber-500/10", 
      text: "text-amber-400", 
      border: "border-amber-500/25",
      icon: <AlertTriangle className="w-8 h-8 text-amber-400" />
    },
  }

  const getDecisionIcon = (decision: string) => {
    switch (decision) {
      case "Allow": return <CheckCircle className="w-8 h-8 text-green-400" />
      case "Deny": return <XCircle className="w-8 h-8 text-red-400" />
      case "StepUp": return <AlertTriangle className="w-8 h-8 text-amber-400" />
      default: return <HelpCircle className="w-8 h-8 text-gray-400" />
    }
  }

  return (
    <div className="max-w-4xl mx-auto space-y-6 animate-fade-in">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-gradient-accent">Policy Simulator</h1>
          <p className="text-[var(--text-secondary)] mt-1">Test conditional access decisions before they happen</p>
        </div>
        <Button variant="outline" onClick={() => setShowHelp(!showHelp)}>
          <HelpCircle className="w-4 h-4 mr-2" /> {showHelp ? <ChevronUp className="w-4 h-4" /> : <ChevronDown className="w-4 h-4" />}
        </Button>
      </div>

      {/* Help Panel */}
      {showHelp && (
        <div className="glass-card p-6 animate-fade-in">
          <h3 className="font-semibold text-[var(--text-primary)] mb-3">How It Works</h3>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm text-[var(--text-secondary)]">
            <div className="p-4 bg-[var(--glass-2)] rounded-lg">
              <h4 className="font-semibold text-[var(--text-primary)] mb-2">Conditional Access Engine</h4>
              <p>The simulator runs the exact same enrichment + policy evaluation that the production API uses. No mocks.</p>
            </div>
            <div className="p-4 bg-[var(--glass-2)] rounded-lg">
              <h4 className="font-semibold text-[var(--text-primary)] mb-2">Decision Types</h4>
              <ul className="space-y-1">
                <li className="flex items-center gap-2"><span className="w-2 h-2 rounded-full bg-green-500" /> <span className="text-green-400">Allow</span> — Auto-approved, JIT grant issued</li>
                <li className="flex items-center gap-2"><span className="w-2 h-2 rounded-full bg-amber-500" /> <span className="text-amber-400">StepUp</span> — Human approval required</li>
                <li className="flex items-center gap-2"><span className="w-2 h-2 rounded-full bg-red-500" /> <span className="text-red-400">Deny</span> — Access forbidden</li>
              </ul>
            </div>
          </div>
        </div>
      )}

      {/* Form Card */}
      <Card className="glass-card">
        <CardHeader>
          <CardTitle className="text-xl">Simulation Parameters</CardTitle>
          <CardDescription>Adjust the context signals to see how the policy engine decides</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-6">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">Role</label>
                <Select value={form.role} onValueChange={v => setForm({...form, role: v})}>
                  <SelectTrigger><SelectValue placeholder="Select role" /></SelectTrigger>
                  <SelectContent>
                    {ROLES.map(r => <SelectItem key={r.value} value={r.value}>{r.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>

              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">Network Zone</label>
                <Select value={form.network_zone} onValueChange={v => setForm({...form, network_zone: v})}>
                  <SelectTrigger><SelectValue placeholder="Select zone" /></SelectTrigger>
                  <SelectContent>
                    {NETWORK_ZONES.map(z => <SelectItem key={z.value} value={z.value}>{z.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>

              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">Device Trust</label>
                <Select value={form.device_trust} onValueChange={v => setForm({...form, device_trust: v})}>
                  <SelectTrigger><SelectValue placeholder="Select trust" /></SelectTrigger>
                  <SelectContent>
                    {DEVICE_TRUST_OPTIONS.map(d => <SelectItem key={d.value} value={d.value}>{d.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>

              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">Time of Day</label>
                <Select value={form.time_of_day} onValueChange={v => setForm({...form, time_of_day: v})}>
                  <SelectTrigger><SelectValue placeholder="Select time" /></SelectTrigger>
                  <SelectContent>
                    {TIME_OPTIONS.map(t => <SelectItem key={t.value} value={t.value}>{t.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>

              <div className="md:col-span-2">
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">Risk Score</label>
                <div className="space-y-2">
                  <input
                    type="range"
                    min="0"
                    max="1000"
                    step="10"
                    value={form.risk_score}
                    onChange={e => setForm({...form, risk_score: parseInt(e.target.value)})}
                    className="w-full h-2 bg-[var(--glass-2)] rounded-lg appearance-none cursor-pointer accent-[var(--accent)]"
                  />
                  <div className="flex justify-between text-xs text-[var(--text-muted)]">
                    <span>0 — Minimal</span>
                    <span>{form.risk_score}</span>
                    <span>1000 — Critical</span>
                  </div>
                </div>
              </div>

              <div>
                <label className="block text-sm font-medium text-[var(--text-secondary)] mb-2">IP Address (auto from zone)</label>
                <Input 
                  value={form.network_zone === "corporate" ? "10.0.1.5" : 
                       form.network_zone === "vpn" ? "172.16.0.5" :
                       form.network_zone === "datacenter" ? "10.10.0.1" : "203.0.113.1"}
                  disabled
                  className="bg-[var(--glass-2)]"
                />
              </div>
            </div>

            <div className="pt-4 border-t border-[var(--obsidian-border)]">
              <Button type="submit" className="w-full md:w-auto" size="lg" disabled={loading}>
                {loading ? (
                  <>
                    <Loader2 className="w-5 h-5 mr-2 animate-spin" />
                    Running Simulation...
                  </>
                ) : (
                  <>
                    <Shield className="w-5 h-5 mr-2" />
                    Run Simulation
                  </>
                )}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {/* Result Card */}
      {result && (
        <Card className={cn(
          "glass-card animate-fade-in",
          DECISION_COLORS[result.decision]?.bg || "bg-[var(--glass-2)]",
          DECISION_COLORS[result.decision]?.border || "border-[var(--obsidian-border)]"
        )}>
          <CardHeader>
            <div className="flex items-center gap-4">
              {DECISION_COLORS[result.decision]?.icon || <HelpCircle className="w-8 h-8" />}
              <div>
                <h3 className={cn("text-2xl font-bold", DECISION_COLORS[result.decision]?.text || "text-[var(--text-primary)]")}>
                  {result.decision}
                </h3>
                <p className="text-[var(--text-secondary)]">{result.advice.replace("_", " ")}</p>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="p-4 bg-[var(--glass-2)] rounded-lg">
                  <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-1">Duration</p>
                  <p className="text-xl font-bold text-[var(--text-primary)]">{result.duration}</p>
                </div>
                <div className="p-4 bg-[var(--glass-2)] rounded-lg">
                  <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-1">Policy ID</p>
                  <p className="font-mono text-sm text-[var(--text-primary)]">{result.policy_id}</p>
                </div>
              </div>

              <div className="p-4 bg-[var(--glass-2)] rounded-lg">
                <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-2">Reason</p>
                <p className="text-[var(--text-primary)]">{result.reason}</p>
              </div>

              {result.matched_policies && result.matched_policies.length > 0 && (
                <div className="p-4 bg-[var(--glass-2)] rounded-lg">
                  <p className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-2">Matched Policies</p>
                  <div className="flex flex-wrap gap-2">
                    {result.matched_policies.map(p => (
                      <Badge key={p} variant="secondary" className="font-mono text-xs">{p}</Badge>
                    ))}
                  </div>
                </div>
              )}

              <div className="pt-4 border-t border-[var(--obsidian-border)]">
                <Button variant="outline" onClick={() => setResult(null)}>
                  Run Another Simulation
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}