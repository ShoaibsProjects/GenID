# ObserveID Architecture — Complete Figma Design Brief
## Version 1.0 | July 2026

---

## 1. THE STRATEGIC VISION

**What is ObserveID?**
An open-source Identity Governance and Administration (IGA) platform — the identity fabric for the AI era. It unifies humans, AI agents, service accounts, IoT devices, and RPA bots under a single graph-based policy engine, evaluated in milliseconds, orchestrated by durable workflows.

**Why is this different?**
- Neo4j graph relationships are first-class citizens (not SQL JOINs)
- Temporal durable workflows survive crashes and retry automatically
- Cedar policy-as-code is version-controlled and testable
- Every identity type (human, agent, bot, device) is governed equally

**Who is this image for?**
Engineers and CTOs evaluating identity platforms. They don't read — they scan. The image must communicate the product in 3 seconds, not 3 minutes.

**The 3-Second Rule:**
1. What is this? → A living identity organism (not a static platform)
2. Why should I care? → It governs humans + AI agents on the same graph
3. What's the magic? → Multi-tier access evaluation (the access beam moment)

---

## 2. AESTHETIC DIRECTION

**Style:** Studio Ghibli meets Tron: Legacy meets Blade Runner 2049
- Organic warmth (Ghibli) + Digital precision (Tron) + Cinematic atmosphere (Blade Runner)
- NOT cyberpunk neon-overload — restrained, purposeful glow
- NOT flat corporate — depth, atmosphere, life

**Mood:** Awe, wonder, trust. "This is what the future of identity looks like."

**Key principle:** The image is NOT a technical diagram with boxes and arrows. It is a living organism where identity relationships flow like synapses through a dark crystalline landscape.

---

## 3. COLOR SYSTEM

### Primary Palette
| Name | Hex | Usage |
|------|-----|-------|
| Obsidian Base | #0B0D11 | Background, deep space |
| Obsidian Light | #0F1118 | Secondary background, fog |
| Amber Identity | #F59E0B | Identity fabric, windows, accent |
| Amber Light | #FBBF24 | Particles, JWT tokens, glow |
| Amber Dark | #F38020 | Cloudflare Edge, shields |

### Component Colors
| Component | Primary | Glow | Text |
|-----------|---------|------|------|
| Cloudflare Edge | #F38020 | #FBBF24 | #FBC394 |
| Frontend | #3B82F6 | #60A5FA | #93C5FD |
| API Engine | #8B5CF6 | #A78BFA | #C4B5FD |
| Graph (Neo4j core) | #14B8A6 | #2DD4BF | #2DD4BF |
| Policy Engine | #EC4899 | #F472B6 | #F472B6 |
| AI Copilot | #8B5CF6 | #A78BFA | #C4B5FD |
| Connectors | #3B82F6 | #60A5FA | #93C5FD |
| Vault | #EF4444 | #F87171 | #FCA5A5 |
| Postgres | #3B82F6 | #60A5FA | #93C5FD |
| Neo4j | #10B981 | #34D399 | #6EE7B7 |
| Redis | #EF4444 | #F87171 | #FCA5A5 |
| Temporal | #EC4899 | #F472B6 | #FDA4AF |
| Workflow/Event | #EC4899 | #F472B6 | #FDA4AF |
| Observability | #EAB308 | #FDE047 | #FDE047 |

### Gradient Definitions
```
Background: linear-gradient(135deg, #0B0D11 0%, #0F1118 50%, #0B0D11 100%)
Amber Glow: linear-gradient(135deg, #F59E0B 0%, #FBBF24 100%)
Fog: radial-gradient(ellipse, #F59E0B 0% opacity 15%, transparent 100%)
```

---

## 4. TYPOGRAPHY

### Display Font
**Primary:** Plus Jakarta Sans, -apple-system, sans-serif
- Weight: 700-800 (ExtraBold)
- Usage: Title "OBSERVEID", section headers
- Size: 42px for title, 13-14px for component labels
- Letter spacing: -1px for title, 0-0.5px for headers

