import "@testing-library/jest-dom/jest-globals"
import { render, screen, fireEvent } from "@testing-library/react"
import { Card, CardHeader, CardBody, CardFooter } from "../Card"

describe("Card", () => {
  it("renders children", () => {
    render(<Card>Card content</Card>)
    expect(screen.getByText("Card content")).toBeInTheDocument()
  })

  it("applies default variant border", () => {
    const { container } = render(<Card>Test</Card>)
    expect(container.firstChild).toHaveClass("border-border")
  })

  it("applies accent variant with left border", () => {
    const { container } = render(<Card variant="accent">Accent</Card>)
    expect(container.firstChild).toHaveClass("border-l-[3px]")
  })

  it("applies error variant with red left border", () => {
    const { container } = render(<Card variant="error">Error</Card>)
    expect(container.firstChild).toHaveClass("border-l-[3px]")
  })

  it("calls onClick when clickable", () => {
    const onClick = jest.fn()
    render(<Card onClick={onClick}>Clickable</Card>)
    fireEvent.click(screen.getByText("Clickable"))
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it("applies hover styles when hover prop is true", () => {
    const { container } = render(<Card hover>Hoverable</Card>)
    expect(container.firstChild).toHaveClass("hover:bg-white/[0.02]")
  })

  it("applies custom className", () => {
    const { container } = render(<Card className="custom">Test</Card>)
    expect(container.firstChild).toHaveClass("custom")
  })
})

describe("CardHeader", () => {
  it("renders children", () => {
    render(<Card><CardHeader>Header</CardHeader></Card>)
    expect(screen.getByText("Header")).toBeInTheDocument()
  })
})

describe("CardBody", () => {
  it("renders children", () => {
    render(<Card><CardBody>Body</CardBody></Card>)
    expect(screen.getByText("Body")).toBeInTheDocument()
  })
})

describe("CardFooter", () => {
  it("renders children", () => {
    render(<Card><CardFooter>Footer</CardFooter></Card>)
    expect(screen.getByText("Footer")).toBeInTheDocument()
  })
})
