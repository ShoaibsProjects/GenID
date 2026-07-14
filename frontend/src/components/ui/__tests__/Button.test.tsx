import "@testing-library/jest-dom/jest-globals"
import { render, screen, fireEvent } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { Button } from "../Button"

describe("Button", () => {
  it("renders children text", () => {
    render(<Button>Click me</Button>)
    expect(screen.getByRole("button", { name: "Click me" })).toBeInTheDocument()
  })

  it("applies primary variant by default", () => {
    render(<Button>Primary</Button>)
    const btn = screen.getByRole("button")
    expect(btn.className).toContain("bg-accent")
  })

  it("applies secondary variant", () => {
    render(<Button variant="secondary">Secondary</Button>)
    const btn = screen.getByRole("button")
    expect(btn.className).toContain("bg-transparent")
    expect(btn.className).toContain("text-secondary")
  })

  it("applies ghost variant", () => {
    render(<Button variant="ghost">Ghost</Button>)
    const btn = screen.getByRole("button")
    expect(btn.className).toContain("border-transparent")
  })

  it("applies danger variant", () => {
    render(<Button variant="danger">Danger</Button>)
    const btn = screen.getByRole("button")
    expect(btn.className).toContain("bg-red-900/30")
  })

  it("applies size classes correctly", () => {
    const { rerender } = render(<Button size="sm">Small</Button>)
    expect(screen.getByRole("button").className).toContain("h-8")

    rerender(<Button size="md">Medium</Button>)
    expect(screen.getByRole("button").className).toContain("h-9")

    rerender(<Button size="lg">Large</Button>)
    expect(screen.getByRole("button").className).toContain("h-11")
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

  it("is disabled and shows spinner when loading", () => {
    render(<Button loading>Loading</Button>)
    const btn = screen.getByRole("button")
    expect(btn).toBeDisabled()
    expect(btn.querySelector("svg")).toBeInTheDocument()
  })

  it("does not call onClick when disabled", async () => {
    const user = userEvent.setup()
    const onClick = jest.fn()
    render(<Button disabled onClick={onClick}>No click</Button>)
    await user.click(screen.getByRole("button"))
    expect(onClick).not.toHaveBeenCalled()
  })

  it("renders icon when provided", () => {
    render(<Button icon={<span data-testid="icon">★</span>}>With icon</Button>)
    expect(screen.getByTestId("icon")).toBeInTheDocument()
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
