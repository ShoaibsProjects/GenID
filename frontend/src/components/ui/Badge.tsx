"use client"

import { cn } from "@/lib/utils"

type BadgeVariant = "success" | "warning" | "danger" | "info" | "neutral"

const colors: Record<BadgeVariant, string> = {
  success: "bg-green-500/10 text-green-400 border-green-500/25",
  warning: "bg-amber-500/10 text-amber-400 border-amber-500/25",
  danger: "bg-red-500/10 text-red-400 border-red-500/25",
  info: "bg-blue-500/10 text-blue-400 border-blue-500/25",
  neutral: "bg-white/[0.06] text-secondary border-white/[0.1]",
}

interface BadgeProps {
  variant?: BadgeVariant
  children: React.ReactNode
  className?: string
}

export function Badge({ variant = "neutral", children, className }: BadgeProps) {
  return (
    <span className={cn(
      "inline-flex items-center px-2 py-0.5 rounded text-[0.65rem] font-semibold uppercase tracking-wider font-mono border",
      colors[variant],
      className
    )}>
      {children}
    </span>
  )
}
