// ─── GenID Reimagined API Client ───────────────────────
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
      return localStorage.getItem("genid_api_url") || ""
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
    localStorage.setItem("genid_api_url", url)
  }
}

export function getApiUrl(): string {
  if (typeof window !== "undefined") {
    return localStorage.getItem("genid_api_url") || ""
  }
  return ""
}

// ─── Auth Token Management ─────────────────────────────────────
// Token persisted in localStorage. Auto-attached as Authorization: Bearer to every request.

const TOKEN_KEY = "genid_access_token"
const USER_KEY  = "genid_user_email"

// Dev login used for JWT auto-recovery on 401 (stale signing key).
const DEV_LOGIN_USER = "admin@genid.io"
const DEV_LOGIN_PASS = "dev-login"

export function getAuthToken(): string | null {
  if (typeof window === "undefined") return null
  return localStorage.getItem(TOKEN_KEY)
}

export function setAuthToken(token: string, userEmail: string): void {
  if (typeof window === "undefined") return
  localStorage.setItem(TOKEN_KEY, token)
  localStorage.setItem(USER_KEY, userEmail)
}

export function clearAuthToken(): void {
  if (typeof window === "undefined") return
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
}

export function getStoredUserEmail(): string | null {
  if (typeof window === "undefined") return null
  return localStorage.getItem(USER_KEY)
}

export function isLoggedIn(): boolean {
  return !!getAuthToken()
}

// ─── Tenant Management ─────────────────────────────────────
const TENANT_KEY = "genid_tenant"
const TENANT_MAP: Record<string, string> = {
  default: "00000000-0000-0000-0000-000000000001",
  acme:    "11111111-1111-1111-1111-111111111111",
  globex:  "22222222-2222-2222-2222-222222222222",
  umbrella:"33333333-3333-3333-3333-333333333333",
}

export function getTenantId(): string {
  if (typeof window === "undefined") return TENANT_MAP.default
  const slug = localStorage.getItem(TENANT_KEY) || "default"
  return TENANT_MAP[slug] || TENANT_MAP.default
}

export function getTenantSlug(): string {
  if (typeof window === "undefined") return "default"
  return localStorage.getItem(TENANT_KEY) || "default"
}

// Login via the dev login endpoint. Returns true on success.
export async function devLogin(username: string, password: string): Promise<{ token: string; user_id: string } | null> {
  const base = getApiBase()
  const res = await fetch(`${base}/api/v1/dev/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) return null
  const data = await res.json()
  if (data.access_token) {
    setAuthToken(data.access_token, username)
    return { token: data.access_token, user_id: data.user_id }
  }
  return null
}

interface RequestOptions {
  method?: string
  body?: any
  headers?: Record<string, string>
}

// authFetch is a drop-in replacement for fetch() that attaches the JWT
// to same-origin API calls. Use for all /api/*, /scim/*, /graphql calls.
// On 401 (stale/rotated JWT) it clears the token, re-logins once, and retries.
export async function authFetch(input: string, init: RequestInit = {}): Promise<Response> {
  let token = getAuthToken()
  const headers = new Headers(init.headers || {})
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`)
  }
  const tenantId = getTenantId()
  if (tenantId && !headers.has("X-Tenant-ID")) {
    headers.set("X-Tenant-ID", tenantId)
  }
  let res = await fetch(input, { ...init, headers })

  // Auto-recover from stale JWT (e.g. signing key rotated on deploy):
  // clear + re-login + retry once. Skip the login endpoint itself.
  if (res.status === 401 && token && !input.includes("/api/v1/dev/login")) {
    clearAuthToken()
    const fresh = await devLogin(DEV_LOGIN_USER, DEV_LOGIN_PASS)
    if (fresh) {
      const retryHeaders = new Headers(init.headers || {})
      retryHeaders.set("Authorization", `Bearer ${fresh.token}`)
      const t = getTenantId()
      if (t && !retryHeaders.has("X-Tenant-ID")) retryHeaders.set("X-Tenant-ID", t)
      res = await fetch(input, { ...init, headers: retryHeaders })
    }
  }
  return res
}

