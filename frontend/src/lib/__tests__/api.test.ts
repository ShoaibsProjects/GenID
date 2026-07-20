import "@testing-library/jest-dom/jest-globals"
import { setApiUrl, getApiUrl } from "../api"

beforeEach(() => {
  localStorage.clear()
})

describe("setApiUrl / getApiUrl", () => {
  it("returns empty string by default", () => {
    expect(getApiUrl()).toBe("")
  })

  it("stores and retrieves API URL", () => {
    setApiUrl("https://api.example.com")
    expect(getApiUrl()).toBe("https://api.example.com")
  })

  it("overwrites previous URL", () => {
    setApiUrl("https://first.com")
    setApiUrl("https://second.com")
    expect(getApiUrl()).toBe("https://second.com")
  })

  it("stores to localStorage", () => {
    setApiUrl("https://test.com")
    expect(localStorage.getItem("observeid_api_url")).toBe("https://test.com")
  })
})

describe("fetchIdentities", () => {
  it("calls the correct endpoint", async () => {
    const mockResponse = { identities: [], total: 0 }
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockResponse),
    })

    const { fetchIdentities } = await import("../api")
    const result = await fetchIdentities()
    expect(result).toEqual(mockResponse)
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/identities",
      expect.objectContaining({ method: "GET" })
    )
  })
})

describe("fetchAgents", () => {
  it("calls the correct endpoint", async () => {
    const mockResponse = { agents: [], total: 0 }
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockResponse),
    })

    const { fetchAgents } = await import("../api")
    const result = await fetchAgents()
    expect(result).toEqual(mockResponse)
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/agents",
      expect.objectContaining({ method: "GET" })
    )
  })
})

describe("apiRequest error handling", () => {
  it("throws on non-OK response", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 404,
      text: () => Promise.resolve("not found"),
    })

    const { fetchIdentities } = await import("../api")
    await expect(fetchIdentities()).rejects.toThrow("API GET /api/v1/identities: 404 not found")
  })

  it("includes method in error message", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: false,
      status: 500,
      text: () => Promise.resolve("internal error"),
    })

    const { deleteIdentity } = await import("../api")
    await expect(deleteIdentity("123")).rejects.toThrow("DELETE")
  })
})

describe("createIdentity", () => {
  it("sends POST with body", async () => {
    const mockResponse = { id: "new-id" }
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockResponse),
    })

    const { createIdentity } = await import("../api")
    await createIdentity({
      email: "test@example.com",
      display_name: "Test User",
      type: "human",
    })

    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/identities",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          email: "test@example.com",
          display_name: "Test User",
          type: "human",
          source: "manual",
        }),
      })
    )
  })
})

describe("connector endpoints", () => {
  it("fetchConnectors calls GET", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ connectors: [] }),
    })
    const { fetchConnectors } = await import("../api")
    await fetchConnectors()
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/connectors",
      expect.objectContaining({ method: "GET" })
    )
  })

  it("createConnector calls POST", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ id: "c1" }),
    })
    const { createConnector } = await import("../api")
    await createConnector({ name: "LDAP", type: "ldap" })
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/connectors",
      expect.objectContaining({ method: "POST" })
    )
  })

  it("syncConnector calls POST to /sync", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({}),
    })
    const { syncConnector } = await import("../api")
    await syncConnector("c1")
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/connectors/c1/sync",
      expect.objectContaining({ method: "POST" })
    )
  })
})

describe("health endpoint", () => {
  it("fetchHealth calls /health", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ status: "ok" }),
    })
    const { fetchHealth } = await import("../api")
    await fetchHealth()
    expect(global.fetch).toHaveBeenCalledWith(
      "/health",
      expect.objectContaining({ method: "GET" })
    )
  })
})

describe("QUERY method endpoints (RFC 10008)", () => {
  beforeEach(() => {
    localStorage.clear()
    jest.resetModules()
  })

  it("checkAccess uses QUERY method", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ allowed: true }),
    })
    const { checkAccess } = await import("../api")
    await checkAccess({ identity_id: "i1", resource_id: "r1", action: "read" })
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/access/check",
      expect.objectContaining({ method: "QUERY" })
    )
  })

  it("copilotQuery uses QUERY method", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ answer: "test" }),
    })
    const { copilotQuery } = await import("../api")
    await copilotQuery({ question: "who has admin access?" })
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/copilot/query",
      expect.objectContaining({ method: "QUERY" })
    )
  })

  it("testConnectorConnection uses POST method", async () => {
    global.fetch = jest.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ success: true }),
    })
    const { testConnectorConnection } = await import("../api")
    await testConnectorConnection({ name: "LDAP", type: "ldap" })
    expect(global.fetch).toHaveBeenCalledWith(
      "/api/v1/connectors/test",
      expect.objectContaining({ method: "POST" })
    )
  })
})
