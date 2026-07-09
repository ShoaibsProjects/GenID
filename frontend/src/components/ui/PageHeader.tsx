"use client"

interface PageHeaderProps {
  title: string
  description?: string
  children?: React.ReactNode
  actions?: React.ReactNode
}

export function PageHeader({ title, description, children, actions }: PageHeaderProps) {
  return (
    <div className="flex items-start justify-between">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
        {description && <p className="text-sm text-secondary mt-1">{description}</p>}
      </div>
      <div className="flex items-center gap-2">
        {actions}
        {children}
      </div>
    </div>
  )
}
