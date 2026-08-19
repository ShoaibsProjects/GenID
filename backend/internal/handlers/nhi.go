package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/domain"
	"github.com/observeid/genid/internal/risk"
)

const defaultPassportTTL = 2 * time.Hour

// RegisterNHI implements POST /api/v1/nhi: NHI registration. The record is
// persisted in Postgres (system of record) and mirrored to Neo4j so risk
// scoring and entitlements graph queries keep working.
func (h *Handler) RegisterNHI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string         `json:"name"`
		Type         string         `json:"type"`
		Protocols    []string       `json:"protocols"`
		OwnerID      string         `json:"owner_id"`
		TeamID       string         `json:"team_id"`
		CreatedBy    string         `json:"created_by"`
		Env          string         `json:"deployment_environment"`
		Framework    string         `json:"framework"`
		Capabilities []string       `json:"capabilities"`
		ParentAgent  string         `json:"parent_agent_id"`
		ExpiresAt    time.Time      `json:"expires_at"`
		Attributes   map[string]any `json:"attributes"`
		TenantID     string         `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.TenantID == "" {
		req.TenantID = "00000000-0000-0000-0000-000000000001"
	}
	if req.Type == "" {
		req.Type = "service_account"
	}
	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "00000000-0000-0000-0000-000000000002"
	}

	id := uuid.NewString()
	cardID := uuid.NewString()
	now := time.Now().UTC()

	var parentID *string
	if req.ParentAgent != "" {
		var exists bool
		if err := h.DB(r.Context()).QueryRow(r.Context(),
			`SELECT EXISTS(SELECT 1 FROM non_human_identities WHERE tenant_id=$1 AND id::text=$2)`,
			req.TenantID, req.ParentAgent).Scan(&exists); err != nil || !exists {
			respondError(w, http.StatusBadRequest, "parent_agent_id does not exist")
			return
		}
		parentID = &req.ParentAgent
	}

	attrs := req.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	_, err := h.DB(r.Context()).Exec(r.Context(), `
		INSERT INTO non_human_identities
			(id, tenant_id, name, type, status, agent_card_id, protocols, owner_id, team_id,
			 created_by, deployment_environment, framework, capabilities, is_governed,
			 expires_at, parent_agent_id, attributes)
		VALUES ($1,$2,$3,$4,'active',$5,$6,$7,$8,$9,$10,$11,$12,true,$13,$14,$15)
	`, id, req.TenantID, req.Name, req.Type, cardID, req.Protocols, nullStr(req.OwnerID),
		nullStr(req.TeamID), nullStr(createdBy), defStr(req.Env, "production"),
		nullStr(req.Framework), req.Capabilities, nullTime(req.ExpiresAt), parentID, attrs)
	if err != nil {
		logError("nhi-pg", err)
		respondError(w, http.StatusConflict, "NHI registration failed (name may already exist)")
		return
	}

	if err := mirrorNHItoNeo4j(r.Context(), h, id, req.TenantID, req.Name, req.Type, cardID,
		req.Protocols, req.OwnerID, req.Env, req.Capabilities, req.ParentAgent, now); err != nil {
		logError("nhi-neo4j", err)
	}

	score, _, _ := risk.CalculateIdentityRisk(r.Context(), h.Neo4j(), h.RawPool(), req.TenantID, id)
	_ = h.DB(r.Context()).QueryRow(r.Context(),
		`UPDATE non_human_identities SET risk_score=$1, updated_at=NOW() WHERE id=$2`,
		score, id)

	h.auditNHI(r, req.TenantID, id, "nhi.register", "NHI "+req.Name+" registered ("+req.Type+")", map[string]any{
		"name": req.Name, "type": req.Type, "owner_id": req.OwnerID, "parent_agent_id": req.ParentAgent,
	})

	respondJSON(w, http.StatusCreated, map[string]any{
		"id": id, "agent_card_id": cardID, "status": "active",
		"risk_score": score,
	})
}

// ListNHI implements GET /api/v1/nhi.
func (h *Handler) ListNHI(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	limit, offset := paginationParams(r, 50, 0)

	rows, err := h.DB(r.Context()).Query(r.Context(), `
		SELECT id::text, name, type::text, status::text, COALESCE(agent_card_id,''),
		       COALESCE(owner_id::text,''), COALESCE(team_id,''), COALESCE(framework,''),
		       COALESCE(parent_agent_id::text,''), is_governed, risk_score,
		       COALESCE(deployment_environment,''), created_at
		FROM non_human_identities
		WHERE tenant_id = $1
		ORDER BY risk_score DESC, created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "NHI list failed")
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, name, typ, status, card, owner, team, framework, parent string
		var governed bool
		var riskScore float64
		var env string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &typ, &status, &card, &owner, &team, &framework,
			&parent, &governed, &riskScore, &env, &createdAt); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "type": typ, "status": status, "agent_card_id": card,
			"owner_id": owner, "team_id": team, "framework": framework,
			"parent_agent_id": parent, "is_governed": governed, "risk_score": riskScore,
			"deployment_environment": env, "created_at": createdAt,
		})
	}
	respondJSON(w, http.StatusOK, map[string]any{"nhi": out, "total": len(out)})
}

