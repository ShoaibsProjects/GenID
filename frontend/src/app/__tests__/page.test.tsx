import "@testing-library/jest-dom/jest-globals"
import { render, screen } from "@testing-library/react"
import HomePage from "../page"

describe("HomePage", () => {
  it("renders the main heading", () => {
    render(<HomePage />)
    expect(screen.getByText("ObserveID Reimagined Identity Fabric")).toBeInTheDocument()
  })

  it("renders the subtitle", () => {
    render(<HomePage />)
    expect(screen.getByText(/Event-Driven, AI-Native Identity Governance Platform/)).toBeInTheDocument()
  })

  it("renders Launch Dashboard link", () => {
    render(<HomePage />)
    const link = screen.getByText("Launch Dashboard")
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute("href", "/dashboard")
  })

  it("renders View Identities link", () => {
    render(<HomePage />)
    const link = screen.getByText("View Identities")
    expect(link).toBeInTheDocument()
    expect(link).toHaveAttribute("href", "/identities")
  })

  it("renders the three feature cards", () => {
    render(<HomePage />)
    expect(screen.getByText("GraphRAG AI Copilot")).toBeInTheDocument()
    expect(screen.getByText("Agent Identity Platform")).toBeInTheDocument()
    expect(screen.getByText("Real-Time Durable Execution")).toBeInTheDocument()
  })

  it("renders feature descriptions", () => {
    render(<HomePage />)
    expect(screen.getByText(/Natural language identity queries/)).toBeInTheDocument()
    expect(screen.getByText(/First-class identity for AI agents/)).toBeInTheDocument()
    expect(screen.getByText(/Temporal-powered workflows/)).toBeInTheDocument()
  })
})