// ─── Audit Chain Verification ────────────────────────────

export interface AuditVerifyResult {
  status: "intact" | "tampered"
  checked: number
  broken_at?: string
  reason?: string
  verified_at: string
}

export function fetchAuditVerify(): Promise<AuditVerifyResult> {
  return apiRequest<AuditVerifyResult>("/api/v1/audit/verify")
}

async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, headers = {} } = options
  const base = getApiBase()

  let token = getAuthToken()
  let res = await rawFetch(base, path, method, body, token, headers)

  // Auto-recover from stale JWT: clear + re-login + retry once
  if (res.status === 401 && token && path !== "/api/v1/dev/login") {
    clearAuthToken()
    const fresh = await devLogin(DEV_LOGIN_USER, DEV_LOGIN_PASS)
    if (fresh) {
      token = fresh.token
      res = await rawFetch(base, path, method, body, token, headers)
    }
  }

  if (!res.ok) {
    const text = await res.text()
    throw new Error(`API ${method} ${path}: ${res.status} ${text}`)
  }

  return res.json()
}

async function rawFetch(
  base: string,
  path: string,
  method: string,
  body: any,
  token: string | null,
  headers: Record<string, string>
): Promise<Response> {
  const authHeaders: Record<string, string> = {}
  if (token) authHeaders["Authorization"] = `Bearer ${token}`
  // Inject tenant header for RLS filtering
  const tenantId = getTenantId()
  if (tenantId) authHeaders["X-Tenant-ID"] = tenantId
  return fetch(`${base}${path}`, {
    method,
    headers: { "Content-Type": "application/json", ...authHeaders, ...headers },
    body: body ? JSON.stringify(body) : undefined,
  })
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

// ─── Access Certifications ────────────────────────────────

export interface CertificationEntry {
  id: string
  identity_id: string
  identity_email: string
  display_name: string
  resource: string
  risk_score: number
  status: string
  decision: string | null
  created_at: string
}

export interface CertificationCampaign {
  id: string
  name: string
  campaign_type: string
  status: string
  starts_at: string
  ends_at: string
  pending_count: number
  entries: CertificationEntry[]
}

export interface CertificationsResponse {
  campaigns: CertificationCampaign[]
}

export function fetchCertifications(): Promise<CertificationsResponse> {
  return apiRequest<CertificationsResponse>("/api/v1/certifications")
}

export function generateCertification(data: {
  campaign_name?: string
  campaign_type?: string
  created_by?: string
}): Promise<{ status: string; workflow_id: string; run_id: string }> {
  return apiRequest<{ status: string; workflow_id: string; run_id: string }>(
    "/api/v1/certifications/generate",
    { method: "POST", body: data },
  )
}

export function decideCertificationEntry(
  entryId: string,
  decision: "approved" | "revoked",
  notes = "",
): Promise<{ status: string; entry_id: string; decision: string; decided_at: string }> {
  return apiRequest<{ status: string; entry_id: string; decision: string; decided_at: string }>(
    `/api/v1/certifications/entries/${encodeURIComponent(entryId)}/decide`,
    { method: "POST", body: { decision, notes } },
  )
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

// syncHRConnector runs an HR-source sync that diffs the connector roster
// against the canonical identities table: creates new, updates moved,
// terminates removed, reinstates re-added.
export function syncHRConnector(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/sync-hr`, { method: "POST", body: {} })
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

export function getConnector(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}`)
}

export function testConnector(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/connectors/${id}/test`, { method: "POST" })
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
  return apiRequest<any>("/api/v1/access/check", { method: "QUERY", body: data })
}

export function grantAccess(data: any): Promise<any> {
  return apiRequest<any>("/api/v1/access/grant", { method: "POST", body: data })
}

export function revokeAccess(data: any): Promise<any> {
  return apiRequest<any>("/api/v1/access/revoke", { method: "POST", body: data })
}

export function requestJITAccess(data: {
  identity_id: string;
  resource_id: string;
  resource_type?: string;
  action?: string;
  duration_mins: number;
  reason: string;
}): Promise<any> {
  return apiRequest<any>("/api/v1/access/jit", { method: "POST", body: data })
}

// ─── NHI Registry / JIT Passports ──────────────────────────────

export interface NHIRecord {
  id: string
  name: string
  type: string
  status: string
  agent_card_id?: string
  owner_id?: string
  team_id?: string
  framework?: string
  parent_agent_id?: string
  is_governed?: boolean
  risk_score?: number
  deployment_environment?: string
  created_at?: string
}

export interface NHIPassport {
  id: string
  scope: string
  resource_id?: string
  grant_id?: string
  status: string
  issued_at: string
  expires_at: string
  revoked_at?: string | null
  consumed_at?: string | null
  parent_passport_id?: string
}

export function fetchNHI(): Promise<{ nhi: NHIRecord[]; total: number }> {
  return apiRequest<{ nhi: NHIRecord[]; total: number }>("/api/v1/nhi")
}

export function fetchNHIRecord(id: string): Promise<any> {
  return apiRequest<any>(`/api/v1/nhi/${id}`)
}

export function registerNHI(data: {
  name: string
  type?: string
  protocols?: string[]
  owner_id?: string
  team_id?: string
  deployment_environment?: string
  framework?: string
  capabilities?: string[]
  parent_agent_id?: string
  expires_at?: string
  attributes?: Record<string, any>
}): Promise<any> {
  return apiRequest<any>("/api/v1/nhi", { method: "POST", body: data })
}

export function fetchPassports(nhiId: string): Promise<{ passports: NHIPassport[]; total: number }> {
  return apiRequest<any>(`/api/v1/nhi/${nhiId}/passports`)
}

export function issuePassport(nhiId: string, data: {
  scope?: string
  resource_id?: string
  grant_id?: string
  ttl_minutes?: number
  issued_by?: string
}): Promise<{ passport_id: string; token: string; nhi_id: string; scope: string; status: string; issued_at: string; expires_at: string }> {
  return apiRequest<any>(`/api/v1/nhi/${nhiId}/passports`, { method: "POST", body: data })
}

export function revokePassports(nhiId: string): Promise<{ revoked: number }> {
  return apiRequest<any>(`/api/v1/nhi/${nhiId}/passports/revoke`, { method: "POST" })
}

export function consumePassport(nhiId: string, pid: string): Promise<{ status: string }> {
  return apiRequest<any>(`/api/v1/nhi/${nhiId}/passports/${pid}/consume`, { method: "POST" })
}

// ─── Events (real-time) ─────────────────────────────────────────

export interface PlatformEvent {
  id?: string
  type: string
  summary: string
  timestamp?: string
  [key: string]: any
}

export function listEvents(params?: { limit?: number }): Promise<{ events: PlatformEvent[]; total: number }> {
  const q = new URLSearchParams()
  if (params?.limit) q.set("limit", String(params.limit))
  const qs = q.toString() ? `?${q.toString()}` : ""
  return apiRequest<{ events: PlatformEvent[]; total: number }>(`/api/v1/events${qs}`).catch(() => ({ events: [], total: 0 }))
}

// ─── Firecall (Break-Glass) ─────────────────────────────────────
export function requestFirecallAccess(data: {
  identity_id: string;
  resource_id: string;
  resource_type?: string;
  action?: string;
  reason: string;
  justification: string;
  duration_mins: number;
  incident_id?: string;
  idempotency_key?: string;
}): Promise<any> {
  const headers: Record<string, string> = {}
  if (data.idempotency_key) headers["Idempotency-Key"] = data.idempotency_key
  return apiRequest<any>("/api/v1/access/firecall", {
    method: "POST",
    body: {
      identity_id: data.identity_id,
      resource_id: data.resource_id,
      resource_type: data.resource_type || "system",
      action: data.action || "admin",
      reason: data.reason,
      justification: data.justification,
      duration_mins: data.duration_mins,
      incident_id: data.incident_id || "",
    },
    headers,
  })
}

export function revokeJITSession(data: {
  identity_id: string;
  resource_id: string;
  reason?: string;
}): Promise<any> {
  return apiRequest<any>("/api/v1/access/jit/revoke", { method: "POST", body: data })
}

// ─── Workflow Request Lifecycle ──────────────────────────────────

export interface WorkflowRequest {
  id: string
  tenant_id: string
  type: string
  status: string
  requester_id: string
  target_id: string
  payload: any
  idempotency_key?: string
  temporal_workflow_id?: string
  created_at: string
  updated_at: string
  expires_at?: string
  completed_at?: string
  failure_reason?: string
}

export interface WorkflowAuditEntry {
  id: number
  request_id: string
  step: string
  actor: string
  details: any
  ts: string
}

export function listRequests(params?: {
  status?: string
  type?: string
  limit?: number
}): Promise<{ requests: WorkflowRequest[]; total: number }> {
  const q = new URLSearchParams()
  if (params?.status) q.set("status", params.status)
  if (params?.type) q.set("type", params.type)
  if (params?.limit) q.set("limit", String(params.limit))
  const qs = q.toString() ? `?${q.toString()}` : ""
  return apiRequest<{ requests: WorkflowRequest[]; total: number }>(`/api/v1/requests${qs}`)
}

export function getRequest(id: string): Promise<{
  request: WorkflowRequest
  approvals: any[]
  audit: WorkflowAuditEntry[]
}> {
  return apiRequest<any>(`/api/v1/requests/${id}`)
}

// ─── Approval Routing (Phase 2.2) ─────────────────────────

export interface Approval {
  id: string
  request_id: string
  level: number
  approver_id?: string
  approver_email: string
  approver_role: string
  status: string
  comment?: string
  decided_at?: string
  due_at?: string
  created_at: string
}

export function listPendingApprovals(params?: {
  approver_id?: string
}): Promise<{ approvals: Approval[]; total: number }> {
  const q = new URLSearchParams()
  if (params?.approver_id) q.set("approver_id", params.approver_id)
  const qs = q.toString() ? `?${q.toString()}` : ""
  return apiRequest<{ approvals: Approval[]; total: number }>(`/api/v1/approvals/queue${qs}`)
}

export function decideApproval(approvalId: string, data: {
  approver_id: string
  approved: boolean
  comment?: string
}): Promise<any> {
  return apiRequest<any>(`/api/v1/approvals/${approvalId}/decide`, { method: "POST", body: data })
}

// ─── Self-Service Catalog (Phase 2.4) ───────────────────────

export interface CatalogRole {
  id: string
  tenant_id: string
  name: string
  description: any
  role_type: string
  approval_required: boolean
  max_duration_hours: number | null
}

export function listRoleCatalog(): Promise<{ roles: CatalogRole[]; total: number }> {
  return apiRequest<{ roles: CatalogRole[]; total: number }>("/api/v1/catalog/roles")
}

export function requestRoleAccess(data: {
  identity_id: string
  role_id: string
  requested_by: string
  reason: string
  duration_hours?: number
}): Promise<any> {
  return apiRequest<any>("/api/v1/access/request-role", { method: "POST", body: data })
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
  return apiRequest<any>("/api/v1/copilot/query", { method: "QUERY", body: data })
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

// ─── GraphQL Client ─────────────────────────────────────────
// Provides GraphQL queries and mutations alongside the REST API.
// All requests go to POST /graphql on the same base URL.

const GQL_ENDPOINT = "/graphql"

interface GQLResponse<T> {
  data?: T
  errors?: Array<{ message: string; locations?: any[]; path?: string[] }>
}

async function gqlRequest<T>(
  query: string,
  variables?: Record<string, any>,
): Promise<T> {
  const base = getApiBase()

  const authHeaders: Record<string, string> = {}
  const token = getAuthToken()
  if (token) {
    authHeaders["Authorization"] = `Bearer ${token}`
  }
  const tenantId = getTenantId()
  if (tenantId) authHeaders["X-Tenant-ID"] = tenantId

  const res = await fetch(`${base}${GQL_ENDPOINT}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...authHeaders,
    },
    body: JSON.stringify({ query, variables }),
  })

  const json: GQLResponse<T> = await res.json()

  if (json.errors && json.errors.length > 0) {
    throw new Error(`GraphQL: ${json.errors.map((e) => e.message).join(", ")}`)
  }

  if (!json.data) {
    throw new Error("GraphQL: empty response")
  }

  return json.data
}

