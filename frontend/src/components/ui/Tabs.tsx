"use client"

import { cn } from "@/lib/utils"

interface Tab {
  id: string
  label: string
  count?: number
}

interface TabsProps {
  tabs: Tab[]
  active: string
  onChange: (id: string) => void
  className?: string
}

export function Tabs({ tabs, active, onChange, className }: TabsProps) {
  return (
    <div className={cn("inline-flex bg-white/[0.03] border border-border rounded p-0.5", className)}>
      {tabs.map((tab) => (
        <button
          key={tab.id}
          onClick={() => onChange(tab.id)}
          className={cn(
            "px-3 py-1.5 rounded-sm text-xs font-semibold transition-all duration-150",
            active === tab.id
              ? "bg-white/[0.08] text-primary"
              : "text-secondary hover:text-primary"
          )}
        >
          {tab.label}
          {tab.count !== undefined && (
            <span className={cn(
              "ml-1.5 px-1.5 py-0.5 rounded text-[0.6rem] font-mono",
              active === tab.id ? "bg-white/[0.1]" : "bg-white/[0.04]"
            )}>
              {tab.count}
            </span>
          )}
        </button>
      ))}
    </div>
  )
}
