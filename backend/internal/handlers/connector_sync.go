package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/connector"
	"github.com/observeid/genid/internal/stores"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (h *Handler) SyncConnectorGroups(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	groups, err := h.ConnectorManager().SyncGroups(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_failed",
			"error":  err.Error(),
		})
		return
	}

	tenantID, tenantErr := h.ConnectorTenantID(id)
	var created, updated int
	var persistErr error
	if tenantErr != nil {
		persistErr = tenantErr
	} else {
		created, updated, persistErr = h.Store().PersistSyncedGroups(r.Context(), tenantID, id, groups)
	}
	if persistErr != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Group sync persistence error: %s", persistErr.Error()),
			Tags:    []string{"connector", "group-sync", "error"},
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":         "groups_sync_completed",
		"groups_created": created,
		"groups_updated": updated,
		"groups_total":   len(groups),
		"connector_id":   id,
	})
}

func (h *Handler) SyncConnectorEntitlements(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	entitlements, err := h.ConnectorManager().SyncEntitlements(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_failed",
			"error":  err.Error(),
		})
		return
	}

	tenantID, tenantErr := h.ConnectorTenantID(id)
	if tenantErr != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Entitlement sync persistence error: %s", tenantErr.Error()),
			Tags:    []string{"connector", "entitlement-sync", "error"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"status":       "sync_completed_with_errors",
			"connector_id": id,
			"error":        tenantErr.Error(),
		})
		return
	}

	if err := h.Store().PersistSyncedEntitlements(r.Context(), tenantID, id, entitlements); err != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Entitlement sync persistence error: %s", err.Error()),
			Tags:    []string{"connector", "entitlement-sync", "error"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"status":       "sync_completed_with_errors",
			"connector_id": id,
			"error":        err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":             "entitlements_sync_completed",
		"entitlements_total": len(entitlements),
		"connector_id":       id,
	})
}

func (h *Handler) SyncConnectorResources(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	resources, err := h.ConnectorManager().SyncResources(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_failed",
			"error":  err.Error(),
		})
		return
	}

	tenantID, tenantErr := h.ConnectorTenantID(id)
	if tenantErr != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Resource sync persistence error: %s", tenantErr.Error()),
			Tags:    []string{"connector", "resource-sync", "error"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"status":       "sync_completed_with_errors",
			"connector_id": id,
			"error":        tenantErr.Error(),
		})
		return
	}

	if err := h.Store().PersistSyncedResources(r.Context(), tenantID, id, resources); err != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Resource sync persistence error: %s", err.Error()),
			Tags:    []string{"connector", "resource-sync", "error"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"status":       "sync_completed_with_errors",
			"connector_id": id,
			"error":        err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":          "resources_sync_completed",
		"resources_total": len(resources),
		"connector_id":    id,
	})
}

// ─── CSV Upload ────────────────────────────────────────────────

