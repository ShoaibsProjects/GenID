import "@testing-library/jest-dom/jest-globals"
import { render, screen, fireEvent } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { Modal } from "../Modal"

describe("Modal", () => {
  it("renders nothing when closed", () => {
    render(
      <Modal open={false} onClose={jest.fn()} title="Test">
        Content
      </Modal>
    )
    expect(screen.queryByText("Test")).not.toBeInTheDocument()
  })

  it("renders when open", () => {
    render(
      <Modal open={true} onClose={jest.fn()} title="Test Modal">
        Modal content
      </Modal>
    )
    expect(screen.getByText("Test Modal")).toBeInTheDocument()
    expect(screen.getByText("Modal content")).toBeInTheDocument()
  })

  it("renders description when provided", () => {
    render(
      <Modal open={true} onClose={jest.fn()} title="Title" description="Description text">
        Content
      </Modal>
    )
    expect(screen.getByText("Description text")).toBeInTheDocument()
  })

  it("calls onClose when backdrop is clicked", async () => {
    const user = userEvent.setup()
    const onClose = jest.fn()
    render(
      <Modal open={true} onClose={onClose} title="Title">
        Content
      </Modal>
    )
    // The backdrop div
    const backdrop = document.querySelector(".backdrop-blur-sm")!
    await user.click(backdrop)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it("calls onClose when close button is clicked", async () => {
    const user = userEvent.setup()
    const onClose = jest.fn()
    render(
      <Modal open={true} onClose={onClose} title="Title">
        Content
      </Modal>
    )
    // The close button (svg with X path)
    const closeBtn = screen.getByRole("button")
    await user.click(closeBtn)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it("renders footer when provided", () => {
    render(
      <Modal
        open={true}
        onClose={jest.fn()}
        title="Title"
        footer={<button>Save</button>}
      >
        Content
      </Modal>
    )
    expect(screen.getByText("Save")).toBeInTheDocument()
  })

  it("does not render footer when not provided", () => {
    const { container } = render(
      <Modal open={true} onClose={jest.fn()} title="Title">
        Content
      </Modal>
    )
    expect(container.querySelector("footer")).toBeNull()
  })

  it("applies size classes", () => {
    const { rerender } = render(
      <Modal open={true} onClose={jest.fn()} title="Title" size="sm">
        Content
      </Modal>
    )
    expect(document.querySelector(".max-w-sm")).toBeInTheDocument()

    rerender(
      <Modal open={true} onClose={jest.fn()} title="Title" size="lg">
        Content
      </Modal>
    )
    expect(document.querySelector(".max-w-2xl")).toBeInTheDocument()
  })
})
