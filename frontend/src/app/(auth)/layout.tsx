"use client"
import { useEffect } from "react"
import { isLoggedIn } from "@/lib/api"

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    if (typeof window === "undefined") return
    if (isLoggedIn()) {
      window.location.replace("/dashboard")
    }
  }, [])
  return <>{children}</>
}