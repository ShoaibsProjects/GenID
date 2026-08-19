import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { Button } from "../Button"

describe("Button", () => {
  it("renders children text", () => {
    render(<Button>Click me</Button>)
    expect(screen.getByRole("button", { name: "Click me" })).toBeInTheDocument()
  })

  it("applies primary variant by default", () => {
    render(<Button>Primary</Button>)
    expect(screen.getByRole("button").className).toContain("bg-primary")
  })

  it("applies secondary variant", () => {
    render(<Button variant="secondary">Secondary</Button>)
    expect(screen.getByRole("button").className).toContain("bg-secondary")
  })

  it("applies outline variant", () => {
    render(<Button variant="outline">Outline</Button>)
    expect(screen.getByRole("button").className).toContain("border-input")
  })

  it("applies destructive variant", () => {
    render(<Button variant="destructive">Destructive</Button>)
    expect(screen.getByRole("button").className).toContain("bg-destructive")
  })

  it("applies ghost variant", () => {
    render(<Button variant="ghost">Ghost</Button>)
    expect(screen.getByRole("button").className).toContain("hover:bg-accent")
  })

  it("applies size classes correctly", () => {
    const { rerender } = render(<Button size="sm">Small</Button>)
    expect(screen.getByRole("button").className).toContain("h-8")

    rerender(<Button size="lg">Large</Button>)
    expect(screen.getByRole("button").className).toContain("h-10")
  })

  it("calls onClick when clicked", async () => {
    const user = userEvent.setup()
    const onClick = jest.fn()
    render(<Button onClick={onClick}>Click</Button>)
    await user.click(screen.getByRole("button"))
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it("is disabled when disabled prop is true", () => {
    render(<Button disabled>Disabled</Button>)
    expect(screen.getByRole("button")).toBeDisabled()
  })

  it("does not call onClick when disabled", async () => {
    const user = userEvent.setup()
    const onClick = jest.fn()
    render(<Button disabled onClick={onClick}>No click</Button>)
    await user.click(screen.getByRole("button"))
    expect(onClick).not.toHaveBeenCalled()
  })

  it("applies custom className", () => {
    render(<Button className="custom-class">Custom</Button>)
    expect(screen.getByRole("button").className).toContain("custom-class")
  })

  it("passes through additional button attributes", () => {
    render(<Button type="submit" data-testid="submit-btn">Submit</Button>)
    const btn = screen.getByTestId("submit-btn")
    expect(btn).toHaveAttribute("type", "submit")
  })
})