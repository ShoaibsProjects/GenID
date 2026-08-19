import { render, screen } from "@testing-library/react"
import { Badge } from "../Badge"

describe("Badge", () => {
  it("renders children text", () => {
    render(<Badge>Active</Badge>)
    expect(screen.getByText("Active")).toBeInTheDocument()
  })

  it("applies default variant by default", () => {
    render(<Badge>Test</Badge>)
    expect(screen.getByText("Test").className).toContain("bg-primary")
  })

  it("applies secondary variant", () => {
    render(<Badge variant="secondary">Secondary</Badge>)
    expect(screen.getByText("Secondary").className).toContain("bg-secondary")
  })

  it("applies destructive variant", () => {
    render(<Badge variant="destructive">Destructive</Badge>)
    expect(screen.getByText("Destructive").className).toContain("bg-destructive")
  })

  it("applies outline variant", () => {
    render(<Badge variant="outline">Outline</Badge>)
    expect(screen.getByText("Outline").className).toContain("text-foreground")
  })

  it("applies custom className", () => {
    render(<Badge className="custom">Test</Badge>)
    expect(screen.getByText("Test").className).toContain("custom")
  })
})