"use client"

export default function HomePage() {
  return (
    <div className="flex items-center justify-center min-h-[80vh]">
      <div className="text-center max-w-2xl">
        <div className="w-16 h-16 rounded-2xl bg-brand-600 mx-auto mb-6 flex items-center justify-center shadow-lg shadow-brand-600/20">
          <svg className="w-8 h-8 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
              d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
          </svg>
        </div>

        <h1 className="text-4xl font-bold text-white mb-4">
          ObserveID Reimagined Identity Fabric
        </h1>
        <p className="text-lg text-gray-400 mb-8 leading-relaxed">
          Event-Driven, AI-Native Identity Governance Platform.
          <br />
          Real-time access control for humans, AI agents, and machines.
        </p>

        <div className="flex items-center justify-center gap-4">
          <a href="/dashboard" className="btn-primary px-8 py-3 text-base">
            Launch Dashboard
          </a>
          <a href="/identities" className="btn-secondary px-8 py-3 text-base">
            View Identities
          </a>
        </div>

        <div className="mt-12 grid grid-cols-3 gap-6 text-left">
          <div className="glass-card p-5">
            <div className="w-10 h-10 rounded-lg bg-brand-600/20 flex items-center justify-center mb-3">
              <svg className="w-5 h-5 text-brand-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M13 10V3L4 14h7v7l9-11h-7z" />
              </svg>
            </div>
            <h3 className="text-sm font-semibold text-white mb-1">GraphRAG AI Copilot</h3>
            <p className="text-xs text-gray-400">Natural language identity queries with graph-based intelligence</p>
          </div>

          <div className="glass-card p-5">
            <div className="w-10 h-10 rounded-lg bg-sky-600/20 flex items-center justify-center mb-3">
              <svg className="w-5 h-5 text-sky-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M3.75 3v11.25A2.25 2.25 0 006 16.5h2.25M3.75 3h-1.5m1.5 0h16.5m0 0h1.5m-1.5 0v11.25A2.25 2.25 0 0118 16.5h-2.25m-7.5 0h7.5m-7.5 0l-1 3m8.5-3l1 3m0 0l.5 1.5m-.5-1.5h-9.5m0 0l-.5 1.5" />
              </svg>
            </div>
            <h3 className="text-sm font-semibold text-white mb-1">Agent Identity Platform</h3>
            <p className="text-xs text-gray-400">First-class identity for AI agents with kill switch and delegation chains</p>
          </div>

          <div className="glass-card p-5">
            <div className="w-10 h-10 rounded-lg bg-emerald-600/20 flex items-center justify-center mb-3">
              <svg className="w-5 h-5 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M12 6v6h4.5m4.5 0a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <h3 className="text-sm font-semibold text-white mb-1">Real-Time Durable Execution</h3>
            <p className="text-xs text-gray-400">Temporal-powered workflows with guaranteed delivery and retry</p>
          </div>
        </div>
      </div>
    </div>
  )
}
