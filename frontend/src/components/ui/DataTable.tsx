"use client"

import { cn } from "@/lib/utils"

interface Column<T> {
  key: string
  header: string
  render: (row: T) => React.ReactNode
  align?: "left" | "right"
  width?: string
}

interface DataTableProps<T> {
  columns: Column<T>[]
  data: T[]
  keyField: string
  onRowClick?: (row: T) => void
  emptyMessage?: string
  loading?: boolean
}

export function DataTable<T extends Record<string, any>>({
  columns,
  data,
  keyField,
  onRowClick,
  emptyMessage = "No data",
  loading,
}: DataTableProps<T>) {
  if (loading) {
    return (
      <div className="p-12 text-center text-sm text-secondary">
        <div className="animate-pulse space-y-3">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="h-10 bg-white/[0.03] rounded" />
          ))}
        </div>
      </div>
    )
  }

  if (data.length === 0) {
    return (
      <div className="p-12 text-center">
        <p className="text-sm text-secondary">{emptyMessage}</p>
      </div>
    )
  }

  return (
    <table className="w-full border-collapse">
      <thead>
        <tr className="border-b border-border">
          {columns.map((col) => (
            <th
              key={col.key}
              className={cn(
                "px-4 py-3 text-left text-[0.65rem] font-semibold text-muted uppercase tracking-wider font-mono",
                col.align === "right" && "text-right"
              )}
              style={col.width ? { width: col.width } : undefined}
            >
              {col.header}
            </th>
          ))}
        </tr>
      </thead>
      <tbody className="divide-y divide-border/50">
        {data.map((row) => (
          <tr
            key={row[keyField]}
            className={cn(
              "transition-colors duration-100",
              onRowClick && "cursor-pointer hover:bg-white/[0.02]"
            )}
            onClick={() => onRowClick?.(row)}
          >
            {columns.map((col) => (
              <td
                key={col.key}
                className={cn(
                  "px-4 py-3 text-sm",
                  col.align === "right" && "text-right"
                )}
              >
                {col.render(row)}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}
