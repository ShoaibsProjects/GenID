import { render, screen, fireEvent } from "@testing-library/react"
import {
  Card,
  CardHeader,
  CardFooter,
  CardTitle,
  CardDescription,
  CardContent,
} from "../Card"

describe("Card", () => {
  it("renders children", () => {
    render(<Card>Card content</Card>)
    expect(screen.getByText("Card content")).toBeInTheDocument()
  })

  it("applies default border", () => {
    const { container } = render(<Card>Test</Card>)
    expect(container.firstChild).toHaveClass("border")
  })

  it("calls onClick when clickable", () => {
    const onClick = jest.fn()
    render(<Card onClick={onClick}>Clickable</Card>)
    fireEvent.click(screen.getByText("Clickable"))
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it("applies custom className", () => {
    const { container } = render(<Card className="custom">Test</Card>)
    expect(container.firstChild).toHaveClass("custom")
  })
})

describe("Card sections", () => {
  it("CardHeader renders children", () => {
    render(<Card><CardHeader>Header</CardHeader></Card>)
    expect(screen.getByText("Header")).toBeInTheDocument()
  })

  it("CardTitle renders children", () => {
    render(<Card><CardTitle>Title</CardTitle></Card>)
    expect(screen.getByText("Title")).toBeInTheDocument()
  })

  it("CardDescription renders children", () => {
    render(<Card><CardDescription>Description</CardDescription></Card>)
    expect(screen.getByText("Description")).toBeInTheDocument()
  })

  it("CardContent renders children", () => {
    render(<Card><CardContent>Content</CardContent></Card>)
    expect(screen.getByText("Content")).toBeInTheDocument()
  })

  it("CardFooter renders children", () => {
    render(<Card><CardFooter>Footer</CardFooter></Card>)
    expect(screen.getByText("Footer")).toBeInTheDocument()
  })
})