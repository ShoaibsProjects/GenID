"use client"

import { Button } from "./Button"

interface EmptyStateProps {
  title: string
  description?: string
  action?: { label: string; onClick: () => void }
  icon?: React.ReactNode
}

export function EmptyState({ title, description, action, icon }: EmptyStateProps) {
  return (
    <div className="py-16 text-center">
      {icon && <div className="mb-4 inline-flex text-muted">{icon}</div>}
      <p className="text-sm font-semibold text-secondary mb-1">{title}</p>
      {description && <p className="text-xs text-muted max-w-sm mx-auto mb-4">{description}</p>}
      {action && (
        <Button variant="primary" size="sm" onClick={action.onClick}>
          {action.label}
        </Button>
      )}
    </div>
  )
}
