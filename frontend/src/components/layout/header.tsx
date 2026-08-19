"use client"
import { useState, useEffect } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/Button"
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet"
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
  Menu,
  ChevronRight,
  Sun,
  Moon,
  Bell,
  Search,
  User,
  LogOut,
} from "lucide-react"

export function Header({ sidebarOpen, setSidebarOpen }: { sidebarOpen: boolean; setSidebarOpen: (open: boolean) => void }) {
  const pathname = usePathname()
  const [theme, setTheme] = useState<"dark" | "light">("dark")
  const [mounted, setMounted] = useState(false)
  const [notifications, setNotifications] = useState(3) // demo
  const [userMenuOpen, setUserMenuOpen] = useState(false)
  const [tenant, setTenant] = useState("default")
  const TENANTS = [
    { id: "default", label: "Default Tenant" },
    { id: "acme", label: "Acme Corp" },
    { id: "globex", label: "Globex" },
    { id: "umbrella", label: "Umbrella" },
  ]

  useEffect(() => {
    setMounted(true)
    const stored = localStorage.getItem("theme") as "dark" | "light" | null
    if (stored) {
      setTheme(stored)
      document.documentElement.classList.toggle("dark", stored === "dark")
    } else if (window.matchMedia("(prefers-color-scheme: dark)").matches) {
      setTheme("dark")
    }
    setTenant(localStorage.getItem("genid_tenant") || "default")
  }, [])

  const toggleTheme = () => {
    const newTheme = theme === "dark" ? "light" : "dark"
    setTheme(newTheme)
    localStorage.setItem("theme", newTheme)
    document.documentElement.classList.toggle("dark", newTheme === "dark")
  }

  const switchTenant = (id: string) => {
    setTenant(id)
    localStorage.setItem("genid_tenant", id)
    setUserMenuOpen(false)
    window.location.reload()
  }

  const logout = () => {
    localStorage.removeItem("genid_access_token")
    localStorage.removeItem("genid_user_email")
    window.location.replace("/login")
  }

  if (!mounted) return <header className="h-16" />

  return (
    <header className="fixed top-0 left-[288px] right-0 z-50 h-16 bg-[var(--obsidian-raised)] border-b border-[var(--obsidian-border)] flex items-center px-4 lg:px-6">
      <div className="flex items-center justify-between w-full max-w-screen-2xl mx-auto">
        {/* Left: Mobile menu + Search */}
        <div className="flex items-center gap-3 flex-1 lg:flex-none w-64">
          <Button variant="ghost" size="icon" className="lg:hidden" onClick={() => setSidebarOpen(true)}>
            <Menu className="h-5 w-5" />
          </Button>
          
          <div className="relative w-full max-w-xs lg:max-w-md">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-muted)]" />
            <input
              type="search"
              placeholder="Search identities, resources, policies..."
              className="w-full pl-10 pr-4 py-2 text-sm rounded-lg bg-[var(--glass-2)] border border-[var(--obsidian-border)] text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:outline-none focus:border-[var(--accent)] focus:ring-1 focus:ring-[var(--accent)] transition-all duration-200"
            />
          </div>
        </div>

        {/* Right: Notifications, Theme, User */}
        <div className="flex items-center gap-2 lg:gap-3">
          {/* Notifications */}
          <div className="relative">
            <Button variant="ghost" size="icon" className="relative">
              <Bell className="h-5 w-5" />
              {notifications > 0 && (
                <span className="absolute -top-1 -right-1 w-4 h-4 bg-red-500 text-white text-[10px] font-bold rounded-full flex items-center justify-center">
                  {notifications > 9 ? "9+" : notifications}
                </span>
              )}
            </Button>
          </div>

          {/* Theme Toggle */}
          <Button variant="ghost" size="icon" onClick={toggleTheme}>
            {theme === "dark" ? (
              <Sun className="h-5 w-5" />
            ) : (
              <Moon className="h-5 w-5" />
            )}
          </Button>

          {/* User Menu */}
          <div className="relative">
            <button
              type="button"
              onClick={() => setUserMenuOpen((v) => !v)}
              className="flex items-center gap-2 px-3 py-1.5 hover:bg-[var(--glass-2)] rounded-xl transition-colors"
            >
              <div className="w-8 h-8 rounded-full flex items-center justify-center" style={{ background: 'linear-gradient(135deg, #F59E0B, #D97706)' }}>
                <User className="w-4 h-4 text-white" />
              </div>
              <span className="hidden sm:block font-medium text-sm text-[var(--text-primary)]">Admin</span>
            </button>

            {userMenuOpen && (
              <>
                <div className="fixed inset-0 z-40" onClick={() => setUserMenuOpen(false)} />
                <div className="absolute right-0 mt-2 w-64 z-50 rounded-xl border border-[var(--obsidian-border)] bg-[var(--obsidian-raised)] shadow-2xl p-2 animate-fade-in">
                  <div className="px-3 py-2 border-b border-[var(--obsidian-border)]">
                    <p className="text-sm font-semibold text-[var(--text-primary)]">admin@genid.io</p>
                    <p className="text-xs text-[var(--text-muted)]">Administrator</p>
                  </div>
                  <div className="py-2">
                    <p className="px-3 pb-1 text-[0.65rem] font-semibold uppercase tracking-wider text-[var(--text-muted)]">Tenant</p>
                    {TENANTS.map((t) => (
                      <button
                        key={t.id}
                        type="button"
                        onClick={() => switchTenant(t.id)}
                        className={`w-full text-left px-3 py-1.5 text-sm rounded-lg transition-colors ${
                          tenant === t.id
                            ? "bg-[var(--accent-dim)] text-[var(--accent)]"
                            : "text-[var(--text-secondary)] hover:bg-[var(--glass-2)]"
                        }`}
                      >
                        {t.label}
                        {tenant === t.id && <span className="float-right text-[var(--accent)]">•</span>}
                      </button>
                    ))}
                  </div>
                  <button
                    type="button"
                    onClick={logout}
                    className="w-full flex items-center gap-2 px-3 py-2 text-sm text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
                  >
                    <LogOut className="w-4 h-4" />
                    Sign out
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      </div>
    </header>
  )
}