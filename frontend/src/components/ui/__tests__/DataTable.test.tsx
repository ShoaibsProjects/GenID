import "@testing-library/jest-dom/jest-globals"
import { render, screen, fireEvent } from "@testing-library/react"
import { DataTable } from "../DataTable"

interface TestRow {
  id: string
  name: string
  status: string
}

const columns = [
  { key: "name", header: "Name", render: (row: TestRow) => row.name },
  { key: "status", header: "Status", render: (row: TestRow) => row.status },
]

const data: TestRow[] = [
  { id: "1", name: "Alice", status: "Active" },
  { id: "2", name: "Bob", status: "Inactive" },
]

describe("DataTable", () => {
  it("renders column headers", () => {
    render(<DataTable columns={columns} data={data} keyField="id" />)
    expect(screen.getByText("Name")).toBeInTheDocument()
    expect(screen.getByText("Status")).toBeInTheDocument()
  })

  it("renders data rows", () => {
    render(<DataTable columns={columns} data={data} keyField="id" />)
    expect(screen.getByText("Alice")).toBeInTheDocument()
    expect(screen.getByText("Bob")).toBeInTheDocument()
    expect(screen.getByText("Active")).toBeInTheDocument()
    expect(screen.getByText("Inactive")).toBeInTheDocument()
  })

  it("shows empty message when data is empty", () => {
    render(
      <DataTable
        columns={columns}
        data={[]}
        keyField="id"
        emptyMessage="No records"
      />
    )
    expect(screen.getByText("No records")).toBeInTheDocument()
  })

  it("shows default empty message", () => {
    render(<DataTable columns={columns} data={[]} keyField="id" />)
    expect(screen.getByText("No data")).toBeInTheDocument()
  })

  it("calls onRowClick when a row is clicked", () => {
    const onRowClick = jest.fn()
    render(
      <DataTable columns={columns} data={data} keyField="id" onRowClick={onRowClick} />
    )
    fireEvent.click(screen.getByText("Alice"))
    expect(onRowClick).toHaveBeenCalledWith(data[0])
  })

  it("shows loading state", () => {
    render(<DataTable columns={columns} data={[]} keyField="id" loading />)
    expect(screen.queryByText("No data")).not.toBeInTheDocument()
    // Loading shows skeleton animation
    expect(document.querySelector(".animate-pulse")).toBeInTheDocument()
  })

  it("renders table element", () => {
    render(<DataTable columns={columns} data={data} keyField="id" />)
    expect(screen.getByRole("table")).toBeInTheDocument()
  })

  it("renders thead and tbody", () => {
    const { container } = render(<DataTable columns={columns} data={data} keyField="id" />)
    expect(container.querySelector("thead")).toBeInTheDocument()
    expect(container.querySelector("tbody")).toBeInTheDocument()
  })

  it("supports right-aligned columns", () => {
    const rightColumns = [
      { key: "name", header: "Name", render: (row: TestRow) => row.name, align: "right" as const },
    ]
    render(<DataTable columns={rightColumns} data={data} keyField="id" />)
    const th = screen.getByText("Name")
    expect(th.className).toContain("text-right")
  })
})
