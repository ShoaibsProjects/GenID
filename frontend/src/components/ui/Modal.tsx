"use client"

import { Card, CardHeader, CardBody, CardFooter } from "./Card"
import { Button } from "./Button"

interface ModalProps {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  children: React.ReactNode
  footer?: React.ReactNode
  size?: "sm" | "md" | "lg"
}

const widths: Record<string, string> = {
  sm: "max-w-sm",
  md: "max-w-lg",
  lg: "max-w-2xl",
}

export function Modal({ open, onClose, title, description, children, footer, size = "md" }: ModalProps) {
  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[10vh]">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      {/* Content */}
      <div className={`relative w-full ${widths[size]} mx-4`}>
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-lg font-bold">{title}</h2>
                {description && <p className="text-sm text-secondary mt-0.5">{description}</p>}
              </div>
              <button
                onClick={onClose}
                className="w-7 h-7 flex items-center justify-center rounded text-secondary hover:text-primary hover:bg-white/[0.06] transition-colors"
              >
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18 6L6 18M6 6l12 12"/></svg>
              </button>
            </div>
          </CardHeader>
          <CardBody>{children}</CardBody>
          {footer && <CardFooter>{footer}</CardFooter>}
        </Card>
      </div>
    </div>
  )
}
