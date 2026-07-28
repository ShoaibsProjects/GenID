"use client"

import { useEffect, useState, useCallback } from "react"
import {
  fetchCertifications,
  generateCertification,
  type CertificationCampaign,
  type CertificationEntry,
} from "@/lib/api"

export default function CertificationsPage() {
  const [campaigns, setCampaigns] = useState<CertificationCampaign[]>([])
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [decisionLog, setDecisionLog] = useState<Record<string, "approved" | "revoked">>({})

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

  const handleApprove = (entry: CertificationEntry, campaignName: string) => {
    console.log("[CERTIFICATION][APPROVE]", {
      campaign: campaignName,
      entry_id: entry.id,
      identity_id: entry.identity_id,
      user: entry.display_name || entry.identity_email,
      resource: entry.resource,
      at: new Date().toISOString(),
    })
    setDecisionLog((prev) => ({ ...prev, [entry.id]: "approved" }))
  }

  const handleRevoke = (entry: CertificationEntry, campaignName: string) => {
    console.log("[CERTIFICATION][REVOKE]", {
      campaign: campaignName,
      entry_id: entry.id,
      identity_id: entry.identity_id,
      user: entry.display_name || entry.identity_email,
      resource: entry.resource,
      at: new Date().toISOString(),
    })
    setDecisionLog((prev) => ({ ...prev, [entry.id]: "revoked" }))
  }

  const totalPending = campaigns.reduce(
    (acc, c) => acc + (c.pending_count || 0),
    0,
  )

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Access Certifications</h1>
          <p className="text-sm text-gray-400 mt-1">
            Periodic access reviews and recertification campaigns
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
          <span className="text-xs text-gray-500 uppercase">Completed</span>
          <div className="text-2xl font-bold text-emerald-400 mt-1">
            {Object.values(decisionLog).length}
         </div>
       </div>
     </div>

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
                      <th className="py-2 pr-3">User</th>
                      <th className="py-2 pr-3">Resource</th>
                      <th className="py-2 pr-3">Status</th>
                      <th className="py-2 pr-3 text-right">Decision</th>
                   </tr>
                 </thead>
                  <tbody>
                    {c.entries.map((e) => {
                      const decision = decisionLog[e.id]
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
                            <span className="inline-block px-2 py-0.5 text-xs rounded bg-amber-500/10 text-amber-400 border border-amber-500/20">
                              {e.status}
                           </span>
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
                                  className="text-xs px-2 py-1 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/30 hover:bg-emerald-500/20"
                                  onClick={() => handleApprove(e, c.name)}
                                >
                                  Approve
                               </button>
                                <button
                                  className="text-xs px-2 py-1 rounded bg-red-500/10 text-red-400 border border-red-500/30 hover:bg-red-500/20"
                                  onClick={() => handleRevoke(e, c.name)}
                                >
                                  Revoke
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
            Approve / Revoke actions are logged to the browser console only — wire them
            to a Temporal decision workflow in a follow-up phase.
         </p>
       </div>
     </div>
   </div>
  )
}
