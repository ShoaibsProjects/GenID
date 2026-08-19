"use client"
import { useEffect, useState } from "react"
import { isLoggedIn } from "@/lib/api"
import "@/styles/globals.css"
import { Sidebar } from "@/components/layout/sidebar"
import { Header } from "@/components/layout/header"

export default function RootLayout({ children }: { children: React.ReactNode }) {
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [isAuthPage, setIsAuthPage] = useState(true)

  useEffect(() => {
    const path = window.location.pathname
    setIsAuthPage(path === "/login" || path.startsWith("/login"))
  }, [])

  if (isAuthPage) {
    return (
      <html lang="en" className="dark">
        <body className="min-h-screen">
          {children}
        </body>
      </html>
    )
  }

  return (
    <html lang="en" className="dark">
      <body className="min-h-screen">
        <AuthBootstrap />
        <div className="flex h-screen overflow-hidden">
          <Sidebar isOpen={sidebarOpen} onClose={() => setSidebarOpen(false)} />
          <div className="flex-1 flex flex-col overflow-hidden">
            <Header sidebarOpen={sidebarOpen} setSidebarOpen={setSidebarOpen} />
            <main className="flex-1 overflow-y-auto relative z-10 pt-16">
              <div className="px-6 py-6 max-w-[1640px] mx-auto animate-fade-in">
                {children}
              </div>
            </main>
          </div>
        </div>
      </body>
    </html>
  )
}

function AuthBootstrap() {
  useEffect(() => {
    if (typeof window === "undefined") return
    const path = window.location.pathname
    // Skip the login page itself.
    if (path === "/login" || path.startsWith("/login")) return
    if (!isLoggedIn()) {
      window.location.replace("/login")
    }
  }, [])
  return null
}