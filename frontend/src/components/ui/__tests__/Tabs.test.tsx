import "@testing-library/jest-dom/jest-globals"
import { render, screen, fireEvent } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { Tabs } from "../Tabs"

const tabs = [
  { id: "all", label: "All" },
  { id: "active", label: "Active" },
  { id: "inactive", label: "Inactive", count: 5 },
]

describe("Tabs", () => {
  it("renders all tab labels", () => {
    render(<Tabs tabs={tabs} active="all" onChange={jest.fn()} />)
    expect(screen.getByText("All")).toBeInTheDocument()
    expect(screen.getByText("Active")).toBeInTheDocument()
    expect(screen.getByText("Inactive")).toBeInTheDocument()
  })

  it("shows count when provided", () => {
    render(<Tabs tabs={tabs} active="all" onChange={jest.fn()} />)
    expect(screen.getByText("5")).toBeInTheDocument()
  })

  it("calls onChange with tab id when clicked", async () => {
    const user = userEvent.setup()
    const onChange = jest.fn()
    render(<Tabs tabs={tabs} active="all" onChange={onChange} />)
    await user.click(screen.getByText("Active"))
    expect(onChange).toHaveBeenCalledWith("active")
  })

  it("applies active styles to selected tab", () => {
    render(<Tabs tabs={tabs} active="active" onChange={jest.fn()} />)
    const activeBtn = screen.getByText("Active").closest("button")!
    expect(activeBtn.className).toContain("bg-white/[0.08]")
    expect(activeBtn.className).toContain("text-primary")
  })

  it("applies inactive styles to non-selected tabs", () => {
    render(<Tabs tabs={tabs} active="all" onChange={jest.fn()} />)
    const inactiveBtn = screen.getByText("Active").closest("button")!
    expect(inactiveBtn.className).toContain("text-secondary")
  })

  it("applies custom className", () => {
    const { container } = render(
      <Tabs tabs={tabs} active="all" onChange={jest.fn()} className="custom-tabs" />
    )
    expect(container.firstChild).toHaveClass("custom-tabs")
  })
})
