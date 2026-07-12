// ─── ObserveID API Client ──────────────────────────────────
// Auto-detects deployment: relative URLs for same-origin (Go-served),
// or tunnel URL for Cloudflare Pages. Configurable via settings.

function getApiBase(): string {
  // Runtime check: if on Cloudflare Pages (no local backend), need tunnel URL
  if (typeof window !== "undefined") {
    // Check if we're on the Go backend (same origin serves both frontend + API)
    // If we can reach /health on the same origin, assume same-origin API
    const isLocalOrBackend = window.location.hostname === "localhost" ||
      window.location.hostname === "127.0.0.1" ||
      !window.location.hostname.includes("pages.dev")
    if (!isLocalOrBackend) {
      return localStorage.getItem("observeid_api_url") || ""
    }
  }
  // Built-time override from env
  if (typeof process !== "undefined" && process.env?.NEXT_PUBLIC_API_URL) {
    return process.env.NEXT_PUBLIC_API_URL
  }
  return ""
}

// Allows runtime API URL configuration (saved to localStorage)
export function setApiUrl(url: string): void {
  if (typeof window !== "undefined") {
    localStorage.setItem("observeid_api_url", url)
  }
}

export function getApiUrl(): string {
  if (typeof window !== "undefined") {
    return localStorage.getItem("observeid_api_url") || ""
  }
  return ""
}

interface RequestOptions {
  method?: string
  body?: any
  headers?: Record<string, string>
}

async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, headers = {} } = options
  const base = getApiBase()

  const res = await fetch(`${base}${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      ...headers,
    },
    body: body ? JSON.stringify(body) : undefined,
  })

  if (!res.ok) {
    const text = await res.text()
    throw new Error(`API ${method} ${path}: ${res.status} ${text}`)
  }

  return res.json()
}

// ─── Identities ───────────────────────────────────────────

export interface Identity {
  id: string
  name: string
  email: string
  status: string
  type: string
  department: string
  risk_score: number
}

export interface IdentitiesResponse {
  identities: Identity[]
  total: number
}

export interface Agent {
  id: string
  name: string
  type: string
  status: string
  risk_score: number
  is_governed: boolean
  owner_name: string
}

export interface AgentsResponse {
  agents: Agent[]
  total: number
}

export function fetchIdentities(): Promise<IdentitiesResponse> {
  return apiRequest<IdentitiesResponse>("/api/v1/identities")
}

export function fetchIdentity(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/identities/${id}`)
}

export function fetchAgents(): Promise<AgentsResponse> {
  return apiRequest<AgentsResponse>("/api/v1/agents")
}

export function createIdentity(data: {
  email: string
  display_name: string
  type: string
  department?: string
  employee_id?: string
  manager_id?: string
  source?: string
  tenant_id?: string
}): Promise<any> {
  return apiRequest<any>("/api/v1/identities", {
    method: "POST",
    body: { ...data, source: data.source || "manual" },
  })
}

export function updateIdentity(id: string, data: Record<string, any>): Promise<any> {
  return apiRequest<any>(`/api/v1/identities/${id}`, {
    method: "PATCH",
    body: data,
  })
}

export function deleteIdentity(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/identities/${id}`, { method: "DELETE" })
}

// ─── Connectors ───────────────────────────────────────────

export interface ConnectorConfig {
  id?: string
  name: string
  type: string
  endpoint?: string
  auth_type?: string
  username?: string
  password?: string
  client_id?: string
  client_secret?: string
  tenant_name?: string
  base_dn?: string
  domain?: string
}

export function fetchConnectors(): Promise<any> {
  return apiRequest<any>("/api/v1/connectors")
}

export function createConnector(config: ConnectorConfig): Promise<any> {
  return apiRequest<any>("/api/v1/connectors", { method: "POST", body: config })
}

export function testConnectorConnection(config: ConnectorConfig): Promise<any> {
  return apiRequest<any>("/api/v1/connectors/test", { method: "POST", body: config })
}

export function connectConnector(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/connect`, { method: "POST" })
}

