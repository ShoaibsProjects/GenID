"use client"

import { cn } from "@/lib/utils"

type CardVariant = "default" | "accent" | "error"

const borders: Record<CardVariant, string> = {
  default: "border-border",
  accent: "border-l-[3px] border-l-accent border-border",
  error: "border-l-[3px] border-l-red-500 border-border",
}

interface CardProps {
  variant?: CardVariant
  className?: string
  children: React.ReactNode
  onClick?: () => void
  hover?: boolean
}

export function Card({ variant = "default", className, children, onClick, hover }: CardProps) {
  return (
    <div
      className={cn(
        "bg-surface-raised border rounded",
        borders[variant],
        hover && "transition-colors duration-150 hover:bg-white/[0.02] cursor-pointer",
        className
      )}
      onClick={onClick}
    >
      {children}
    </div>
  )
}

export function CardHeader({ className, children }: { className?: string; children: React.ReactNode }) {
  return <div className={cn("px-5 py-4 border-b border-border", className)}>{children}</div>
}

export function CardBody({ className, children }: { className?: string; children: React.ReactNode }) {
  return <div className={cn("p-5", className)}>{children}</div>
}

export function CardFooter({ className, children }: { className?: string; children: React.ReactNode }) {
  return <div className={cn("px-5 py-3 border-t border-border bg-white/[0.01]", className)}>{children}</div>
}