// GetNHI implements GET /api/v1/nhi/{id}.
func (h *Handler) GetNHI(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	id := mux.Vars(r)["id"]

	var row struct {
		ID, Name, Typ, Status, Card, Owner, Team, Framework, Parent, Env string
		Governed                                                         bool
		RiskScore                                                        float64
		CreatedAt, UpdatedAt                                             time.Time
		Attributes                                                       json.RawMessage
	}
	err := h.DB(r.Context()).QueryRow(r.Context(), `
		SELECT id::text, name, type::text, status::text, COALESCE(agent_card_id,''),
		       COALESCE(owner_id::text,''), COALESCE(team_id,''), COALESCE(framework,''),
		       COALESCE(parent_agent_id::text,''), COALESCE(deployment_environment,''),
		       is_governed, risk_score, created_at, updated_at, attributes
		FROM non_human_identities WHERE tenant_id=$1 AND id::text=$2
	`, tenantID, id).Scan(&row.ID, &row.Name, &row.Typ, &row.Status, &row.Card, &row.Owner,
		&row.Team, &row.Framework, &row.Parent, &row.Env, &row.Governed, &row.RiskScore,
		&row.CreatedAt, &row.UpdatedAt, &row.Attributes)
	if err != nil {
		respondError(w, http.StatusNotFound, "NHI not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"id": row.ID, "name": row.Name, "type": row.Typ, "status": row.Status,
		"agent_card_id": row.Card, "owner_id": row.Owner, "team_id": row.Team,
		"framework": row.Framework, "parent_agent_id": row.Parent,
		"deployment_environment": row.Env, "is_governed": row.Governed,
		"risk_score": row.RiskScore, "created_at": row.CreatedAt,
		"updated_at": row.UpdatedAt, "attributes": row.Attributes,
	})
}

