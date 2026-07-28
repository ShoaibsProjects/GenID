"use client"

import { useEffect, useState, useCallback } from "react"
import {
  fetchCertifications,
  generateCertification,
  decideCertificationEntry,
  type CertificationCampaign,
  type CertificationEntry,
} from "@/lib/api"

type Decision = "approved" | "revoked"

export default function CertificationsPage() {
  const [campaigns, setCampaigns] = useState<CertificationCampaign[]>([])
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [busyEntry, setBusyEntry] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [decisions, setDecisions] = useState<Record<string, Decision>>({})

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchCertifications()
      setCampaigns(data.campaigns || [])
    } catch (e: any) {
      setError(e?.message || "Failed to load campaigns")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const handleGenerate = async () => {
    setGenerating(true)
    setError(null)
    try {
      const stamp = new Date().toISOString().slice(0, 10)
      await generateCertification({
        campaign_name: `Access Review ${stamp}`,
        campaign_type: "quarterly",
      })
      await load()
    } catch (e: any) {
      setError(e?.message || "Failed to generate campaign")
    } finally {
      setGenerating(false)
    }
  }

  const handleDecide = async (
    entry: CertificationEntry,
    campaign: CertificationCampaign,
    decision: Decision,
  ) => {
    if (!confirm(`${decision === "approved" ? "Approve" : "Revoke"} access for ${entry.display_name || entry.identity_email}?`)) return
    setBusyEntry(entry.id)
    try {
      console.log(`[CERTIFICATION][${decision.toUpperCase()}]`, {
        campaign: campaign.name,
        entry_id: entry.id,
        identity_id: entry.identity_id,
        user: entry.display_name || entry.identity_email,
        resource: entry.resource,
        at: new Date().toISOString(),
      })
      await decideCertificationEntry(entry.id, decision)
      setDecisions((prev) => ({ ...prev, [entry.id]: decision }))
    } catch (e: any) {
      setError(`Failed to record decision: ${e?.message || "unknown error"}`)
    } finally {
      setBusyEntry(null)
    }
  }

  const totalPending = campaigns.reduce(
    (acc, c) => acc + (c.pending_count || 0),
    0,
  )
  const totalDone = Object.values(decisions).length
  const latestCampaign = campaigns[0]

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Access Certifications</h1>
          <p className="text-sm text-gray-400 mt-1">
            SOC-2 access reviews & quarterly recertification campaigns
        </p>
      </div>
        <button
          className="btn-primary text-xs px-3 py-1.5 disabled:opacity-50"
          onClick={handleGenerate}
          disabled={generating}
        >
          {generating ? "Generating…" : "+ Generate Campaign"}
      </button>
    </div>

      <div className="grid grid-cols-3 gap-3">
        <div className="glass-card p-4">
          <span className="text-xs text-gray-500 uppercase">Campaigns</span>
          <div className="text-2xl font-bold text-white mt-1">{campaigns.length}</div>
      </div>
        <div className="glass-card p-4">
          <span className="text-xs text-gray-500 uppercase">Pending Reviews</span>
          <div className="text-2xl font-bold text-amber-400 mt-1">{totalPending}</div>
      </div>
        <div className="glass-card p-4">
          <span className="text-xs text-gray-500 uppercase">Decisions Logged</span>
          <div className="text-2xl font-bold text-emerald-400 mt-1">{totalDone}</div>
      </div>
    </div>

      {latestCampaign && (
        <div className="glass-card p-4 border-l-4 border-brand-400">
          <div className="text-xs text-gray-500 uppercase tracking-wider">Latest Campaign</div>
          <div className="text-lg font-semibold text-white mt-1">{latestCampaign.name}</div>
          <div className="text-xs text-gray-400 mt-1">
            {latestCampaign.pending_count} pending review{latestCampaign.pending_count === 1 ? "" : "s"} · ends {new Date(latestCampaign.ends_at).toLocaleDateString()}
        </div>
      </div>
      )}

      {error && (
        <div className="glass-card p-3 border border-red-500/40 text-sm text-red-400">
          {error}
      </div>
      )}

      {loading ? (
        <div className="glass-card p-12 text-center text-gray-500">
          Loading certification campaigns…
      </div>
      ) : campaigns.length === 0 ? (
        <div className="glass-card p-12 text-center">
          <div className="text-gray-500">
            <p className="text-sm">No certification campaigns yet</p>
            <p className="text-xs text-gray-600 mt-1">
              Click <span className="font-mono text-brand-400">+ Generate Campaign</span>{" "}
              to trigger the Temporal AccessCertificationWorkflow.
          </p>
        </div>
      </div>
      ) : (
        <div className="space-y-4">
          {campaigns.map((c) => (
            <div key={c.id} className="glass-card p-4">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <h3 className="text-sm font-semibold text-white">{c.name}</h3>
                  <p className="text-xs text-gray-500 mt-0.5">
                    <span className="font-mono text-brand-400">{c.campaign_type}</span>{" "}
                    · status: <span className="text-gray-300">{c.status}</span> ·{" "}
                    {c.pending_count} pending review
                    {c.pending_count === 1 ? "" : "s"}
                </p>
              </div>
                <span className="text-xs text-gray-500">
                  ends {new Date(c.ends_at).toLocaleDateString()}
              </span>
            </div>

              {c.entries && c.entries.length > 0 ? (
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-xs text-gray-500 uppercase border-b border-white/10">
                      <th className="py-2 pr-3">Identity</th>
                      <th className="py-2 pr-3">Resource</th>
                      <th className="py-2 pr-3">Risk Score</th>
                      <th className="py-2 pr-3 text-right">Actions</th>
                  </tr>
                </thead>
                  <tbody>
                    {c.entries.map((e) => {
                      const decision = decisions[e.id]
                      const isBusy = busyEntry === e.id
                      return (
                        <tr
                          key={e.id}
                          className="border-b border-white/5 hover:bg-white/5"
                        >
                          <td className="py-2 pr-3">
                            <div className="text-white">
                              {e.display_name || e.identity_email}
                          </div>
                            <div className="text-xs text-gray-500 font-mono">
                              {e.identity_email}
                          </div>
                        </td>
                          <td className="py-2 pr-3 text-gray-300">
                            <span className="font-mono text-xs">{e.resource}</span>
                        </td>
                          <td className="py-2 pr-3">
                            <RiskBadge score={e.risk_score} />
                        </td>
                          <td className="py-2 pr-3 text-right">
                            {decision ? (
                              <span
                                className={
                                  decision === "approved"
                                    ? "inline-block px-2 py-0.5 text-xs rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20"
                                    : "inline-block px-2 py-0.5 text-xs rounded bg-red-500/10 text-red-400 border border-red-500/20"
                                }
                              >
                                {decision}
                            </span>
                            ) : (
                              <div className="flex gap-2 justify-end">
                                <button
                                  className="text-xs px-2 py-1 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 hover:bg-emerald-500/20 disabled:opacity-50"
                                  onClick={() => handleDecide(e, c, "approved")}
                                  disabled={isBusy}
                                >
                                  {isBusy ? "…" : "Approve"}
                              </button>
                                <button
                                  className="text-xs px-2 py-1 rounded bg-red-500/10 text-red-400 border border-red-500/30 hover:bg-red-500/20 disabled:opacity-50"
                                  onClick={() => handleDecide(e, c, "revoked")}
                                  disabled={isBusy}
                                >
                                  {isBusy ? "…" : "Revoke"}
                              </button>
                            </div>
                            )}
                        </td>
                      </tr>
                      )
                    })}
                </tbody>
              </table>
              ) : (
                <p className="text-xs text-gray-500">
                  No pending review entries in this campaign.
              </p>
              )}
          </div>
          ))}
      </div>
      )}

      <div className="glass-card p-4">
        <h3 className="text-sm font-semibold text-gray-300 uppercase tracking-wider mb-2">
          About Certifications
      </h3>
        <div className="text-sm text-gray-400 space-y-1">
          <p>
            Access certifications ensure that all entitlements are reviewed
            periodically by managers and resource owners.
        </p>
          <p>
            Clicking <span className="font-mono text-brand-400">Generate Campaign</span>{" "}
            starts the Temporal{" "}
            <span className="font-mono text-brand-400">AccessCertificationWorkflow</span>{" "}
            which scans PostgreSQL for users with Administrator / Security Reviewer roles
            or risk-score {">"} 0.7, creates a campaign row, and inserts{" "}
            <span className="font-mono text-brand-400">certification_entries</span> with
            status <span className="font-mono text-brand-400">pending_review</span>.
        </p>
          <p>
            Approve / Revoke calls{" "}
            <span className="font-mono text-brand-400">POST /api/v1/certifications/entries/{`{id}`}/decide</span>{" "}
            which writes the decision to PostgreSQL and logs to the audit trail.
        </p>
      </div>
    </div>
  </div>
  )
}

function RiskBadge({ score }: { score: number }) {
  let label = "Low"
  let cls = "bg-emerald-500/10 text-emerald-400 border-emerald-500/20"
  if (score >= 0.7) {
    label = "High"
    cls = "bg-red-500/10 text-red-400 border-red-500/20"
  } else if (score >= 0.4) {
    label = "Medium"
    cls = "bg-amber-500/10 text-amber-400 border-amber-500/20"
  }
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded border ${cls}`}>
      <span>{label}</span>
      <span className="font-mono opacity-70">{score.toFixed(2)}</span>
  </span>
  )
}