### Monospace (Technical)
**Primary:** JetBrains Mono, Fira Code, SF Mono, monospace
- Weight: 400-600
- Usage: All technical text, component descriptions, labels
- Size: 9-12px
- Letter spacing: 0-1px

### Text Hierarchy
```
Title:          Plus Jakarta Sans, 42px, weight 800, #F59E0B
Subtitle:       JetBrains Mono, 16px, weight 400, #717D96, spacing 2px
Component Name: JetBrains Mono, 12-13px, weight 600, [component color]
Component Desc: JetBrains Mono, 9-10px, weight 400, #9CA3AF
Technical:      JetBrains Mono, 8-9px, weight 400, #52525B
Legend:         JetBrains Mono, 10-11px, weight 400-600, #9CA3AF / #717D96
```

---

## 5. LAYOUT & COMPOSITION

### Canvas Size
- **Width:** 1920px (16:9 landscape)
- **Height:** 1080px

### Vertical Layering (Top to Bottom)
```
Layer 1: EDGE CANOPY (y: 72-170)
  └── Cloudflare Edge ring + light rain

Layer 2: APPLICATION (y: 224-450)
  ├── Frontend Cathedral (left, x: 380-620)
  ├── API Engine Monolith (right, x: 1300-1540)
  └── Arrow: Edge → ALB → Frontend + API

Layer 3: CORE SERVICES (y: 458-620)
  ├── Graph Pod (center, x: 908-1012) ← LARGEST, focal point
  ├── Policy Pod (left, x: 630-730)
  ├── AI Copilot Pod (right, x: 1190-1290)
  ├── Connectors Pod (back-left, x: 755-845)
  └── Vault Pod (back-right, x: 1075-1165)

Layer 4: DATA FOUNDATION (y: 740-860)
  ├── Postgres (left, x: 310-450)
  ├── Neo4j (center-left, x: 610-750)
  ├── Redis (center-right, x: 910-1050)
  └── Temporal (right, x: 1210-1350)

Layer 5: OBSERVABILITY (y: 880-960)
  └── Legend bar
```

### Horizontal Flow
```
Left (x: 180)     → Engineer character (foreground)
Left-Center (x: 500) → Frontend Cathedral
Center (x: 960)     → Graph Pod (focal point)
Right-Center (x: 1420) → API Engine
Right (x: 1600)    → Legend text
```

### Composition Principles
- **Diagonal energy flow:** Engineer (bottom-left) → Graph (center) → Edge (top)
- **Asymmetry:** Frontend (left) ≠ API Engine (right) — intentional visual tension
- **Depth layers:** Fog between strata creates atmospheric perspective
- **Negative space:** Top-right area is deliberately empty (breathing room)

---

## 6. COMPONENT DESIGN SPECS

### Engineer Character (Bottom-Left Foreground)
```
Position: x=180, y=780 (translated)
Size: ~110w x 200h

Body:
  - Silhouette path: obsidian black (#0B0D11) with subtle stroke (#1F2937)
  - Glowing circuit patterns: amber (#F59E0B) paths with glow filter
  - 3 circuit lines: y=140, y=160, y=140 (chest area)

Head:
  - Ellipse: rx=18, ry=22, skin tone (#F5D5B0)
  - Silver hair: path with curves, color #C0C0C0
  - Amber tech-visor: ellipse rx=14, ry=6, gradient amber, strong glow

Hand (raised, channeling energy):
  - Path extending from body
  - Energy circle: r=8, #F59E0B, strong glow
  - Outer glow: r=15, #FBBF24, opacity 0.4

Platform:
  - Rectangle: 150w x 20h, rx=4, fill #1C1E26, stroke #3B82F6
```

### Cloudflare Edge Ring (Top Center)
```
Position: x=960, y=130

Ring:
  - Ellipse: rx=250, ry=50
  - Stroke: #F38020, 2px, dasharray 15,10
  - Filter: glow (stdDeviation 6)
  - 6 obsidian shards at 60° intervals

Center Portal:
  - Circle: r=20, fill #F38020, strong glow

Light Rain (5 lines):
  - x offsets: -30, 0, 30
  - Path: quadratic curve from y=-40 to y=60
  - Stroke: #FBBF24, 2px, opacity 0.5, glow filter
```

