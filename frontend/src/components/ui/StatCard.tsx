"use client"

import { Card, CardBody } from "./Card"

interface StatCardProps {
  label: string
  value: string | number
  trend?: { value: string; positive: boolean }
  icon?: React.ReactNode
  onClick?: () => void
}

export function StatCard({ label, value, trend, icon, onClick }: StatCardProps) {
  return (
    <Card hover={!!onClick} onClick={onClick}>
      <CardBody>
        <div className="flex items-start justify-between">
          <div className="space-y-1">
            <p className="text-[0.65rem] font-semibold text-muted uppercase tracking-wider font-mono">{label}</p>
            <p className="text-2xl font-bold tracking-tight font-mono">{value}</p>
            {trend && (
              <p className={`text-xs font-medium ${trend.positive ? "text-green-400" : "text-red-400"}`}>
                {trend.positive ? "↑" : "↓"} {trend.value}
              </p>
            )}
          </div>
          {icon && <div className="text-muted">{icon}</div>}
        </div>
      </CardBody>
    </Card>
  )
}
