import "@testing-library/jest-dom/jest-globals"
import { render, screen } from "@testing-library/react"
import { StatCard } from "../StatCard"

describe("StatCard", () => {
  it("renders label and value", () => {
    render(<StatCard label="Users" value={42} />)
    expect(screen.getByText("Users")).toBeInTheDocument()
    expect(screen.getByText("42")).toBeInTheDocument()
  })

  it("renders string value", () => {
    render(<StatCard label="Status" value="Active" />)
    expect(screen.getByText("Active")).toBeInTheDocument()
  })

  it("renders positive trend with up arrow", () => {
    const { container } = render(<StatCard label="Growth" value={100} trend={{ value: "+12%", positive: true }} />)
    const trendP = container.querySelector("p.text-green-400")
    expect(trendP).toBeInTheDocument()
    expect(trendP?.textContent).toContain("↑")
    expect(trendP?.textContent).toContain("+12%")
  })

  it("renders negative trend with down arrow", () => {
    const { container } = render(<StatCard label="Decline" value={50} trend={{ value: "-5%", positive: false }} />)
    const trendP = container.querySelector("p.text-red-400")
    expect(trendP).toBeInTheDocument()
    expect(trendP?.textContent).toContain("↓")
    expect(trendP?.textContent).toContain("-5%")
  })

  it("renders icon when provided", () => {
    render(
      <StatCard
        label="Icon"
        value={1}
        icon={<span data-testid="icon">★</span>}
      />
    )
    expect(screen.getByTestId("icon")).toBeInTheDocument()
  })

  it("calls onClick when clicked", () => {
    const onClick = jest.fn()
    render(<StatCard label="Clickable" value={1} onClick={onClick} />)
    // StatCard wraps in Card with hover when onClick is provided
    expect(screen.getByText("Clickable").closest("[class*='cursor-pointer']")).toBeInTheDocument()
  })

  it("does not have cursor-pointer without onClick", () => {
    render(<StatCard label="Not clickable" value={1} />)
    expect(screen.getByText("Not clickable").closest("[class*='cursor-pointer']")).toBeNull()
  })
})