### Frontend Cathedral (Left Middle)
```
Position: x=500, y=350

Main Structure:
  - Rectangle: 240w x 120h, rx=16 (soft Ghibli corners)
  - Fill: #1C1E26, Stroke: #3B82F6 2px
  - Opacity: 0.8

15 Amber Windows (3 rows x 5 columns):
  - Row 1: 12 windows, x from -105 to 93, y=-45
  - Row 2: 3 windows, x from -105 to -69, y=-27
  - Each: 14w x 14h, rx=4, fill #F59E0B, opacity 0.8, glow filter

Label:
  - Rectangle: 160w x 24h, rx=6, fill #1C1E26, stroke #3B82F6
  - Text: "FRONTEND", monospace 12px, weight 600, #93C5FD
```

### API Engine Monolith (Right Middle)
```
Position: x=1420, y=350

Main Structure:
  - Rectangle: 240w x 140h, rx=12
  - Fill: #1C1E26, Stroke: #8B5CF6 2px, glow filter

6 Middleware Streams (vertical bars):
  - x positions: -80, -50, -20, 10, 40, 70
  - Each: 6w x 120h, rx=3
  - Colors: purple, blue, teal, pink, green, red
  - Opacity: 0.7

Label:
  - "API ENGINE", monospace 12px, #C4B5FD
```

### Graph Pod (CENTER — Largest, Focal Point)
```
Position: x=960, y=480

Icosahedron:
  - Points: "0,-50 43,-25 43,25 0,50 -43,25 -43,-25"
  - Fill: none, Stroke: #14B8A6 2px
  - Filter: glowStrong (stdDeviation 12)
  - Opacity: 0.9

3 Internal Nodes:
  - Positions: (-15,-15), (15,-15), (0,20)
  - Radius: 6, Fill: #2DD4BF, glowStrong filter

3 Connection Lines:
  - Between each node pair
  - Stroke: #14B8A6 2px, opacity 0.5

Label:
  - "GRAPH", monospace 12px, #2DD4BF, weight 600
```

### Policy Pod (Left of Graph)
```
Position: x=680, y=520

Structure:
  - Rectangle: 100w x 70h, rx=8
  - Fill: #1C1E26, Stroke: #EC4899 2px, glow

Text:
  - "permit" at (-30, -10), monospace 12px, #F472B6
  - "forbid" at (-30, 12), monospace 12px, #F87171

Label:
  - "POLICY", #F472B6
```

### AI Copilot Pod (Right of Graph)
```
Position: x=1240, y=520

Orbital Ring:
  - Ellipse: rx=50, ry=35, stroke #A78BFA 2px, glow

6 Orbiting Spheres:
  - Radius: 5, Fill: #A78BFA, glowStrong
  - Positions: (0,-22), (28,-12), (28,12), (0,22), (-28,12), (-28,-12)

Label:
  - "AI COPILOT", #C4B5FD
```

### Connectors Pod (Back-Left)
```
Position: x=800, y=600

Ring:
  - Circle: r=45, stroke #60A5FA 2px, dasharray 6,4, glow

5 Platform Circles:
  - Radius: 8
  - Colors: #3B82F6 (Entra), #10B981 (LDAP), #F59E0B (SCIM), #9CA3AF (Generic), #EC4899 (CSV)

Label:
  - "CONNECTORS", #93C5FD
```

### Vault Pod (Back-Right)
```
Position: x=1120, y=600

Structure:
  - Rectangle: 90w x 60h, rx=8
  - Fill: #1C1E26, Stroke: #EF4444 2px, glow

Lock Ring:
  - Circle: r=18, stroke #EF4444 2px, dasharray 3,3, glow

Label:
  - "VAULT", #FCA5A5
```

