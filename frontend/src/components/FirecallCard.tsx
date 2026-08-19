"use client"
import { useState } from "react"

export function FirecallCard() {
  return (
    <div style={{ padding: 24, background: '#1a1a2e', borderRadius: 16, border: '1px solid rgba(255,255,255,0.1)' }}>
      <h3 style={{ color: '#F0EFEC', marginBottom: 8 }}>Emergency Firecall Access</h3>
      <p style={{ color: '#9C9CA0' }}>Break-glass emergency access for critical incidents</p>
      <div style={{ marginTop: 16 }}>
        <span style={{ 
          display: 'inline-flex', 
          alignItems: 'center', 
          padding: '4px 12px', 
          borderRadius: 9999, 
          fontSize: '0.65rem', 
          fontWeight: 600, 
          textTransform: 'uppercase', 
          letterSpacing: '0.08em',
          background: 'rgba(239,68,68,0.1)',
          color: '#EF4444',
          border: '1px solid rgba(239,68,68,0.25)'
        }}>
          INACTIVE
        </span>
      </div>
    </div>
  )
}