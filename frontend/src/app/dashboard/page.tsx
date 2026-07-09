"use client"

import { useState } from "react"
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, AreaChart, Area } from "recharts"

export default function DashboardPage() {
  const [copilotInput, setCopilotInput] = useState("")

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
            Real-Time Active
          </span>
        </div>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-4 gap-4">
        <StatCard label="Total Identities" value="12,847" change="+142 today" changeType="neutral" />
        <StatCard label="AI Agents / NHI" value="3,291" change="+89 ungoverned" changeType="warning" />
        <StatCard label="Open SoD Violations" value="47" change="+12 critical" changeType="danger" />
        <StatCard label="Security Score" value="87.3%" change="+2.1% this week" changeType="success" />
      </div>

      {/* Main Content Grid */}
      <div className="grid grid-cols-3 gap-6">
        {/* Security Score Chart */}
        <div className="col-span-2 glass-card p-6">
          <h2 className="text-sm font-semibold text-gray-200 mb-4">Security Score Trend</h2>
          <ResponsiveContainer width="100%" height={240}>
            <AreaChart data={securityTrendData}>
              <defs>
                <linearGradient id="scoreGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="#6366f1" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="#6366f1" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="#262938" />
              <XAxis dataKey="day" stroke="#4a4d5d" fontSize={12} />
              <YAxis domain={[75, 95]} stroke="#4a4d5d" fontSize={12} />
              <Tooltip
                contentStyle={{ background: "#1a1d27", border: "1px solid #262938", borderRadius: "8px" }}
                labelStyle={{ color: "#9ca3af" }}
              />
              <Area type="monotone" dataKey="score" stroke="#6366f1" fill="url(#scoreGradient)" strokeWidth={2} />
            </AreaChart>
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
                  <span className="text-xs text-gray-500 w-10 text-right">{item.percentage}%</span>
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
            <h2 className="text-sm font-semibold text-gray-200">Recent Activity</h2>
            <button className="btn-ghost text-xs">View All</button>
          </div>
          <div className="space-y-3">
            {recentEvents.map((event) => (
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
              <p className="text-xs text-brand-400 font-medium mb-1">Suggestion</p>
              <p>"Why does Alice have access to Prod DB?"</p>
            </div>
            <div className="p-3 rounded-lg bg-surface-100/50 text-sm text-gray-300">
              <p className="text-xs text-brand-400 font-medium mb-1">Quick Query</p>
              <p>"Show blast radius for Engineering team"</p>
            </div>
            <div className="p-3 rounded-lg bg-surface-100/50 text-sm text-gray-300">
              <p className="text-xs text-rose-400 font-medium mb-1">Alert</p>
              <p>"Found 12 anomalous agent behaviors in last hour"</p>
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

// ─── Mock Data ─────────────────────────────────────────────

const securityTrendData = [
  { day: "Mon", score: 85.2 },
  { day: "Tue", score: 86.1 },
  { day: "Wed", score: 85.8 },
  { day: "Thu", score: 86.7 },
  { day: "Fri", score: 87.3 },
  { day: "Sat", score: 87.1 },
  { day: "Sun", score: 87.3 },
]

const identityDistribution = [
  { type: "Humans", count: 9556, percentage: 74, color: "bg-brand-500" },
  { type: "Service Accounts", count: 2411, percentage: 19, color: "bg-sky-500" },
  { type: "AI Agents", count: 643, percentage: 5, color: "bg-violet-500" },
  { type: "RPA Bots", count: 192, percentage: 1.5, color: "bg-amber-500" },
  { type: "IoT / Devices", count: 45, percentage: 0.5, color: "bg-emerald-500" },
]

const recentEvents = [
  { id: "1", type: "access", message: "Alice Johnson granted Engineer role access to AWS Production", timestamp: "2 minutes ago" },
  { id: "2", type: "revocation", message: "Contractor role access revoked from Bob Smith — Offboarding completed", timestamp: "7 minutes ago" },
  { id: "3", type: "agent", message: "deploy-bot AI agent — Kill switch activated, cascade revoking 3 delegated agents", timestamp: "15 minutes ago" },
  { id: "4", type: "caep", message: "CAEP session-revoked broadcast sent to Okta, AWS, and Slack for terminated user", timestamp: "22 minutes ago" },
  { id: "5", type: "anomaly", message: "Anomaly detected: Service account 'backup-svc' accessed PII data outside business hours", timestamp: "1 hour ago" },
]