### Data Layer Cards (Bottom Row)
Each card follows this pattern:
```
Structure:
  - Rectangle: 140w x 80h, rx=10
  - Fill: #1C1E26, Stroke: [color] 2px, glow

Internal Elements:
  - Postgres: 6 table rectangles (35w x 25h, rx=4)
  - Neo4j: 3 nodes + 3 connection lines
  - Redis: 3 nodes + sine wave path
  - Temporal: outer circle r=60 + inner dashed circle r=42 + 6 gear circles r=10

Label:
  - Rectangle: 140w x 24h, rx=6
  - [Component Name], monospace 12px, weight 600, [component color]
```

---

## 7. PARTICLE EFFECTS

### JWT Token Fireflies (Golden)
```
10 floating circles:
  - Radius: 2-3px
  - Fill: #FBBF24
  - Filter: glow (stdDeviation 6)
  - Animation: vertical oscillation, 3-5s duration

Positions:
  (300,300), (450,420), (600,350), (800,480),
  (1100,400), (1350,520), (1500,380), (700,600),
  (950,650), (1200,580)
```

### Policy Runes (Glowing Symbols)
```
3 floating text elements:
  - "☉" at (550,480), #F472B6, size 16
  - "✦" at (1150,450), #F87171, size 16
  - "⚡" at (850,550), #A78BFA, size 14

All with glow filter, opacity 0.6, pulsing animation
```

### Access Beam (The Money Shot)
```
Path from Engineer to Graph:
  - d="M 300 860 Q 500 700 700 550 Q 850 480 960 480"
  - Stroke: #F59E0B, 4px, dasharray 10,6
  - Opacity: 0.8
  - Filter: glowStrong

Animated orb:
  - Circle r=6, #FBBF24, glowStrong
  - Animates along the path continuously
```

---

## 8. FILTERS & EFFECTS

### Glow (Standard)
```xml
<filter id="glow">
  <feGaussianBlur stdDeviation="6" result="b"/>
  <feMerge>
    <feMergeNode in="b"/>
    <feMergeNode in="SourceGraphic"/>
  </feMerge>
</filter>
```

### Glow Strong (For focal points)
```xml
<filter id="glowStrong">
  <feGaussianBlur stdDeviation="12" result="b"/>
  <feMerge>
    <feMergeNode in="b"/>
    <feMergeNode in="SourceGraphic"/>
  </feMerge>
</filter>
```

### Drop Shadow
```xml
<filter id="shadow">
  <feDropShadow dx="0" dy="4" stdDeviation="8" flood-opacity="0.5"/>
</filter>
```

### Volumetric Fog
```
Elliptical gradients:
  - Ellipse 1: cx=600, cy=400, rx=400, ry=200, opacity 0.4
  - Ellipse 2: cx=1200, cy=500, rx=500, ry=250, opacity 0.3
  - Fill: radial-gradient(#F59E0B 0% opacity 15%, transparent 100%)
```

---

## 9. CONNECTION LINES

### Vertical Connections (Data Flow)
```
Frontend → Data Layer:
  - Path: M 380 350 L 380 740
  - Stroke: #3B82F6, 2px, opacity 0.4, dasharray 4,4

API → Data Layer:
  - Path: M 1420 350 L 1420 740
  - Stroke: #8B5CF6, 2px, opacity 0.4, dasharray 4,4
```

### Diagonal Connections
```
Graph → Neo4j:
  - Path: M 500 480 L 680 740
  - Stroke: #14B8A6, 2px, opacity 0.3

API → Temporal:
  - Path: M 1420 480 L 1280 740
  - Stroke: #10B981, 2px, opacity 0.3
```

---

## 10. TITLE & LEGEND

### Title (Top-Left)
```
Position: x=80, y=50

"OBSERVEID"
  - Font: Plus Jakarta Sans, 42px, weight 800
  - Fill: #F59E0B
  - Filter: glow
  - Letter spacing: -1px

"IDENTITY FABRIC FOR THE AI ERA"
  - Font: JetBrains Mono, 14px, weight 400
  - Fill: #717D96
  - Letter spacing: 2px

Accent Line:
  - Rectangle: 300w x 2h, fill #F59E0B, glow filter
  - Position: y=85
```