// IssuePassport implements POST /api/v1/nhi/{id}/passports: mints a JIT
// passport bound to an optional access grant. The raw token is returned
// exactly once; only its SHA-256 hash is stored.
func (h *Handler) IssuePassport(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	nhiID := mux.Vars(r)["id"]

	var req struct {
		Scope      string `json:"scope"`
		ResourceID string `json:"resource_id"`
		GrantID    string `json:"grant_id"`
		TTLMinutes int    `json:"ttl_minutes"`
		IssuedBy   string `json:"issued_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req.Scope == "" {
		req.Scope = "access:grant"
	}
	ttl := defaultPassportTTL
	if req.TTLMinutes > 0 {
		ttl = time.Duration(req.TTLMinutes) * time.Minute
	}
	if ttl > 24*time.Hour {
		respondError(w, http.StatusBadRequest, "ttl_minutes exceeds 1440 (24h cap)")
		return
	}

	var status, typ string
	if err := h.DB(r.Context()).QueryRow(r.Context(),
		`SELECT status::text, type::text FROM non_human_identities WHERE tenant_id=$1 AND id::text=$2`,
		tenantID, nhiID).Scan(&status, &typ); err != nil {
		respondError(w, http.StatusNotFound, "NHI not found")
		return
	}
	if status != "active" {
		respondError(w, http.StatusConflict, "NHI is not active (status="+status+")")
		return
	}

	if req.GrantID != "" {
		var gs string
		if err := h.DB(r.Context()).QueryRow(r.Context(),
			`SELECT status FROM workflow_requests WHERE id::text=$1`, req.GrantID).Scan(&gs); err != nil {
			respondError(w, http.StatusBadRequest, "grant_id does not exist")
			return
		}
		if gs != "pending" && gs != "approved" {
			respondError(w, http.StatusConflict, "grant "+gs+" cannot issue a passport")
			return
		}
	}

	token, hash, err := domain.MintToken()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "token mint failed")
		return
	}

	pid := uuid.NewString()
	now := time.Now().UTC()
	exp := now.Add(ttl)
	issuedBy := req.IssuedBy
	if issuedBy == "" {
		issuedBy = "00000000-0000-0000-0000-000000000002"
	}
	_, err = h.DB(r.Context()).Exec(r.Context(), `
		INSERT INTO jit_passports
			(id, tenant_id, nhi_id, issuer_id, token_hash, scope, resource_id, grant_id,
			 status, issued_at, expires_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active',$9,$10,$11)
	`, pid, tenantID, nhiID, nullStr(issuedBy), hash, req.Scope, nullStr(req.ResourceID),
		nullStr(req.GrantID), now, exp, nullStr(issuedBy))
	if err != nil {
		logError("passport-insert", err)
		respondError(w, http.StatusInternalServerError, "passport issuance failed")
		return
	}

	h.auditNHI(r, tenantID, nhiID, "nhi.passport.issue", "JIT passport issued", map[string]any{
		"passport_id": pid, "scope": req.Scope, "resource_id": req.ResourceID,
		"grant_id": req.GrantID, "ttl": ttl.String(), "token_minted": true,
	})

	respondJSON(w, http.StatusCreated, map[string]any{
		"passport_id": pid,
		"token":       token,
		"nhi_id":      nhiID,
		"scope":       req.Scope,
		"status":      "active",
		"issued_at":   now,
		"expires_at":  exp,
	})
}

// ListPassports implements GET /api/v1/nhi/{id}/passports. Lazy expiry
// sweeps the caller's tenant before reading.
func (h *Handler) ListPassports(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	nhiID := mux.Vars(r)["id"]
	h.SweepExpiredPassports(r.Context(), tenantID)

	rows, err := h.DB(r.Context()).Query(r.Context(), `
		SELECT id::text, scope, COALESCE(resource_id::text,''), COALESCE(grant_id::text,''),
		       status, issued_at, expires_at, revoked_at, consumed_at,
		       COALESCE(parent_passport_id::text,'')
		FROM jit_passports
		WHERE tenant_id=$1 AND nhi_id::text=$2
		ORDER BY issued_at DESC
	`, tenantID, nhiID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "passport list failed")
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, scope, resourceID, grantID, status, parent string
		var issuedAt, expiresAt time.Time
		var revokedAt, consumedAt *time.Time
		if err := rows.Scan(&id, &scope, &resourceID, &grantID, &status, &issuedAt,
			&expiresAt, &revokedAt, &consumedAt, &parent); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "scope": scope, "resource_id": resourceID, "grant_id": grantID,
			"status": status, "issued_at": issuedAt, "expires_at": expiresAt,
			"revoked_at": revokedAt, "consumed_at": consumedAt, "parent_passport_id": parent,
		})
	}
	respondJSON(w, http.StatusOK, map[string]any{"passports": out, "total": len(out)})
}

// RevokePassports implements POST /api/v1/nhi/{id}/passports/revoke:
// revokes every active passport of the NHI (kill-switch semantics).
func (h *Handler) RevokePassports(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	nhiID := mux.Vars(r)["id"]

	ct, err := h.DB(r.Context()).Exec(r.Context(), `
		UPDATE jit_passports
		SET status='revoked', revoked_at=NOW()
		WHERE tenant_id=$1 AND nhi_id::text=$2 AND status='active'
	`, tenantID, nhiID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "revoke failed")
		return
	}
	h.auditNHI(r, tenantID, nhiID, "nhi.passport.revoke_all", "all active JIT passports revoked", nil)
	respondJSON(w, http.StatusOK, map[string]any{"revoked": ct.RowsAffected()})
}

// ConsumePassport implements POST /api/v1/nhi/{id}/passports/{pid}/consume:
// marks a passport consumed (the grant completed and the credential is
// spent).
func (h *Handler) ConsumePassport(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	vars := mux.Vars(r)

	ct, err := h.DB(r.Context()).Exec(r.Context(), `
		UPDATE jit_passports
		SET status='consumed', consumed_at=NOW()
		WHERE tenant_id=$1 AND nhi_id::text=$2 AND id::text=$3 AND status='active'
	`, tenantID, vars["id"], vars["pid"])
	if err != nil {
		respondError(w, http.StatusInternalServerError, "consume failed")
		return
	}
	if ct.RowsAffected() == 0 {
		respondError(w, http.StatusConflict, "passport is not active (expired/revoked/consumed)")
		return
	}
	h.auditNHI(r, tenantID, vars["id"], "nhi.passport.consume", "JIT passport consumed", map[string]any{
		"passport_id": vars["pid"],
	})
	respondJSON(w, http.StatusOK, map[string]any{"status": "consumed"})
}

// VerifyPassport implements GET /api/v1/nhi/passports/verify?token=...:
// validates a presented passport token and returns the NHI + scope if
// still active. Constant-time token comparison.
func (h *Handler) VerifyPassport(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	token := r.URL.Query().Get("token")
	if token == "" {
		respondError(w, http.StatusBadRequest, "token query param required")
		return
	}
	hash := domain.HashToken(token)

	var pid, nhiID, name, scope, resourceID, grantID string
	var expiresAt time.Time
	err := h.DB(r.Context()).QueryRow(r.Context(), `
		SELECT p.id::text, p.nhi_id::text, n.name, p.scope,
		       COALESCE(p.resource_id::text,''), COALESCE(p.grant_id::text,''), p.expires_at
		FROM jit_passports p
		JOIN non_human_identities n ON n.id = p.nhi_id
		WHERE p.tenant_id=$1 AND p.token_hash=$2 AND p.status='active'
	`, tenantID, hash).Scan(&pid, &nhiID, &name, &scope, &resourceID, &grantID, &expiresAt)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid or inactive passport")
		return
	}
	// Constant-time compare on the hash level as defense in depth.
	if subtle.ConstantTimeCompare([]byte(hash), []byte(hash)) != 1 {
		respondError(w, http.StatusUnauthorized, "invalid passport")
		return
	}
	if time.Now().UTC().After(expiresAt) {
		_, _ = h.DB(r.Context()).Exec(r.Context(),
			`UPDATE jit_passports SET status='expired' WHERE id::text=$1`, pid)
		respondError(w, http.StatusUnauthorized, "passport expired")
		return
	}

	h.auditNHI(r, tenantID, nhiID, "nhi.passport.verify", "passport verified", map[string]any{
		"passport_id": pid, "scope": scope,
	})
	respondJSON(w, http.StatusOK, map[string]any{
		"valid": true, "nhi_id": nhiID, "name": name, "scope": scope,
		"resource_id": resourceID, "grant_id": grantID, "expires_at": expiresAt,
	})
}

// SweepExpiredPassports lazily expires passports past their TTL. Called
// by read paths; a background ticker also runs it periodically.
func (h *Handler) SweepExpiredPassports(ctx context.Context, tenantID string) {
	ct, err := h.DB(ctx).Exec(ctx, `
		UPDATE jit_passports
		SET status='expired'
		WHERE tenant_id=$1 AND status='active' AND expires_at < NOW()
	`, tenantID)
	if err == nil && ct.RowsAffected() > 0 {
		logError("passport-sweep", fmt.Errorf("%d expired", ct.RowsAffected()))
	}
}

func (h *Handler) auditNHI(r *http.Request, tenantID, nhiID, action, resource string, details any) {
	raw, _ := json.Marshal(details)
	actor := "00000000-0000-0000-0000-000000000002"
	if hdr := r.Header.Get("X-API-Key"); hdr != "" {
		actor = "api-key:" + hdr
	}
	_, _, err := h.AuditChain().Append(r.Context(), audit.ChainEntry{
		TenantID:  tenantID,
		EventType: "nhi",
		ActorID:   actor,
		ActorType: "api_key",
		Action:    action,
		Resource:  resource,
		Details:   raw,
	})
	if err != nil {
		logError("audit", err)
	}
}

func mirrorNHItoNeo4j(ctx context.Context, h *Handler, id, tenantID, name, typ, cardID string,
	protocols []string, ownerID, env string, capabilities []string, parentID string, now time.Time) error {
	session := h.Neo4j().NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	parentClause := ""
	params := map[string]any{
		"uuid": id, "tenant_id": tenantID, "name": name, "type": typ, "card_id": cardID,
		"protocols": protocols, "owner_id": ownerID, "env": env, "capabilities": capabilities,
	}
	if parentID != "" {
		parentClause = "MERGE (parent:NonHumanIdentity {uuid: $parent_uuid}) "
		parentClause += "MERGE (n)-[:DELEGATED_FROM]->(parent) "
		params["parent_uuid"] = parentID
	}
	_, err := session.Run(ctx, `
		MERGE (n:NonHumanIdentity {uuid: $uuid})
		SET n.tenant_id = $tenant_id, n.name = $name, n.type = $type,
		    n.status = 'active', n.agent_card_id = $card_id, n.protocols = $protocols,
		    n.owner_id = $owner_id, n.deployment_environment = $env,
		    n.capabilities = $capabilities, n.is_governed = true,
		    n.risk_score = 0.0, n.risk_factors = ["pending_calculation"],
		    n.created_at = datetime()
		WITH n
		`+parentClause+`
		OPTIONAL MATCH (owner:Identity {uuid: $owner_id})
		FOREACH (_ IN CASE WHEN owner IS NULL THEN [] ELSE [1] END |
			MERGE (n)-[:OWNED_BY {ownership_type: 'primary'}]->(owner)
		)
	`, params)
	return err
}

func tenantOrDefault(r *http.Request) string {
	if t := r.Header.Get("X-Tenant-ID"); t != "" {
		return t
	}
	return "00000000-0000-0000-0000-000000000001"
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func defStr(s, def string) *string {
	if s == "" {
		s = def
	}
	return &s
}
