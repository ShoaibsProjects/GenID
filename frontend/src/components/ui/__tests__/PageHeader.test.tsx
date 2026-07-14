import "@testing-library/jest-dom/jest-globals"
import { render, screen } from "@testing-library/react"
import { PageHeader } from "../PageHeader"

describe("PageHeader", () => {
  it("renders title", () => {
    render(<PageHeader title="Dashboard" />)
    expect(screen.getByText("Dashboard")).toBeInTheDocument()
  })

  it("renders title as h1", () => {
    render(<PageHeader title="Test" />)
    const heading = screen.getByRole("heading", { level: 1 })
    expect(heading).toHaveTextContent("Test")
  })

  it("renders description when provided", () => {
    render(<PageHeader title="Test" description="Page description" />)
    expect(screen.getByText("Page description")).toBeInTheDocument()
  })

  it("does not render description when not provided", () => {
    render(<PageHeader title="Test" />)
    expect(screen.queryByText(/description/i)).not.toBeInTheDocument()
  })

  it("renders actions when provided", () => {
    render(
      <PageHeader title="Test" actions={<button>Add Item</button>} />
    )
    expect(screen.getByText("Add Item")).toBeInTheDocument()
  })

  it("renders children in actions area", () => {
    render(
      <PageHeader title="Test">
        <button>Child action</button>
      </PageHeader>
    )
    expect(screen.getByText("Child action")).toBeInTheDocument()
  })
})
