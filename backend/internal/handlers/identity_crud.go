package handlers

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/middleware"
	"net/http"
)

func (h *Handler) CreateIdentityRecord(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email       string            `json:"email"`
		DisplayName string            `json:"display_name"`
		FirstName   string            `json:"first_name"`
		LastName    string            `json:"last_name"`
		Type        string            `json:"type"`
		Status      string            `json:"status"`
		Department  string            `json:"department"`
		Title       string            `json:"title"`
		EmployeeID  string            `json:"employee_id"`
		ManagerID   string            `json:"manager_id"`
		Source      string            `json:"source"`
		TenantID    string            `json:"tenant_id"`
		Phone       string            `json:"phone"`
		Attributes  map[string]string `json:"attributes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if input.Email == "" || input.DisplayName == "" {
		respondError(w, http.StatusBadRequest, "email and display_name are required")
		return
	}
	if input.Type == "" {
		input.Type = "human"
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if input.Source == "" {
		input.Source = "manual"
	}
	if input.TenantID == "" || input.TenantID == "default" {
		// Derive tenant from X-Tenant-ID header (preferred) or JWT claim.
		if hdrTenant := r.Header.Get("X-Tenant-ID"); hdrTenant != "" {
			input.TenantID = hdrTenant
		} else if txTenant := middleware.TenantIDFromContext(r.Context()); txTenant != "" {
			input.TenantID = txTenant
		} else {
			input.TenantID = "00000000-0000-0000-0000-000000000001"
		}
	}

	id := uuid.New().String()

	// Handle nullable UUID fields
	var managerID interface{}
	if input.ManagerID != "" {
		managerID = input.ManagerID
	}

	// 1. Write to PostgreSQL (RETURNING id to handle ON CONFLICT returning existing row)
	var returnedID string
	var attrsJSON []byte
	if input.Attributes != nil {
		attrsJSON, _ = json.Marshal(input.Attributes)
	}
	if attrsJSON == nil {
		attrsJSON = []byte("{}")
	}
	err := h.DB(r.Context()).QueryRow(r.Context(), `
		INSERT INTO identities (id, tenant_id, type, status, email, display_name, department, employee_id, manager_id, source, risk_score, attributes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (tenant_id, email) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			department   = EXCLUDED.department,
			employee_id  = EXCLUDED.employee_id,
			status       = 'active',
			updated_at   = NOW()
		RETURNING id
	`, id, input.TenantID, input.Type, input.Status, input.Email, input.DisplayName,
		input.Department, input.EmployeeID, managerID, input.Source, 0.0, attrsJSON).Scan(&returnedID)
	if err != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "identity", Path: r.URL.Path,
			Message: fmt.Sprintf("PG create failed: %s", err.Error()),
			Tags:    []string{"identity", "create", "error"},
		})
		respondError(w, http.StatusInternalServerError, "Failed to persist identity to database")
		return
	}
	// Use the actual id from the database (handles ON CONFLICT returning existing row)
	id = returnedID

	// 2. Write to Neo4j (graph)
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err = session.Run(r.Context(), `
		MERGE (i:Identity {uuid: $uuid})
		SET i.tenant_id = $tenant_id, i.type = $type, i.status = 'active',
		    i.email = $email, i.display_name = $display_name,
		    i.first_name = $first_name, i.last_name = $last_name,
		    i.department = $department, i.title = $title,
		    i.employee_id = $employee_id, i.manager_id = $manager_id,
		    i.source = $source, i.phone = $phone,
		    i.risk_score = 0.0, i.risk_factors = ["pending_calculation"],
		    i.updated_at = datetime(),
		    i.created_at = COALESCE(i.created_at, datetime())
	`, map[string]any{
		"uuid": id, "tenant_id": input.TenantID, "type": input.Type,
		"email": input.Email, "display_name": input.DisplayName,
		"first_name": input.FirstName, "last_name": input.LastName,
		"department": input.Department, "title": input.Title,
		"employee_id": input.EmployeeID, "manager_id": input.ManagerID,
		"source": input.Source, "phone": input.Phone,
	})
	if err != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "identity", Path: r.URL.Path,
			Message: fmt.Sprintf("Neo4j create failed (PG written, id=%s): %s", id, err.Error()),
			Tags:    []string{"identity", "create", "neo4j", "error"},
		})
	}

	h.AuditStore().Append(audit.Entry{
		Level: audit.LevelInfo, Service: "identity", Path: r.URL.Path,
		Message: fmt.Sprintf("Created identity: %s (%s)", input.DisplayName, input.Email),
		Tags:    []string{"identity", "create", "success"},
	})

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":           id,
		"status":       "created",
		"email":        input.Email,
		"display_name": input.DisplayName,
		"type":         input.Type,
	})
}

func (h *Handler) UpdateIdentityRecord(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	// Build Neo4j SET clause with property name validation
	setClauses := ""
	pgSet := ""
	pgParams := []any{id}
	pgIdx := 2
	params := map[string]any{"uuid": id}
	allowedKeys := map[string]string{
		"display_name": "display_name", "first_name": "first_name", "last_name": "last_name",
		"email": "email", "department": "department", "status": "status",
		"type": "type", "title": "title", "manager_id": "manager_id",
		"phone": "phone", "risk_score": "risk_score",
	}
	for key, val := range updates {
		dbCol, ok := allowedKeys[key]
		if !ok {
			continue
		}
		paramKey := "p_" + key
		setClauses += fmt.Sprintf("i.%s = $%s, ", key, paramKey)
		params[paramKey] = val
		pgSet += fmt.Sprintf("%s = $%d, ", dbCol, pgIdx)
		pgParams = append(pgParams, val)
		pgIdx++
	}

	if setClauses == "" {
		respondJSON(w, http.StatusOK, map[string]string{"status": "no_changes"})
		return
	}
	setClauses += "i.updated_at = datetime()"

	// Update Neo4j
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	query := fmt.Sprintf("MATCH (i:Identity {uuid: $uuid}) SET %s", setClauses)
	_, err := session.Run(r.Context(), query, params)

	// Also update PostgreSQL (same fields)
	pgSet += "updated_at = NOW()"
	if _, errUpdate := h.DB(r.Context()).Exec(r.Context(), fmt.Sprintf(`
		UPDATE identities SET %s WHERE id = $1
	`, pgSet), pgParams...); errUpdate != nil {
		logError("postgres", fmt.Errorf("update failed: %w", errUpdate))
	}

	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to update identity")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) DeleteIdentityRecord(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	// Soft-delete in PostgreSQL
	if _, err := h.DB(r.Context()).Exec(r.Context(), `
		UPDATE identities SET status = 'terminated', updated_at = NOW() WHERE id = $1
	`, id); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to delete from database")
		return
	}

	// Soft-delete in Neo4j
	session := h.Neo4j().NewSession(r.Context(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(r.Context())

	_, err := session.Run(r.Context(), `
		MATCH (i:Identity {uuid: $uuid})
		SET i.status = 'terminated', i.updated_at = datetime()
	`, map[string]any{"uuid": id})
	if err != nil {
		logError("neo4j", fmt.Errorf("delete failed: %w", err))
	}

	h.AuditStore().Append(audit.Entry{
		Level: audit.LevelInfo, Service: "identity", Path: r.URL.Path,
		Message: fmt.Sprintf("Deleted identity: %s", id),
		Tags:    []string{"identity", "delete"},
	})

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ─── Bulk Import Handler ──────────────────────────────────

func (h *Handler) BulkImportIdentities(w http.ResponseWriter, r *http.Request) {
	type ImportRec struct {
		Email       string `json:"email"`
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
		Department  string `json:"department"`
		Title       string `json:"title"`
		EmployeeID  string `json:"employee_id"`
		Source      string `json:"source"`
		TenantID    string `json:"tenant_id"`
	}
	var req struct {
		Records []ImportRec `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request: expected JSON with 'records' array")
		return
	}
	if len(req.Records) == 0 {
		respondError(w, http.StatusBadRequest, "No records provided")
		return
	}
	if len(req.Records) > 5000 {
		req.Records = req.Records[:5000]
	}

	created, updated, failed := 0, 0, 0
	var errs []string

	for i, rec := range req.Records {
		if rec.Email == "" || rec.DisplayName == "" {
			failed++
			errs = append(errs, fmt.Sprintf("row %d: missing email or display_name", i+1))
			continue
		}
		if rec.Type == "" {
			rec.Type = "human"
		}
		if rec.Source == "" {
			rec.Source = "hris"
		}
		if rec.TenantID == "" {
			if hdrTenant := r.Header.Get("X-Tenant-ID"); hdrTenant != "" {
				rec.TenantID = hdrTenant
			} else if txTenant := middleware.TenantIDFromContext(r.Context()); txTenant != "" {
				rec.TenantID = txTenant
			} else {
				rec.TenantID = "00000000-0000-0000-0000-000000000001"
			}
		}

		id := uuid.New().String()
		tag, err := h.DB(r.Context()).Exec(r.Context(), `
			INSERT INTO identities (id, tenant_id, type, status, email, display_name, department, employee_id, source)
			VALUES ($1,$2,$3,'active',$4,$5,$6,$7,$8)
			ON CONFLICT (tenant_id, email) DO UPDATE SET
				display_name=EXCLUDED.display_name, department=EXCLUDED.department,
				employee_id=EXCLUDED.employee_id, status='active', updated_at=NOW()
		`, id, rec.TenantID, rec.Type, rec.Email, rec.DisplayName, rec.Department, rec.EmployeeID, rec.Source)

		if err != nil {
			failed++
			errs = append(errs, fmt.Sprintf("row %d (%s): %s", i+1, rec.Email, err.Error()))
			continue
		}
		if tag.Insert() {
			created++
		} else {
			updated++
		}
	}

	h.AuditStore().Append(audit.Entry{
		Level: audit.LevelInfo, Service: "identity", Path: r.URL.Path,
		Message: fmt.Sprintf("Bulk import: %d created, %d updated, %d failed", created, updated, failed),
		Tags:    []string{"identity", "bulk", "import"},
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"status": "completed", "created": created, "updated": updated,
		"failed": failed, "total": len(req.Records), "errors": errs,
	})
}

// ─── CSV Import/Export ────────────────────────────────────────
