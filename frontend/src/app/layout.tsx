"use client"
import { usePathname } from "next/navigation"
import "@/styles/globals.css"

/* ─── Navigation ──────────────────────────────────────────── */
const NAV = [
  {
    group: "Overview",
    items: [
      { href: "/dashboard",     label: "Dashboard",     icon: "◈" },
      { href: "/audit",         label: "Access Logs",   icon: "⊞" },
    ]
  },
  {
    group: "Identities",
    items: [
      { href: "/identities",    label: "People",        icon: "⊙" },
      { href: "/agents",        label: "Machines",      icon: "◇" },
    ]
  },
  {
    group: "Governance",
    items: [
      { href: "/connectors",    label: "Directories",   icon: "◉" },
      { href: "/groups",        label: "Roles & Groups",icon: "▣" },
      { href: "/access",        label: "Access Control",icon: "◈" },
      { href: "/policies",      label: "Policies",      icon: "⊡" },
    ]
  },
  {
    group: "Compliance",
    items: [
      { href: "/certifications",label: "Reviews",       icon: "◎" },
      { href: "/sod",           label: "SoD Guard",     icon: "▲" },
    ]
  },
  {
    group: "System",
    items: [
      { href: "/vault",         label: "Vault",         icon: "◬" },
      { href: "/idp",           label: "IDP / OIDC",    icon: "⊛" },
      { href: "/settings",      label: "Settings",      icon: "⚙" },
    ]
  },
]

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className="dark">
      <body>
        <div className="flex h-screen overflow-hidden">
          <Sidebar />
          <main className="flex-1 overflow-y-auto relative z-10">
            <div className="px-10 py-8 max-w-[1640px] mx-auto animate-fade-in">
              {children}
            </div>
          </main>
        </div>
      </body>
    </html>
  )
}

function Sidebar() {
  return (
    <aside className="w-64 h-screen shrink-0 flex flex-col relative z-20" style={{ background: 'rgba(12, 12, 16, 0.85)', backdropFilter: 'blur(32px)', borderRight: '1px solid rgba(255, 255, 255, 0.04)' }}>
      {/* Logo */}
      <div className="px-5 py-6 border-b" style={{ borderColor: 'rgba(255, 255, 255, 0.04)' }}>
        <div className="flex items-center gap-3">
          <div style={{ width: 36, height: 36, borderRadius: 10, background: 'linear-gradient(135deg, rgba(245,158,11,0.15), rgba(217,119,6,0.20))', display: 'flex', alignItems: 'center', justifyContent: 'center', border: '1px solid rgba(245,158,11,0.20)' }}>
            <svg style={{ width: 18, height: 18, color: '#F59E0B' }} fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
          </div>
          <div>
            <h1 className="text-base font-bold tracking-tight leading-none" style={{ background: 'linear-gradient(135deg, #FBBF24, #F59E0B, #D97706)', WebkitBackgroundClip: 'text', WebkitTextFillColor: 'transparent' }}>ObserveID</h1>
            <p className="text-[0.55rem] font-semibold uppercase tracking-[0.15em]" style={{ color: '#5C5C62', marginTop: 2 }}>Fabric v1</p>
          </div>
        </div>
      </div>

      {/* Nav */}
      <nav className="flex-1 overflow-y-auto px-3 py-5 space-y-6">
        {NAV.map((group) => (
          <div key={group.group}>
            <p className="px-3 mb-2 text-[0.55rem] font-bold uppercase tracking-[0.15em]" style={{ color: '#5C5C62' }}>{group.group}</p>
            <div className="space-y-0.5">
              {group.items.map((item) => (
                <NavItem key={item.href} href={item.href} icon={item.icon} label={item.label} />
              ))}
            </div>
          </div>
        ))}
      </nav>

      {/* Footer */}
      <div className="px-4 py-4 border-t" style={{ borderColor: 'rgba(255, 255, 255, 0.04)' }}>
        <div className="flex items-center gap-2.5">
          <span style={{ width: 7, height: 7, borderRadius: '50%', background: '#34D399', boxShadow: '0 0 8px rgba(52, 211, 153, 0.4)' }} />
          <span className="text-xs" style={{ color: '#5C5C62' }}>Identity Fabric</span>
          <span className="text-xs" style={{ color: '#34D399' }}>Live</span>
        </div>
      </div>
    </aside>
  )
}

function NavItem({ href, icon, label }: { href: string; icon: string; label: string }) {
  const pathname = usePathname()
  const isActive = pathname ? pathname.startsWith(href) : false
  return (
    <a
      href={href}
      style={{
        display: 'flex', alignItems: 'center', gap: 10,
        padding: '8px 12px', borderRadius: 8,
        fontSize: '0.8125rem', fontWeight: isActive ? 600 : 450,
        textDecoration: 'none',
        color: isActive ? '#FBBF24' : '#5C5C62',
        background: isActive ? 'rgba(245, 158, 11, 0.05)' : 'transparent',
        borderLeft: isActive ? '2px solid #F59E0B' : '2px solid transparent',
        transition: 'all 0.2s cubic-bezier(0.16, 1, 0.3, 1)',
      }}
      onMouseEnter={(e) => { if (!isActive) { e.currentTarget.style.background = 'rgba(255,255,255,0.03)'; e.currentTarget.style.color = '#F0EFEC'; }}}
      onMouseLeave={(e) => { if (!isActive) { e.currentTarget.style.background = 'transparent'; e.currentTarget.style.color = '#5C5C62'; }}}
    >
      <span style={{ fontSize: '1rem', opacity: 0.7 }}>{icon}</span>
      {label}
    </a>
  )
}
