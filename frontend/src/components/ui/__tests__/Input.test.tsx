import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { Input } from "../Input"

describe("Input", () => {
  it("renders input element", () => {
    render(<Input placeholder="Enter text" />)
    expect(screen.getByPlaceholderText("Enter text")).toBeInTheDocument()
  })

  it("handles value changes", async () => {
    const user = userEvent.setup()
    render(<Input />)
    const input = screen.getByRole("textbox")
    await user.type(input, "hello")
    expect(input).toHaveValue("hello")
  })

  it("applies custom className", () => {
    render(<Input className="custom" />)
    expect(screen.getByRole("textbox").className).toContain("custom")
  })

  it("passes through input attributes", () => {
    render(<Input type="email" maxLength={50} data-testid="email" />)
    const input = screen.getByTestId("email")
    expect(input).toHaveAttribute("type", "email")
    expect(input).toHaveAttribute("maxLength", "50")
  })
})