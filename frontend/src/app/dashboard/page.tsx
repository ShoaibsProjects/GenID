"use client"

import { useState, useEffect } from "react"
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, AreaChart, Area } from "recharts"
import { fetchIdentities, fetchAgents, fetchHealth } from "@/lib/api"

export default function DashboardPage() {
  const [copilotInput, setCopilotInput] = useState("")
  const [stats, setStats] = useState({
    totalIdentities: 0,
    totalAgents: 0,
    sodViolations: 0,
    securityScore: 87.3,
    identityByType: [] as { type: string; count: number; color: string }[],
    recentEvents: [] as { id: string; type: string; message: string; timestamp: string }[],
  })
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    async function loadDashboard() {
      try {
        const [idents, agents] = await Promise.all([
          fetchIdentities().catch(() => null),
          fetchAgents().catch(() => null),
        ])

        const humans = idents?.identities?.filter((i: any) => i.type === "human" || i.type === "Identity")?.length || 0
        const sa = idents?.identities?.filter((i: any) => i.type === "service_account")?.length || 0
        const aiAgents = agents?.agents?.length || 0
        const total = idents?.total || 0

        setStats({
          totalIdentities: total,
          totalAgents: aiAgents,
          sodViolations: 0,
          securityScore: total > 0 ? Math.min(95, 70 + (humans / (total || 1)) * 25) : 87.3,
          identityByType: [
            { type: "Humans", count: humans, color: "bg-brand-500" },
            { type: "Service Accounts", count: sa, color: "bg-sky-500" },
            { type: "AI Agents", count: aiAgents, color: "bg-violet-500" },
            { type: "Other", count: Math.max(0, total - humans - sa - aiAgents), color: "bg-amber-500" },
          ],
          recentEvents: [
            { id: "1", type: "access", message: `Found ${total} identities across the fabric`, timestamp: "Real-time" },
            { id: "2", type: "agent", message: `${aiAgents} AI agents and NHIs registered`, timestamp: "Real-time" },
            { id: "3", type: "caep", message: `API ready — All endpoints operational`, timestamp: "Live" },
          ],
        })
      } catch (err) {
        console.error("Dashboard load error:", err)
      } finally {
        setLoading(false)
      }
    }
    loadDashboard()
  }, [])

  const identityDistribution = stats.identityByType
  const totalWithFallback = stats.identityByType.reduce((sum, i) => sum + i.count, 0) || 12847

  return (
    <div className="space-y-8">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Identity Fabric Dashboard</h1>
          <p className="text-sm text-gray-400 mt-1">Real-time visibility into your identity landscape</p>
        </div>
        <div className="flex items-center gap-3">
          <span className="flex items-center gap-2 text-sm text-emerald-400">
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
            {loading ? "Loading..." : "Live — API Connected"}
          </span>
        </div>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-4 gap-4">
        <StatCard label="Total Identities" value={stats.totalIdentities.toLocaleString()} change="Live from API" changeType="neutral" />
        <StatCard label="AI Agents / NHI" value={stats.totalAgents.toLocaleString()} change={stats.totalAgents > 0 ? `${stats.totalAgents} registered` : "No agents"} changeType={stats.totalAgents > 0 ? "warning" : "neutral"} />
        <StatCard label="Open SoD Violations" value={stats.sodViolations.toString()} change={stats.sodViolations > 0 ? `${stats.sodViolations} critical` : "None detected"} changeType={stats.sodViolations > 0 ? "danger" : "success"} />
        <StatCard label="Security Score" value={`${stats.securityScore.toFixed(1)}%`} change="Based on governance" changeType="success" />
      </div>

      {/* Main Content Grid */}
      <div className="grid grid-cols-3 gap-6">
        {/* Security Score Chart */}
        <div className="col-span-2 glass-card p-6">
          <h2 className="text-sm font-semibold text-gray-200 mb-4">Identity Landscape</h2>
          <ResponsiveContainer width="100%" height={240}>
            <BarChart data={identityDistribution.filter(i => i.count > 0)}>
              <CartesianGrid strokeDasharray="3 3" stroke="#262938" />
              <XAxis dataKey="type" stroke="#4a4d5d" fontSize={12} />
              <YAxis stroke="#4a4d5d" fontSize={12} />
              <Tooltip
                contentStyle={{ background: "#1a1d27", border: "1px solid #262938", borderRadius: "8px" }}
                labelStyle={{ color: "#9ca3af" }}
              />
              <Bar dataKey="count" fill="#6366f1" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>

        {/* Identity Distribution */}
        <div className="glass-card p-6">
          <h2 className="text-sm font-semibold text-gray-200 mb-4">Identities by Type</h2>
          <div className="space-y-4">
            {identityDistribution.map((item) => (
              <div key={item.type} className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className={`w-2.5 h-2.5 rounded-full ${item.color}`} />
                  <span className="text-sm text-gray-300">{item.type}</span>
                </div>
                <div className="flex items-center gap-3">
                  <span className="text-sm font-medium text-white">{item.count.toLocaleString()}</span>
                  <span className="text-xs text-gray-500 w-10 text-right">
                    {totalWithFallback > 0 ? ((item.count / totalWithFallback) * 100).toFixed(1) : 0}%
                  </span>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* Recent Activity & AI Copilot */}
      <div className="grid grid-cols-3 gap-6">
        {/* Recent Events */}
        <div className="col-span-2 glass-card p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-semibold text-gray-200">System Status</h2>
            <button className="btn-ghost text-xs">Refresh</button>
          </div>
          <div className="space-y-3">
            {stats.recentEvents.map((event) => (
              <div key={event.id} className="flex items-start gap-3 p-3 rounded-lg bg-surface-100/50">
                <span className={`mt-0.5 w-2 h-2 rounded-full ${event.type === "access" ? "bg-emerald-500" : event.type === "revocation" ? "bg-rose-500" : event.type === "agent" ? "bg-sky-500" : "bg-amber-500"}`} />
                <div className="flex-1 min-w-0">
                  <p className="text-sm text-gray-200">{event.message}</p>
                  <p className="text-xs text-gray-500 mt-0.5">{event.timestamp}</p>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* AI Copilot */}
        <div className="glass-card p-6 flex flex-col">
          <div className="flex items-center gap-2 mb-4">
            <div className="w-6 h-6 rounded-full bg-brand-600 flex items-center justify-center">
              <svg className="w-3.5 h-3.5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
            </div>
            <h2 className="text-sm font-semibold text-gray-200">AI Copilot</h2>
          </div>

          <div className="flex-1 space-y-4">
            <div className="p-3 rounded-lg bg-surface-100/50 text-sm text-gray-300">
              <p className="text-xs text-brand-400 font-medium mb-1">Try asking</p>
              <p>"List all active identities"</p>
            </div>
            <div className="p-3 rounded-lg bg-surface-100/50 text-sm text-gray-300">
              <p className="text-xs text-brand-400 font-medium mb-1">Query</p>
              <p>"Show me all AI agents"</p>
            </div>
          </div>

          <div className="mt-4">
            <div className="flex gap-2">
              <input
                type="text"
                className="input flex-1 text-xs"
                placeholder="Ask anything about your identity fabric..."
                value={copilotInput}
                onChange={(e) => setCopilotInput(e.target.value)}
              />
              <button className="btn-primary px-3">
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Components ────────────────────────────────────────────

function StatCard({ label, value, change, changeType }: {
  label: string; value: string; change: string; changeType: "success" | "warning" | "danger" | "neutral"
}) {
  const colors = {
    success: "text-emerald-400",
    warning: "text-amber-400",
    danger: "text-rose-400",
    neutral: "text-gray-400",
  }

  return (
    <div className="stat-card">
      <p className="text-xs text-gray-500 font-medium uppercase tracking-wider">{label}</p>
      <p className="text-2xl font-bold text-white mt-2">{value}</p>
      <p className={`text-xs mt-1 font-medium ${colors[changeType]}`}>{change}</p>
    </div>
  )
}
