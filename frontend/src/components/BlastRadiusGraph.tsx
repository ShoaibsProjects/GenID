"use client"

// Canvas-based force graph for blast radius visualization.
// MUST be loaded via next/dynamic({ ssr: false }) — the library touches window.

import { useRef, useEffect, useCallback } from "react"
import ForceGraph2D from "react-force-graph-2d"

export interface GraphNode {
  id: string
  label: string
  type: string
  criticality?: string | null
  permission_level?: string | null
}

export interface GraphLink {
  source: string
  target: string
  type: string
}

const NODE_COLORS: Record<string, string> = {
  Identity: "#60A5FA",          // blue — humans
  NonHumanIdentity: "#C084FC",  // violet — agents/NHI
  Role: "#A78BFA",              // purple
  Entitlement: "#F59E0B",       // amber
  Resource: "#34D399",          // green (non-critical)
}

function nodeColor(n: GraphNode): string {
  if (n.type === "Resource" && (n.criticality === "critical" || n.criticality === "high")) {
    return "#EF4444" // red — critical resources
  }
  return NODE_COLORS[n.type] || "#9C9CA0"
}

export default function BlastRadiusGraph({
  nodes,
  links,
  centerId,
}: {
  nodes: GraphNode[]
  links: GraphLink[]
  centerId: string
}) {
  const fgRef = useRef<any>(null)

  useEffect(() => {
    const t = setTimeout(() => fgRef.current?.zoomToFit(600, 70), 500)
    return () => clearTimeout(t)
  }, [nodes, links])

  const paintNode = useCallback(
    (node: any, ctx: CanvasRenderingContext2D, globalScale: number) => {
      const n = node as GraphNode & { x: number; y: number }
      const isCenter = n.id === centerId
      const radius = isCenter ? 11 : 7
      const color = nodeColor(n)

      // Ambient glow
      ctx.beginPath()
      ctx.arc(n.x, n.y, radius + 7, 0, 2 * Math.PI)
      ctx.fillStyle = color + "22"
      ctx.fill()

      // Core
      ctx.beginPath()
      ctx.arc(n.x, n.y, radius, 0, 2 * Math.PI)
      ctx.fillStyle = color
      ctx.fill()
      ctx.strokeStyle = isCenter ? "#FFFFFF" : color + "AA"
      ctx.lineWidth = isCenter ? 2 : 1
      ctx.stroke()

      // Label
      const fontSize = Math.max(11 / globalScale, 3.5)
      ctx.font = `${isCenter ? "600" : "500"} ${fontSize}px 'JetBrains Mono', monospace`
      ctx.textAlign = "center"
      ctx.textBaseline = "top"
      ctx.fillStyle = isCenter ? "#F0EFEC" : "#C9C9CE"
      ctx.fillText(n.label || n.id, n.x, n.y + radius + 4)

      // Type tag
      const typeSize = Math.max(9 / globalScale, 2.8)
      ctx.font = `${typeSize}px 'JetBrains Mono', monospace`
      ctx.fillStyle = color + "CC"
      ctx.fillText(n.type, n.x, n.y + radius + 4 + fontSize + 1)
    },
    [centerId],
  )

  const paintLink = useCallback(
    (link: any, ctx: CanvasRenderingContext2D, globalScale: number) => {
      const { source, target, type } = link
      if (source.x == null || target.x == null) return

      // Edge line
      ctx.beginPath()
      ctx.moveTo(source.x, source.y)
      ctx.lineTo(target.x, target.y)
      ctx.strokeStyle = "rgba(255,255,255,0.14)"
      ctx.lineWidth = 1.2
      ctx.stroke()

      // Arrowhead
      const angle = Math.atan2(target.y - source.y, target.x - source.x)
      const tRadius = target.id === centerId ? 11 : 7
      const ax = target.x - Math.cos(angle) * (tRadius + 3)
      const ay = target.y - Math.sin(angle) * (tRadius + 3)
      ctx.beginPath()
      ctx.moveTo(ax, ay)
      ctx.lineTo(ax - 6 * Math.cos(angle - 0.42), ay - 6 * Math.sin(angle - 0.42))
      ctx.lineTo(ax - 6 * Math.cos(angle + 0.42), ay - 6 * Math.sin(angle + 0.42))
      ctx.closePath()
      ctx.fillStyle = "rgba(255,255,255,0.35)"
      ctx.fill()

      // Edge label (relationship type) — only when zoomed enough to read
      if (globalScale > 0.7) {
        const mx = (source.x + target.x) / 2
        const my = (source.y + target.y) / 2
        const fs = Math.max(9 / globalScale, 2.5)
        ctx.font = `${fs}px 'JetBrains Mono', monospace`
        ctx.textAlign = "center"
        ctx.textBaseline = "middle"
        ctx.fillStyle = "rgba(156,156,160,0.8)"
        ctx.fillText(type, mx, my)
      }
    },
    [centerId],
  )

  return (
    <ForceGraph2D
      ref={fgRef}
      graphData={{ nodes, links }}
      backgroundColor="#050508"
      nodeCanvasObject={paintNode}
      nodePointerAreaPaint={(node: any, color: string, ctx: CanvasRenderingContext2D) => {
        ctx.fillStyle = color
        ctx.beginPath()
        ctx.arc(node.x, node.y, 13, 0, 2 * Math.PI)
        ctx.fill()
      }}
      linkCanvasObject={paintLink}
      linkCanvasObjectMode={() => "replace"}
      nodeLabel={(n: any) =>
        `<div style="font-family:'JetBrains Mono',monospace;font-size:11px;line-height:1.5">
           <b style="color:${nodeColor(n)}">${n.type}</b> · ${n.label}<br/>
           <span style="color:#5C5C62">${n.id}</span>
           ${n.permission_level ? `<br/>perm: <b>${n.permission_level}</b>` : ""}
           ${n.criticality ? `<br/>criticality: <b>${n.criticality}</b>` : ""}
         </div>`
      }
      cooldownTicks={140}
      d3VelocityDecay={0.28}
      enableNodeDrag
    />
  )
}