export function disconnectConnector(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/disconnect`, { method: "POST" })
}

export function syncConnector(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/sync`, { method: "POST" })
}

export function fetchConnectorUsers(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/users`)
}

export function fetchConnectorIdentities(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/identities`)
}

export function deleteConnector(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}`, { method: "DELETE" })
}

// ─── Connector: Groups / Entitlements / Resources / Full Sync ──

export function fetchConnectorGroups(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/groups`)
}

export function fetchConnectorEntitlements(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/entitlements`)
}

export function fetchConnectorResources(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/resources`)
}

export function fullSyncConnector(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/full-sync`, { method: "POST" })
}

// ─── IAM Lifecycle Management ────────────────────────────

export function executeLCM(req: {
  action: string
  connector_ids: string[]
  user?: any
  group?: any
  external_id?: string
}): Promise<any> {
  return apiRequest<any>("/api/v1/lcm", { method: "POST", body: req })
}

export function fetchLCMHistory(): Promise<any> {
  return apiRequest<any>("/api/v1/lcm/history")
}

// ─── Groups / Roles ──────────────────────────────────────

export function fetchGroups(): Promise<any> {
  return apiRequest<any>("/api/v1/groups")
}

export function createGroup(data: { name: string; description?: string; role_type?: string; tenant_id?: string }): Promise<any> {
  return apiRequest<any>("/api/v1/groups", {
    method: "POST",
    body: { ...data, role_type: data.role_type || "custom", tenant_id: data.tenant_id || "default" },
  })
}

export function deleteGroup(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/groups/${id}`, { method: "DELETE" })
}

export function assignRole(data: { identity_id: string; role_id: string; assigned_by?: string; source?: string }): Promise<any> {
  return apiRequest<any>("/api/v1/roles/assign", {
    method: "POST",
    body: { ...data, assigned_by: data.assigned_by || "admin", source: data.source || "manual" },
  })
}

export function removeRole(data: { identity_id: string; role_id: string }): Promise<any> {
  return apiRequest<any>("/api/v1/roles/remove", { method: "POST", body: data })
}

// ─── Access ──────────────────────────────────────────────

export function checkAccess(data: { identity_id: string; resource_id: string; action: string; tenant_id?: string }): Promise<any> {
  return apiRequest<any>("/api/v1/access/check", { method: "POST", body: data })
}

export function grantAccess(data: any): Promise<any> {
  return apiRequest<any>("/api/v1/access/grant", { method: "POST", body: data })
}

export function revokeAccess(data: any): Promise<any> {
  return apiRequest<any>("/api/v1/access/revoke", { method: "POST", body: data })
}

// ─── CAEP ────────────────────────────────────────────────

export function fetchCAEPEvents(): Promise<any> {
  return apiRequest<any>("/api/v1/caep/events")
}

export function broadcastCAEP(data: { event_type: string; identity_id: string; receivers?: string[] }): Promise<any> {
  return apiRequest<any>("/api/v1/caep/broadcast", { method: "POST", body: data })
}

// ─── Copilot ─────────────────────────────────────────────

export function copilotQuery(data: { question: string; user_id?: string; tenant_id?: string }): Promise<any> {
  return apiRequest<any>("/api/v1/copilot/query", { method: "POST", body: data })
}

// ─── Vault / Secrets ────────────────────────────────────

export function fetchSecrets(): Promise<any> {
  return apiRequest<any>("/api/v1/vault/secrets")
}

export function storeSecret(data: { name: string; type: string; reference?: string; value: string }): Promise<any> {
  return apiRequest<any>("/api/v1/vault/secrets", { method: "POST", body: data })
}

export function retrieveSecret(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/vault/secrets/${id}`)
}

export function deleteSecret(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/vault/secrets/${id}`, { method: "DELETE" })
}

// ─── Health ──────────────────────────────────────────────

export function fetchHealth(): Promise<any> {
  return apiRequest<any>("/health")
}