// ─── GraphQL Types ──────────────────────────────────────────

export interface GQLIdentity {
  id: string
  tenantId: string
  type: string
  status: string
  email: string
  displayName: string
  department?: string | null
  employeeId?: string | null
  managerId?: string | null
  source: string
  riskScore: number
  riskFactors: string[]
  assuranceLevel: string
  attributes: any
  createdAt: string
  updatedAt: string
}

export interface GQLNonHumanIdentity {
  id: string
  tenantId: string
  name: string
  type: string
  status: string
  agentCardId?: string | null
  protocols: string[]
  ownerId?: string | null
  teamId?: string | null
  isGoverned: boolean
  riskScore: number
  expiresAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface GQLConnector {
  id: string
  tenantId: string
  name: string
  connectorType: string
  status: string
  lastSyncAt?: string | null
  lastError?: string | null
  createdAt: string
  updatedAt: string
}

export interface GQLConnectorUser {
  externalId: string
  username?: string | null
  email?: string | null
  displayName?: string | null
  department?: string | null
  enabled: boolean
}

export interface GQLConnectorGroup {
  externalId: string
  name: string
  description?: string | null
  groupType?: string | null
}

export interface GQLConnectorHealth {
  connectorId: string
  connectorName: string
  status: string
  deltaSupported: boolean
  supportsSchema: boolean
}

export interface GQLAuditEntry {
  id: string
  timestamp: string
  level: string
  service: string
  method?: string | null
  path?: string | null
  status?: number | null
  latency?: string | null
  message: string
  sourceIp?: string | null
  tags?: string[]
}

export interface GQLHealthStatus {
  status: string
  service: string
  version: string
}

export interface GQLReadinessResult {
  status: string
  checks: { redis: string; postgres: string; neo4j: string }
}

// ─── Identity Queries ─────────────────────────────────────

const LIST_IDENTITIES_QUERY = `
  query ListIdentities($limit: Int, $offset: Int) {
    identities(limit: $limit, offset: $offset) {
      id tenantId type status email displayName department employeeId
      managerId source riskScore riskFactors assuranceLevel
      attributes createdAt updatedAt
    }
  }
`

const GET_IDENTITY_QUERY = `
  query GetIdentity($id: ID!) {
    identity(id: $id) {
      id tenantId type status email displayName department employeeId
      managerId source riskScore riskFactors assuranceLevel
      attributes createdAt updatedAt
    }
  }
`

export function gqlFetchIdentities(limit?: number, offset?: number): Promise<{ identities: GQLIdentity[] }> {
  return gqlRequest(LIST_IDENTITIES_QUERY, { limit, offset })
}

export function gqlFetchIdentity(id: string): Promise<{ identity: GQLIdentity | null }> {
  return gqlRequest(GET_IDENTITY_QUERY, { id })
}

// ─── Agent Queries ───────────────────────────────────────

const LIST_AGENTS_QUERY = `
  query ListAgents($limit: Int, $offset: Int) {
    agents(limit: $limit, offset: $offset) {
      id tenantId name type status agentCardId protocols
      ownerId teamId isGoverned riskScore expiresAt createdAt updatedAt
    }
  }
`

const GET_AGENT_QUERY = `
  query GetAgent($id: ID!) {
    agent(id: $id) {
      id tenantId name type status agentCardId protocols
      ownerId teamId isGoverned riskScore expiresAt createdAt updatedAt
    }
  }
`

export function gqlFetchAgents(limit?: number, offset?: number): Promise<{ agents: GQLNonHumanIdentity[] }> {
  return gqlRequest(LIST_AGENTS_QUERY, { limit, offset })
}

export function gqlFetchAgent(id: string): Promise<{ agent: GQLNonHumanIdentity | null }> {
  return gqlRequest(GET_AGENT_QUERY, { id })
}

// ─── Connector Queries ───────────────────────────────────

const LIST_CONNECTORS_QUERY = `
  query ListConnectors {
    connectors { id tenantId name connectorType status lastSyncAt lastError createdAt updatedAt }
  }
`

const GET_CONNECTOR_QUERY = `
  query GetConnector($id: ID!) {
    connector(id: $id) {
      id tenantId name connectorType status lastSyncAt lastError createdAt updatedAt
    }
  }
`

const CONNECTOR_USERS_QUERY = `
  query ConnectorUsers($connectorId: ID!, $limit: Int, $offset: Int) {
    connectorUsers(connectorId: $connectorId, limit: $limit, offset: $offset) {
      externalId username email displayName department enabled
    }
  }
`

const CONNECTOR_GROUPS_QUERY = `
  query ConnectorGroups($connectorId: ID!) {
    connectorGroups(connectorId: $connectorId) {
      externalId name description groupType
    }
  }
`

const CONNECTOR_HEALTH_QUERY = `
  query ConnectorHealth($connectorId: ID!) {
    connectorHealth(connectorId: $connectorId) {
      connectorId connectorName status deltaSupported supportsSchema
    }
  }
`

export function gqlFetchConnectors(): Promise<{ connectors: GQLConnector[] }> {
  return gqlRequest(LIST_CONNECTORS_QUERY)
}

export function gqlFetchConnector(id: string): Promise<{ connector: GQLConnector | null }> {
  return gqlRequest(GET_CONNECTOR_QUERY, { id })
}

export function gqlFetchConnectorUsers(connectorId: string, limit?: number, offset?: number): Promise<{ connectorUsers: GQLConnectorUser[] }> {
  return gqlRequest(CONNECTOR_USERS_QUERY, { connectorId, limit, offset })
}

export function gqlFetchConnectorGroups(connectorId: string): Promise<{ connectorGroups: GQLConnectorGroup[] }> {
  return gqlRequest(CONNECTOR_GROUPS_QUERY, { connectorId })
}

export function gqlFetchConnectorHealth(connectorId: string): Promise<{ connectorHealth: GQLConnectorHealth }> {
  return gqlRequest(CONNECTOR_HEALTH_QUERY, { connectorId })
}

// ─── Audit / Health Queries ──────────────────────────────

const AUDIT_LOGS_QUERY = `
  query AuditLogs($limit: Int, $offset: Int, $level: String, $path: String) {
    auditLogs(limit: $limit, offset: $offset, level: $level, path: $path) {
      id timestamp level service method path status latency message sourceIp tags
    }
  }
`

const HEALTH_QUERY = `
  query Health { health { status service version } }
`

const READY_QUERY = `
  query Ready { ready { status checks { redis postgres neo4j } } }
`

export function gqlFetchAuditLogs(limit?: number, offset?: number, level?: string, path?: string): Promise<{ auditLogs: GQLAuditEntry[] }> {
  return gqlRequest(AUDIT_LOGS_QUERY, { limit, offset, level, path })
}

export function gqlFetchHealth(): Promise<{ health: GQLHealthStatus }> {
  return gqlRequest(HEALTH_QUERY)
}

export function gqlFetchReady(): Promise<{ ready: GQLReadinessResult }> {
  return gqlRequest(READY_QUERY)
}

// ─── Identity Mutations ─────────────────────────────────

const CREATE_IDENTITY_MUTATION = `
  mutation CreateIdentity($input: CreateIdentityInput!) {
    createIdentity(input: $input) {
      id tenantId type status email displayName department employeeId
      managerId source riskScore riskFactors assuranceLevel
      attributes createdAt updatedAt
    }
  }
`

const UPDATE_IDENTITY_MUTATION = `
  mutation UpdateIdentity($id: ID!, $input: UpdateIdentityInput!) {
    updateIdentity(id: $id, input: $input) {
      id tenantId type status email displayName department employeeId
      managerId source riskScore riskFactors assuranceLevel
      attributes createdAt updatedAt
    }
  }
`

const DELETE_IDENTITY_MUTATION = `
  mutation DeleteIdentity($id: ID!) {
    deleteIdentity(id: $id)
  }
`

export function gqlCreateIdentity(input: {
  type: string
  email: string
  displayName: string
  department?: string | null
  employeeId?: string | null
  source?: string | null
  attributes?: any
}): Promise<{ createIdentity: GQLIdentity }> {
  return gqlRequest(CREATE_IDENTITY_MUTATION, { input })
}

export function gqlUpdateIdentity(
  id: string,
  input: {
    displayName?: string | null
    department?: string | null
    email?: string | null
    status?: string | null
    attributes?: any
  },
): Promise<{ updateIdentity: GQLIdentity }> {
  return gqlRequest(UPDATE_IDENTITY_MUTATION, { id, input })
}

export function gqlDeleteIdentity(id: string): Promise<{ deleteIdentity: boolean }> {
  return gqlRequest(DELETE_IDENTITY_MUTATION, { id })
}

// ─── Connector Mutations ────────────────────────────────

const CREATE_CONNECTOR_MUTATION = `
  mutation CreateConnector($input: CreateConnectorInput!) {
    createConnector(input: $input) {
      id tenantId name connectorType status lastSyncAt lastError createdAt updatedAt
    }
  }
`

const DELETE_CONNECTOR_MUTATION = `
  mutation DeleteConnector($id: ID!) {
    deleteConnector(id: $id)
  }
`

const CONNECT_CONNECTOR_MUTATION = `
  mutation ConnectConnector($id: ID!) {
    connectConnector(id: $id) {
      id tenantId name connectorType status lastSyncAt lastError createdAt updatedAt
    }
  }
`

const DISCONNECT_CONNECTOR_MUTATION = `
  mutation DisconnectConnector($id: ID!) {
    disconnectConnector(id: $id) {
      id tenantId name connectorType status lastSyncAt lastError createdAt updatedAt
    }
  }
`

const SYNC_CONNECTOR_MUTATION = `
  mutation SyncConnector($id: ID!) {
    syncConnector(id: $id) {
      id tenantId name connectorType status lastSyncAt lastError createdAt updatedAt
    }
  }
`

const SYNC_CONNECTOR_DELTA_MUTATION = `
  mutation SyncConnectorDelta($id: ID!) {
    syncConnectorDelta(id: $id) {
      id tenantId name connectorType status lastSyncAt lastError createdAt updatedAt
    }
  }
`

export function gqlCreateConnector(input: {
  name: string
  connectorType: string
  config: any
}): Promise<{ createConnector: GQLConnector }> {
  return gqlRequest(CREATE_CONNECTOR_MUTATION, { input })
}

export function gqlDeleteConnector(id: string): Promise<{ deleteConnector: boolean }> {
  return gqlRequest(DELETE_CONNECTOR_MUTATION, { id })
}

export function gqlConnectConnector(id: string): Promise<{ connectConnector: GQLConnector }> {
  return gqlRequest(CONNECT_CONNECTOR_MUTATION, { id })
}

export function gqlDisconnectConnector(id: string): Promise<{ disconnectConnector: GQLConnector }> {
  return gqlRequest(DISCONNECT_CONNECTOR_MUTATION, { id })
}

export function gqlSyncConnector(id: string): Promise<{ syncConnector: GQLConnector }> {
  return gqlRequest(SYNC_CONNECTOR_MUTATION, { id })
}

export function gqlSyncConnectorDelta(id: string): Promise<{ syncConnectorDelta: GQLConnector }> {
  return gqlRequest(SYNC_CONNECTOR_DELTA_MUTATION, { id })
}
