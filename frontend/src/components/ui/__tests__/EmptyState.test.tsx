import "@testing-library/jest-dom/jest-globals"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { EmptyState } from "../EmptyState"

describe("EmptyState", () => {
  it("renders title", () => {
    render(<EmptyState title="No data found" />)
    expect(screen.getByText("No data found")).toBeInTheDocument()
  })

  it("renders description when provided", () => {
    render(
      <EmptyState title="Empty" description="No items to show" />
    )
    expect(screen.getByText("No items to show")).toBeInTheDocument()
  })

  it("does not render description when not provided", () => {
    render(<EmptyState title="Empty" />)
    expect(screen.queryByText(/items to show/i)).not.toBeInTheDocument()
  })

  it("renders action button when provided", () => {
    const onClick = jest.fn()
    render(
      <EmptyState
        title="Empty"
        action={{ label: "Create New", onClick }}
      />
    )
    expect(screen.getByText("Create New")).toBeInTheDocument()
  })

  it("calls action onClick when button is clicked", async () => {
    const user = userEvent.setup()
    const onClick = jest.fn()
    render(
      <EmptyState
        title="Empty"
        action={{ label: "Create", onClick }}
      />
    )
    await user.click(screen.getByText("Create"))
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it("renders icon when provided", () => {
    render(
      <EmptyState
        title="Empty"
        icon={<span data-testid="empty-icon">📭</span>}
      />
    )
    expect(screen.getByTestId("empty-icon")).toBeInTheDocument()
  })

  it("does not render action button when not provided", () => {
    render(<EmptyState title="Empty" />)
    expect(screen.queryByRole("button")).not.toBeInTheDocument()
  })
})
