"use client"
import { useEffect, useRef, useState } from "react"
import type { PlatformEvent } from "@/lib/api"

// useEventStream subscribes to the SSE real-time endpoint and returns the
// rolling window (most recent N events). Graceful degradation: if the
// backend does not expose /api/v1/events/stream (or it errors), the hook
// falls back to periodic polling of the REST /api/v1/events endpoint so
// pages still show fresh data.
export function useEventStream(opts?: { limit?: number; pollMs?: number }) {
  const { limit = 50, pollMs = 15000 } = opts || {}
  const [events, setEvents] = useState<PlatformEvent[]>([])
  const [live, setLive] = useState(false)
  const esRef = useRef<EventSource | null>(null)

  const merge = (next: PlatformEvent) =>
    setEvents((prev) => {
      const deduped = [next, ...prev].filter(
        (e, i, arr) => arr.findIndex((x) => x.id && x.id === e.id) === i,
      )
      return deduped.slice(0, limit)
    })

  useEffect(() => {
    let es: EventSource | null = null
    let pollId: ReturnType<typeof setInterval> | undefined
    let disposed = false

    const startPolling = () => {
      const load = async () => {
        try {
          const res = await fetch("/api/v1/events?limit=50", { headers: { Accept: "application/json" } })
          if (!res.ok) return
          const data = await res.json()
          if (disposed || !data.events?.length) return
          const latest = data.events as PlatformEvent[]
          setEvents((prev) => {
            const known = new Set(prev.map((e) => e.id).filter(Boolean))
            const fresh = latest.filter((e) => e.id && !known.has(e.id))
            return [...fresh, ...prev].slice(0, limit)
          })
        } catch {
          /* backend offline — stay quiet */
        }
      }
      load()
      pollId = setInterval(load, pollMs)
    }

    if (typeof window !== "undefined" && typeof EventSource !== "undefined") {
      try {
        es = new EventSource("/api/v1/events/stream")
        esRef.current = es
        es.onopen = () => setLive(true)
        es.onmessage = (e) => {
          try {
            const ev = JSON.parse(e.data)
            if (ev?.type) merge(ev)
          } catch { /* ignore malformed frames */ }
        }
        es.onerror = () => {
          setLive(false)
          es?.close()
          esRef.current = null
          if (!disposed) startPolling()
        }
      } catch {
        startPolling()
      }
    } else {
      startPolling()
    }

    return () => {
      disposed = true
      es?.close()
      if (pollId) clearInterval(pollId)
      esRef.current = null
    }
    // Mount-only subscription. Re-running on opts change would recreate the
    // EventSource; pages use stable opts.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return { events, live }
}