### Legend Bar (Bottom)
```
Position: x=80, y=950

Container:
  - Rectangle: 1760w x 80h, rx=12
  - Fill: #16171E, Stroke: #27272A 1px

Title:
  - "ARCHITECTURE · 2026 EDITION"
  - JetBrains Mono, 11px, weight 600, #717D96

Color Dots + Labels (spaced evenly):
  - 6px circles with component colors
  - Labels: JetBrains Mono, 10px, #9CA3AF
  - Spacing: ~150px between each

Brand:
  - "ObserveID Reimagined" right-aligned
  - JetBrains Mono, 10px, #52525B
```

---

## 11. FIGMA SETUP INSTRUCTIONS

### Frame Setup
1. Create frame: 1920 x 1080
2. Fill: #0B0D11
3. Name: "ObserveID Architecture — Anime v2"

### Layer Organization
```
📁 Title
  ├── Text "OBSERVEID"
  ├── Text "IDENTITY FABRIC..."
  └── Accent line

📁 Edge Canopy
  ├── Ring ellipse
  ├── 6 Shards (group, rotate each 60°)
  ├── Center portal
  └── Light rain (5 paths)

📁 Application Layer
  ├── Frontend Cathedral
  │   ├── Main rect
  │   ├── 15 window rects (group)
  │   └── Label
  └── API Engine
      ├── Main rect
      ├── 6 middleware streams
      └── Label

📁 Core Services
  ├── Graph Pod (center, largest)
  ├── Policy Pod
  ├── AI Copilot Pod
  ├── Connectors Pod
  └── Vault Pod

📁 Data Foundation
  ├── Postgres card
  ├── Neo4j card
  ├── Redis card
  └── Temporal card

📁 Engineer Character
  ├── Body silhouette
  ├── Circuit patterns
  ├── Head + hair
  ├── Visor
  ├── Hand + energy
  └── Platform

📁 Particles
  ├── 10 JWT fireflies
  └── 3 Policy runes

📁 Effects
  ├── Access beam path
  ├── Volumetric fog (2 ellipses)
  └── Connection lines

📁 Legend
  ├── Container rect
  ├── 6 color dots + labels
  └── Brand text
```

### Figma Effects to Apply
1. **Layer Blur:** For fog elements (Gaussian, 20-40px)
2. **Drop Shadow:** For all cards (0, 4, 8, #000000 50%)
3. **Outer Glow:** For nodes and accents (6-12px blur, component color)
4. **Inner Glow:** For the access beam (subtle amber)

### Figma Plugins Recommended
- **Remove Background** — if importing any raster elements
- **Auto Layout** — for legend items
- **Tokens Studio** — to manage the color system as design tokens

---

## 12. ACCESSIBILITY NOTES

- All text meets WCAG AA contrast against dark backgrounds
- Component colors are distinguishable (not relying on color alone — shapes differ)
- Legend provides clear color-to-component mapping
- No animated elements are essential to understanding (static version is complete)

---

## 13. EXPORT SPECIFICATIONS

### For GitHub README
- Format: PNG
- Size: 1920 x 1080 (or 2x for retina: 3840 x 2160)
- File: `media/architecture-anime.png`

### For Presentation/Social
- Format: PNG with transparent background option
- Size: 1920 x 1080

### For Web
- Format: SVG (vector, infinite zoom)
- Optimize: Remove unnecessary groups, flatten transforms

---

## 14. WHAT MAKES THIS UNFORGETTABLE

1. **The Engineer Character** — Human anchor, gives scale, makes it relatable
2. **The Access Beam** — Shows the product's core magic in one visual line
3. **The Graph as Focal Point** — Neo4j is the differentiator, it's the largest element
4. **Ghibli × Tron Aesthetic** — Warm + futuristic = trustworthy innovation
5. **7 Labels Max** — Respects the 3-second rule
6. **Particle Life** — Fireflies and runes make it feel alive, not static
7. **Vertical Stratification** — Edge → App → Core → Data tells the architectural story
8. **Cinematic Scale** — Makes the viewer feel small, like looking at a living city

---

*Design Brief v1.0 — ObserveID Reimagined, July 2026*
*Built with obsessive attention to detail*
