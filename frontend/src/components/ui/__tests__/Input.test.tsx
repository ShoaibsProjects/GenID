import "@testing-library/jest-dom/jest-globals"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { Input, Select } from "../Input"

describe("Input", () => {
  it("renders input element", () => {
    render(<Input placeholder="Enter text" />)
    expect(screen.getByPlaceholderText("Enter text")).toBeInTheDocument()
  })

  it("renders label when provided", () => {
    render(<Input label="Email" />)
    expect(screen.getByText("Email")).toBeInTheDocument()
  })

  it("renders error message when provided", () => {
    render(<Input error="Required field" />)
    expect(screen.getByText("Required field")).toBeInTheDocument()
  })

  it("renders hint when provided and no error", () => {
    render(<Input hint="Enter your email" />)
    expect(screen.getByText("Enter your email")).toBeInTheDocument()
  })

  it("hides hint when error is present", () => {
    render(<Input hint="Some hint" error="Error msg" />)
    expect(screen.queryByText("Some hint")).not.toBeInTheDocument()
    expect(screen.getByText("Error msg")).toBeInTheDocument()
  })

  it("applies error border class", () => {
    render(<Input error="Error" />)
    const input = screen.getByRole("textbox")
    expect(input.className).toContain("border-red-500")
  })

  it("handles value changes", async () => {
    const user = userEvent.setup()
    render(<Input />)
    const input = screen.getByRole("textbox")
    await user.type(input, "hello")
    expect(input).toHaveValue("hello")
  })

  it("renders icon when provided", () => {
    render(<Input icon={<span data-testid="icon">🔍</span>} />)
    expect(screen.getByTestId("icon")).toBeInTheDocument()
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

describe("Select", () => {
  const options = [
    { value: "ld", label: "LDAP" },
    { value: "scim", label: "SCIM" },
  ]

  it("renders select element", () => {
    render(<Select options={options} />)
    expect(screen.getByRole("combobox")).toBeInTheDocument()
  })

  it("renders all options", () => {
    render(<Select options={options} />)
    expect(screen.getByText("LDAP")).toBeInTheDocument()
    expect(screen.getByText("SCIM")).toBeInTheDocument()
  })

  it("renders label when provided", () => {
    render(<Select label="Type" options={options} />)
    expect(screen.getByText("Type")).toBeInTheDocument()
  })

  it("renders error when provided", () => {
    render(<Select error="Required" options={options} />)
    expect(screen.getByText("Required")).toBeInTheDocument()
  })

  it("applies error border class", () => {
    render(<Select error="Error" options={options} />)
    const select = screen.getByRole("combobox")
    expect(select.className).toContain("border-red-500")
  })

  it("handles selection change", async () => {
    const user = userEvent.setup()
    render(<Select options={options} />)
    const select = screen.getByRole("combobox")
    await user.selectOptions(select, "scim")
    expect(select).toHaveValue("scim")
  })
})
