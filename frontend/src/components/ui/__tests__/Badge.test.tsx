import "@testing-library/jest-dom/jest-globals"
import { render, screen } from "@testing-library/react"
import { Badge } from "../Badge"

describe("Badge", () => {
  it("renders children text", () => {
    render(<Badge>Active</Badge>)
    expect(screen.getByText("Active")).toBeInTheDocument()
  })

  it("applies neutral variant by default", () => {
    render(<Badge>Test</Badge>)
    const badge = screen.getByText("Test")
    expect(badge.className).toContain("bg-white/[0.06]")
  })

  it("applies success variant", () => {
    render(<Badge variant="success">Success</Badge>)
    expect(screen.getByText("Success").className).toContain("bg-green-500/10")
  })

  it("applies warning variant", () => {
    render(<Badge variant="warning">Warning</Badge>)
    expect(screen.getByText("Warning").className).toContain("bg-amber-500/10")
  })

  it("applies danger variant", () => {
    render(<Badge variant="danger">Danger</Badge>)
    expect(screen.getByText("Danger").className).toContain("bg-red-500/10")
  })

  it("applies info variant", () => {
    render(<Badge variant="info">Info</Badge>)
    expect(screen.getByText("Info").className).toContain("bg-blue-500/10")
  })

  it("applies custom className", () => {
    render(<Badge className="custom">Test</Badge>)
    expect(screen.getByText("Test").className).toContain("custom")
  })

  it("renders as uppercase with mono font", () => {
    render(<Badge>Test</Badge>)
    const badge = screen.getByText("Test")
    expect(badge.className).toContain("uppercase")
    expect(badge.className).toContain("font-mono")
  })
})
