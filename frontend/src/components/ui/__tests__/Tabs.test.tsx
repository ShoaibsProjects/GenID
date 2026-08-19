import { render, screen } from "@testing-library/react"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "../Tabs"

describe("Tabs", () => {
  it("renders all tab triggers", () => {
    render(
      <Tabs defaultValue="all">
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="active">Active</TabsTrigger>
        </TabsList>
        <TabsContent value="all">All content</TabsContent>
        <TabsContent value="active">Active content</TabsContent>
      </Tabs>
    )
    expect(screen.getByText("All")).toBeInTheDocument()
    expect(screen.getByText("Active")).toBeInTheDocument()
  })

  it("shows the active tab content", () => {
    render(
      <Tabs defaultValue="active">
        <TabsList>
          <TabsTrigger value="all">All</TabsTrigger>
          <TabsTrigger value="active">Active</TabsTrigger>
        </TabsList>
        <TabsContent value="all">All content</TabsContent>
        <TabsContent value="active">Active content</TabsContent>
      </Tabs>
    )
    expect(screen.getByText("Active content")).toBeInTheDocument()
  })
})