func (h *Handler) CSVUpload(w http.ResponseWriter, r *http.Request) {
	const maxCSVSize = 20 << 20

	var req struct {
		Name     string `json:"name"`
		CSVData  string `json:"csv_data"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	if req.CSVData == "" {
		respondError(w, http.StatusBadRequest, "csv_data field is required")
		return
	}
	if len(req.CSVData) > maxCSVSize {
		respondError(w, http.StatusBadRequest, "CSV data exceeds maximum size")
		return
	}

	connectorName := req.Name
	if connectorName == "" {
		connectorName = "CSV Import"
	}

	uploadDir := os.Getenv("CSV_UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "/tmp/genid-csv-uploads"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to create upload directory")
		return
	}

	savePath := filepath.Join(uploadDir, uuid.New().String()+".csv")
	if err := os.WriteFile(savePath, []byte(req.CSVData), 0600); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save file")
		return
	}

	// Validate the CSV can be parsed
	cfg := connector.ConnectorConfig{
		Name:     connectorName,
		Type:     connector.ConnectorTypeCSV,
		Endpoint: savePath,
		Status:   connector.ConnectorStatusConnected,
	}
	if req.TenantID != "" {
		cfg.TenantID = req.TenantID
	}

	// Quick test parse before registering
	tmpConn := connector.NewCSVConnector()
	if err := tmpConn.Configure(cfg); err != nil {
		os.Remove(savePath)
		respondError(w, http.StatusBadRequest, "Invalid connector config: "+err.Error())
		return
	}
	if users, err := tmpConn.ListUsers(r.Context()); err != nil {
		os.Remove(savePath)
		respondError(w, http.StatusBadRequest, "CSV parse error: "+err.Error())
		return
	} else if len(users) == 0 {
		os.Remove(savePath)
		respondError(w, http.StatusBadRequest, "CSV file has no valid user rows")
		return
	}

	id, err := h.ConnectorManager().Register(r.Context(), cfg)
	if err != nil {
		os.Remove(savePath)
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.AuditStore().Append(audit.Entry{
		Level:   audit.LevelInfo,
		Service: "connector",
		Path:    r.URL.Path,
		Message: fmt.Sprintf("CSV connector created: %s", connectorName),
		Tags:    []string{"connector", "csv", "upload"},
	})

	respondJSON(w, http.StatusCreated, map[string]any{
		"connector_id":   id,
		"connector_name": connectorName,
		"status":         "registered",
	})
}

func (h *Handler) SyncConnectorPermissions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	permissions, err := h.ConnectorManager().SyncPermissions(r.Context(), id)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "sync_failed",
			"error":  err.Error(),
		})
		return
	}

	tenantID, tenantErr := h.ConnectorTenantID(id)
	if tenantErr != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Permission sync persistence error: %s", tenantErr.Error()),
			Tags:    []string{"connector", "permission-sync", "error"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"status":       "sync_completed_with_errors",
			"connector_id": id,
			"error":        tenantErr.Error(),
		})
		return
	}

	if err := h.Store().PersistSyncedPermissions(r.Context(), tenantID, id, permissions); err != nil {
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelError, Service: "connector", Path: r.URL.Path,
			Message: fmt.Sprintf("Permission sync persistence error: %s", err.Error()),
			Tags:    []string{"connector", "permission-sync", "error"},
		})
		respondJSON(w, http.StatusOK, map[string]any{
			"status":       "sync_completed_with_errors",
			"connector_id": id,
			"error":        err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":           "permissions_sync_completed",
		"permissions_total": len(permissions),
		"connector_id":     id,
	})
}

// ─── Full Sync ────────────────────────────────────────────────

func (h *Handler) FullSyncConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	start := time.Now()
	var syncErrors []string

	tenantID, tenantErr := h.ConnectorTenantID(id)

	// 1. Sync users
	usersResult, err := h.ConnectorManager().SyncUsers(r.Context(), id)
	if err != nil {
		log.Printf("[SYNC] User sync error: %v", err)
		syncErrors = append(syncErrors, fmt.Sprintf("users: %v", err))
	}
	usersCreated, usersUpdated := 0, 0
	if usersResult != nil && len(usersResult.Users) > 0 {
		usersCreated, usersUpdated, err = h.Store().PersistSyncedUsers(r.Context(), tenantID, id, usersResult.Users)
		if tenantErr != nil {
			err = tenantErr
		}
		if err != nil {
			log.Printf("[SYNC] User persist error: %v", err)
			syncErrors = append(syncErrors, fmt.Sprintf("users persist: %v", err))
		}
	}

	// 2. Sync groups
	groups, err := h.ConnectorManager().SyncGroups(r.Context(), id)
	if err != nil {
		log.Printf("[SYNC] Group sync error: %v", err)
		syncErrors = append(syncErrors, fmt.Sprintf("groups: %v", err))
	}
	groupsCreated, groupsUpdated := 0, 0
	if len(groups) > 0 {
		groupsCreated, groupsUpdated, err = h.Store().PersistSyncedGroups(r.Context(), tenantID, id, groups)
		if tenantErr != nil {
			err = tenantErr
		}
		if err != nil {
			log.Printf("[SYNC] Group persist error: %v", err)
			syncErrors = append(syncErrors, fmt.Sprintf("groups persist: %v", err))
		}
	}

	// 3. Sync entitlements
	entitlements, err := h.ConnectorManager().SyncEntitlements(r.Context(), id)
	if err != nil {
		log.Printf("[SYNC] Entitlement sync error: %v", err)
		syncErrors = append(syncErrors, fmt.Sprintf("entitlements: %v", err))
	}
	if len(entitlements) > 0 {
		if tenantErr != nil {
			log.Printf("[SYNC] Entitlement persist error: %v", tenantErr)
			syncErrors = append(syncErrors, fmt.Sprintf("entitlements persist: %v", tenantErr))
		} else if err := h.Store().PersistSyncedEntitlements(r.Context(), tenantID, id, entitlements); err != nil {
			log.Printf("[SYNC] Entitlement persist error: %v", err)
			syncErrors = append(syncErrors, fmt.Sprintf("entitlements persist: %v", err))
		}
	}

	// 4. Sync resources
	resources, err := h.ConnectorManager().SyncResources(r.Context(), id)
	if err != nil {
		log.Printf("[SYNC] Resource sync error: %v", err)
		syncErrors = append(syncErrors, fmt.Sprintf("resources: %v", err))
	}
	if len(resources) > 0 {
		if tenantErr != nil {
			log.Printf("[SYNC] Resource persist error: %v", tenantErr)
			syncErrors = append(syncErrors, fmt.Sprintf("resources persist: %v", tenantErr))
		} else if err := h.Store().PersistSyncedResources(r.Context(), tenantID, id, resources); err != nil {
			log.Printf("[SYNC] Resource persist error: %v", err)
			syncErrors = append(syncErrors, fmt.Sprintf("resources persist: %v", err))
		}
	}

	// 5. Sync permissions (catalog)
	permissions, err := h.ConnectorManager().SyncPermissions(r.Context(), id)
	if err != nil {
		log.Printf("[SYNC] Permission sync error: %v", err)
		syncErrors = append(syncErrors, fmt.Sprintf("permissions: %v", err))
	}
	if len(permissions) > 0 {
		if tenantErr != nil {
			log.Printf("[SYNC] Permission persist error: %v", tenantErr)
			syncErrors = append(syncErrors, fmt.Sprintf("permissions persist: %v", tenantErr))
		} else if err := h.Store().PersistSyncedPermissions(r.Context(), tenantID, id, permissions); err != nil {
			log.Printf("[SYNC] Permission persist error: %v", err)
			syncErrors = append(syncErrors, fmt.Sprintf("permissions persist: %v", err))
		}
	}

	// 6. Materialize to canonical identity model + Neo4j graph
	materializeResult, matErr := h.Store().MaterializeConnectorData(r.Context(), tenantID, id)
	if matErr != nil {
		log.Printf("[SYNC] Materialization error: %v", matErr)
		syncErrors = append(syncErrors, fmt.Sprintf("materialization: %v", matErr))
	} else if materializeResult != nil {
		if len(materializeResult.Errors) > 0 {
			log.Printf("[SYNC] Materialization partial errors: %v", materializeResult.Errors)
			syncErrors = append(syncErrors, fmt.Sprintf("materialization partial: %d errors (ent=%d res=%d groups=%d graph_nodes=%d graph_edges=%d uncorrelated=%d)",
				len(materializeResult.Errors), materializeResult.EntitlementsUpserted, materializeResult.ResourcesUpserted,
				materializeResult.GroupsUpserted, materializeResult.GraphNodesWritten, materializeResult.GraphEdgesWritten,
				materializeResult.UncorrelatedUsers))
		}
		h.AuditStore().Append(audit.Entry{
			Level: audit.LevelInfo, Service: "connector", Method: "POST", Path: r.URL.Path,
			Message: fmt.Sprintf("Materialized connector %s: %d entitlements, %d resources, %d groups, %d graph nodes, %d graph edges, %d uncorrelated users",
				id, materializeResult.EntitlementsUpserted, materializeResult.ResourcesUpserted,
				materializeResult.GroupsUpserted, materializeResult.GraphNodesWritten,
				materializeResult.GraphEdgesWritten, materializeResult.UncorrelatedUsers),
			Tags: []string{"connector", "materialize"},
		})
	}

	elapsed := time.Since(start)

	status := "full_sync_completed"
	if len(syncErrors) > 0 {
		status = "full_sync_with_errors"
	}

	h.AuditStore().Append(audit.Entry{
		Level:   audit.LevelInfo,
		Service: "connector",
		Method:  "POST",
		Path:    r.URL.Path,
		Message: fmt.Sprintf("Full sync %s for connector %s: %d users, %d groups, %d entitlements, %d resources, %d permissions in %s",
			status, id, usersCreated+usersUpdated, groupsCreated+groupsUpdated, len(entitlements), len(resources), len(permissions), elapsed.Round(time.Millisecond)),
		Tags: []string{"connector", "full-sync"},
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"status":             status,
		"connector_id":       id,
		"users_created":      usersCreated,
		"users_updated":      usersUpdated,
		"groups_created":     groupsCreated,
		"groups_updated":     groupsUpdated,
		"entitlements_total": len(entitlements),
		"resources_total":    len(resources),
		"permissions_total":  len(permissions),
		"errors":             syncErrors,
		"duration":           elapsed.Round(time.Millisecond).String(),
	})
}

// ─── Materialize (standalone) ────────────────────────────────

func (h *Handler) MaterializeConnector(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	tenantID, tenantErr := h.ConnectorTenantID(id)
	if tenantErr != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"status":       "materialize_failed",
			"connector_id": id,
			"error":        tenantErr.Error(),
		})
		return
	}

	res, err := h.Store().MaterializeConnectorData(r.Context(), tenantID, id)
	status := "materialized"
	if err != nil {
		status = "materialize_failed"
		respondJSON(w, http.StatusOK, map[string]any{
			"status":       status,
			"connector_id": id,
			"error":        err.Error(),
		})
		return
	}
	if res == nil {
		res = &stores.MaterializeResult{}
	}
	errors := res.Errors
	if len(errors) > 0 {
		status = "materialized_with_errors"
	}

	h.AuditStore().Append(audit.Entry{
		Level: audit.LevelInfo, Service: "connector", Method: "POST", Path: r.URL.Path,
		Message: fmt.Sprintf("Materialized connector %s: %d entitlements, %d resources, %d groups, %d graph nodes, %d graph edges, %d uncorrelated users",
			id, res.EntitlementsUpserted, res.ResourcesUpserted, res.GroupsUpserted,
			res.GraphNodesWritten, res.GraphEdgesWritten, res.UncorrelatedUsers),
		Tags: []string{"connector", "materialize"},
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"status":                status,
		"connector_id":          id,
		"entitlements_upserted": res.EntitlementsUpserted,
		"resources_upserted":     res.ResourcesUpserted,
		"groups_upserted":        res.GroupsUpserted,
		"graph_nodes_written":    res.GraphNodesWritten,
		"graph_edges_written":    res.GraphEdgesWritten,
		"uncorrelated_users":     res.UncorrelatedUsers,
		"errors":                 errors,
	})
}

func (h *Handler) GetConnectorPosture(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	db := h.DB(r.Context())

	var totalIdentities, totalResources, totalEntitlements int
	_ = db.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_identities WHERE connector_id = $1`, id).Scan(&totalIdentities)
	_ = db.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_resources WHERE connector_id = $1`, id).Scan(&totalResources)
	_ = db.QueryRow(r.Context(), `SELECT COUNT(*) FROM connector_entitlements WHERE connector_id = $1 AND is_active = true`, id).Scan(&totalEntitlements)

	var staleAccounts int
	db.QueryRow(r.Context(), `
		SELECT COUNT(DISTINCT identity_external_id)
		FROM connector_entitlements ce
		JOIN connector_identities ci ON ci.connector_id = ce.connector_id AND ci.external_id = ce.identity_external_id
		WHERE ce.connector_id = $1 AND ce.is_active = true AND ci.enabled = false
	`, id).Scan(&staleAccounts)

	var orphanedSPs int
	db.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM connector_resources WHERE connector_id = $1 AND (owner_ids = ARRAY[]::text[] OR array_length(owner_ids, 1) IS NULL)
	`, id).Scan(&orphanedSPs)

	var privilegedRoles int
	db.QueryRow(r.Context(), `
		SELECT COUNT(DISTINCT identity_external_id)
		FROM connector_entitlements
		WHERE connector_id = $1 AND entitlement_type = 'directory_role' AND is_active = true
	`, id).Scan(&privilegedRoles)

	var toxicAssignments int
	db.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM connector_entitlements
		WHERE connector_id = $1 AND is_active = true
		  AND (entitlement_type = 'directory_role' OR entitlement_type = 'app_role')
	`, id).Scan(&toxicAssignments)

	// Best-effort behavioral data from Graph audit logs (sign-in, MFA, activity)
	behavioral := h.fetchBehavioralPosture(r.Context(), id)

	posture := map[string]any{
		"connector_id": id,
		"posture_level": func() string {
			if staleAccounts > 0 || orphanedSPs > 0 || privilegedRoles > 10 || toxicAssignments > 5 {
				return "poor"
			}
			if orphanedSPs > 0 || staleAccounts > 0 {
				return "at_risk"
			}
			return "good"
		}(),
		"metrics": map[string]any{
			"total_identities":          totalIdentities,
			"total_resources":           totalResources,
			"total_active_entitlements": totalEntitlements,
			"stale_accounts_enabled_false_but_assigned": staleAccounts,
			"orphaned_service_principals_no_owner":     orphanedSPs,
			"privileged_directory_role_assignments":   privilegedRoles,
			"potential_toxic_assignments":               toxicAssignments,
		},
		"behavioral": behavioral,
		"recommendations": []string{},
	}

	recs := []string{}
	if staleAccounts > 0 {
		recs = append(recs, fmt.Sprintf("%d disabled accounts still hold active directory/app assignments — disable or revoke per SailPoint 'stale entitlement' control.", staleAccounts))
	}
	if orphanedSPs > 0 {
		recs = append(recs, fmt.Sprintf("%d service principals/applications have no owners (orphaned SPs) — assign owners to prevent ungoverned access, per SailPoint best practice.", orphanedSPs))
	}
	if privilegedRoles > 10 {
		recs = append(recs, fmt.Sprintf("%d privileged directory role assignments detected — review privileged access per least-privilege / zero-standing-access.", privilegedRoles))
	}
	if behavioral["note"] != nil {
		recs = append(recs, behavioral["note"].(string))
	}
	posture["recommendations"] = recs

	respondJSON(w, http.StatusOK, posture)
}

// fetchBehavioralPosture attempts to fetch sign-in activity, MFA status, and recent activity from Graph audit logs.
// Returns a map with behavioral metrics and a note if data is unavailable (requires AuditLog.Read.All scope).
func (h *Handler) fetchBehavioralPosture(ctx context.Context, connectorID string) map[string]any {
	// Get connector config to obtain Graph client / token
	mgr := h.ConnectorManager()
	cfg, err := mgr.GetConfig(connectorID)
	if err != nil {
		return map[string]any{
			"available": false,
			"note":      "Connector config not found — behavioral posture requires valid Entra connector with AuditLog.Read.All scope",
		}
	}

	// For Entra connectors, use the stored credentials to call Graph audit logs
	// This requires AuditLog.Read.All scope on the app registration
	if cfg.Type != connector.ConnectorTypeEntraID {
		return map[string]any{
			"available": false,
			"note":      "Behavioral posture currently supports Entra ID connectors only",
		}
	}

	// Try to get access token - for client credentials flow
	token, err := h.getGraphToken(ctx, cfg)
	if err != nil {
		return map[string]any{
			"available": false,
			"note":      fmt.Sprintf("Graph token unavailable — behavioral posture requires valid Entra app credentials: %v", err),
		}
	}

	// Best-effort: try to fetch recent sign-ins (last 30 days) and MFA data
	since := time.Now().AddDate(0, 0, -30).Format(time.RFC3339)

	var signInCount int
	var mfaEnabledCount int
	var inactiveUsers []string

	// Try sign-ins endpoint
	client := &http.Client{Timeout: 10 * time.Second}
	signInURL := fmt.Sprintf("https://graph.microsoft.com/v1.0/auditLogs/signIns?$filter=createdDateTime ge %s&$top=500", since)
	req, _ := http.NewRequestWithContext(ctx, "GET", signInURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil || resp == nil || resp.StatusCode != 200 {
		return map[string]any{
			"available":         false,
			"note":              "Sign-in audit logs unavailable — requires AuditLog.Read.All scope on Entra app registration. Add scope and re-consent to enable behavioral posture (inactive users, MFA compliance, risky sign-ins).",
			"sign_in_count":     0,
			"mfa_compliance":    "unknown",
			"inactive_30d":      []string{},
		}
	}
	defer resp.Body.Close()

	var signInData struct {
		Value []struct {
			UserPrincipalName string `json:"userPrincipalName"`
			Status            struct {
				ErrorCode      int    `json:"errorCode"`
				FailureReason string `json:"failureReason"`
			} `json:"status"`
			AuthenticationRequirement string `json:"authenticationRequirement"`
			AuthenticationDetails     []struct {
				AuthenticationMethod string `json:"authenticationMethod"`
				Succeeded            bool   `json:"succeeded"`
			} `json:"authenticationDetails"`
			CreatedDateTime string `json:"createdDateTime"`
			RiskLevel       string `json:"riskLevel"`
			RiskState       string `json:"riskState"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&signInData); err == nil {
		signInCount = len(signInData.Value)
		userSignIns := make(map[string]int)
		for _, si := range signInData.Value {
			if si.UserPrincipalName != "" {
				userSignIns[si.UserPrincipalName]++
				for _, ad := range si.AuthenticationDetails {
					if ad.AuthenticationMethod == "mfa" && ad.Succeeded {
						mfaEnabledCount++
						break
					}
				}
			}
		}

		// Identify inactive users (no sign-in in 30d) by cross-referencing with connector identities
		rows, _ := h.DB(ctx).Query(ctx, `SELECT external_id, user_principal_name FROM connector_identities WHERE connector_id = $1 AND enabled = true`, connectorID)
		for rows.Next() {
			var extID, upn string
			rows.Scan(&extID, &upn)
			if upn != "" && userSignIns[upn] == 0 {
				inactiveUsers = append(inactiveUsers, upn)
			}
		}
		rows.Close()

		mfaCompliance := "partial"
		if signInCount > 0 {
			pct := float64(mfaEnabledCount) / float64(signInCount) * 100
			if pct >= 90 {
				mfaCompliance = "high"
			} else if pct >= 50 {
				mfaCompliance = "medium"
			} else {
				mfaCompliance = "low"
			}
		}

		return map[string]any{
			"available":           true,
			"sign_in_count":       signInCount,
			"mfa_compliance":      mfaCompliance,
			"inactive_30d_count":  len(inactiveUsers),
			"inactive_30d_sample": inactiveUsers[:min(10, len(inactiveUsers))],
			"data_since":          since,
		}
	}

	return map[string]any{
		"available": false,
		"note":      "Failed to parse sign-in audit log response — behavioral posture requires valid AuditLog.Read.All scope and proper app registration.",
	}
}

// getGraphToken obtains an access token for Microsoft Graph using client credentials flow
func (h *Handler) getGraphToken(ctx context.Context, cfg connector.ConnectorConfig) (string, error) {
	// For Entra ID connectors, credentials are in ClientID, ClientSecret, and TenantName/Endpoint
	clientID := cfg.ClientID
	clientSecret := cfg.ClientSecret
	tenantID := cfg.TenantName
	if tenantID == "" {
		tenantID = cfg.Endpoint // fallback
	}

	if clientID == "" || clientSecret == "" || tenantID == "" {
		return "", fmt.Errorf("missing client_id, client_secret, or tenant_id in connector config")
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("scope", "https://graph.microsoft.com/.default")
	data.Set("grant_type", "client_credentials")

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}
	if tokenResp.Error != "" {
		return "", fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	return tokenResp.AccessToken, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── Persistence: Groups ───────────────────────────────────
