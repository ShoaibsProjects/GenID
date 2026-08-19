import { useState } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { cn } from "@/lib/utils"
import {
  LayoutDashboard,
  ClipboardList,
  Inbox,
  Activity,
  Database,
  Users,
  Shield,
  Bot,
  FileText,
  Settings,
  ChevronLeft,
  ChevronRight,
  Sun,
  Moon,
  Link2,
} from "lucide-react"

interface NavItem {
  href: string
  label: string
  icon: any
}

const navItems: NavItem[] = [
  { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { href: "/risk", label: "Risk", icon: Activity },
  { href: "/inbox", label: "Inbox", icon: Inbox },
  { href: "/catalog", label: "Catalog", icon: Database },
  { href: "/identities", label: "Identities", icon: Users },
  { href: "/policies", label: "Roles", icon: Shield },
  { href: "/connectors", label: "Integrations", icon: Link2 },
  { href: "/nhi", label: "NHI Registry", icon: Bot },
  { href: "/audit", label: "Audit", icon: FileText },
  { href: "/settings", label: "Settings", icon: Settings },
]

export function Sidebar({ isOpen, onClose }: { isOpen: boolean; onClose: () => void }) {
  const pathname = usePathname()
  const [collapsed, setCollapsed] = useState(false)

  return (
    <div
      className="flex-shrink-0 flex flex-col h-screen z-40 transition-all duration-300 bg-[var(--obsidian-raised)] border-r border-[var(--obsidian-border)] overflow-hidden"
      style={{ width: collapsed ? "72px" : "288px" }}
    >
      {/* Logo */}
      <div className="flex items-center justify-between h-16 px-4 border-b border-[var(--obsidian-border)]">
        <Link href="/dashboard" className="flex items-center gap-3">
          <div className="w-9 h-9 rounded-lg flex items-center justify-center flex-shrink-0" style={{ background: 'linear-gradient(135deg, #F59E0B, #D97706)' }}>
            <svg className="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
          </div>
          {!collapsed && <span className="font-semibold text-lg text-[var(--text-primary)]">GenID</span>}
        </Link>
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="text-[var(--text-muted)] hover:text-[var(--text-primary)] p-2 rounded-lg hover:bg-[var(--glass-2)] transition-colors"
        >
          {collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronLeft className="h-4 w-4" />}
        </button>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto py-4 px-3" role="navigation" aria-label="Main navigation">
        <ul className="space-y-1">
          {navItems.map((item) => {
            const isActive = pathname === item.href || pathname.startsWith(item.href + "/")
            const Icon = item.icon
            return (
              <li key={item.href}>
                <Link
                  href={item.href}
                  className={cn(
                    "flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all duration-200",
                    isActive
                      ? "bg-[var(--accent-dim)] text-[var(--accent)]"
                      : "text-[var(--text-muted)] hover:bg-[var(--glass-2)] hover:text-[var(--text-primary)]"
                  )}
                >
                  <Icon className="h-5 w-5 flex-shrink-0" />
                  {!collapsed && <span className="font-medium text-sm">{item.label}</span>}
                </Link>
              </li>
            )
          })}
        </ul>
      </nav>

      {/* Footer */}
      <div className="p-3 border-t border-[var(--obsidian-border)]">
        <button
          onClick={() => {
            const el = document.documentElement
            const isDark = el.classList.contains("dark")
            el.classList.toggle("dark", !isDark)
            localStorage.setItem("theme", isDark ? "light" : "dark")
          }}
          className="w-full flex items-center gap-3 px-3 py-2.5 rounded-xl text-[var(--text-muted)] hover:bg-[var(--glass-2)] hover:text-[var(--text-primary)] transition-all duration-200"
        >
          <Sun className="w-5 h-5 flex-shrink-0" />
          {!collapsed && <span className="text-sm font-medium">Theme</span>}
        </button>
      </div>
    </div>
  )
}

export default function SidebarWrapper() {
  return (
    <div className="flex-shrink-0 flex flex-col h-screen z-40 transition-all duration-300 bg-[var(--obsidian-raised)] border-r border-[var(--obsidian-border)]" style={{ width: "288px" }}>
      <div>Sidebar placeholder</div>
    </div>
  )
}
