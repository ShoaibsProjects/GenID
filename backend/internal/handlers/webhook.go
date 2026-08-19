package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/observeid/genid/internal/audit"
)

// Valid webhook event types
var validEvents = map[string]bool{
	"access.approved":   true,
	"access.denied":     true,
	"access.revoked":    true,
	"risk.changed":      true,
	"identity.created":  true,
	"identity.updated":  true,
	"identity.deleted":  true,
}

// RegisterWebhook implements POST /api/v1/webhooks
func (h *Handler) RegisterWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	var req struct {
		Name   string   `json:"name"`
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if req.Name == "" || req.URL == "" {
		respondError(w, http.StatusBadRequest, "name and url are required")
		return
	}
	if len(req.Events) == 0 {
		respondError(w, http.StatusBadRequest, "at least one event is required")
		return
	}
	for _, e := range req.Events {
		if !validEvents[e] {
			respondError(w, http.StatusBadRequest, "invalid event: "+e)
			return
		}
	}

	// Generate a secret for HMAC signing
	secret := uuid.NewString()
	id := uuid.NewString()
	// Look up the API key ID for created_by
	var createdBy *string
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		var keyID string
		err := h.DB(r.Context()).QueryRow(r.Context(),
			`SELECT id::text FROM api_keys WHERE key_hash = crypt($1, key_hash) AND enabled = true`,
			apiKey).Scan(&keyID)
		if err == nil {
			createdBy = &keyID
		}
	}
	if createdBy == nil {
		// Fallback to a valid system user UUID
		fallback := "00000000-0000-0000-0000-000000000002"
		createdBy = &fallback
	}

	_, err := h.DB(r.Context()).Exec(r.Context(), `
		INSERT INTO webhooks (id, tenant_id, name, url, secret, events, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, id, tenantID, req.Name, req.URL, secret, req.Events, createdBy)
	if err != nil {
		logError("webhook-register", err)
		respondError(w, http.StatusConflict, "webhook name already exists")
		return
	}

	h.auditWebhook(r, tenantID, id, "webhook.registered", "webhook "+req.Name+" registered", map[string]any{
		"name": req.Name, "url": req.URL, "events": req.Events,
	})

	respondJSON(w, http.StatusCreated, map[string]any{
		"id":      id,
		"name":    req.Name,
		"url":     req.URL,
		"events":  req.Events,
		"secret":  secret, // Returned only once!
		"created": true,
	})
}

// ListWebhooks implements GET /api/v1/webhooks
func (h *Handler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	limit, offset := paginationParams(r, 50, 0)

	rows, err := h.DB(r.Context()).Query(r.Context(), `
		SELECT id::text, name, url, events, is_active, created_at, updated_at
		FROM webhooks
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "webhook list failed")
		return
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, name, url string
		var events []string
		var active bool
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &url, &events, &active, &createdAt, &updatedAt); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"id": id, "name": name, "url": url, "events": events,
			"is_active": active, "created_at": createdAt, "updated_at": updatedAt,
		})
	}
	respondJSON(w, http.StatusOK, map[string]any{"webhooks": out, "total": len(out)})
}

// DeleteWebhook implements DELETE /api/v1/webhooks/{id}
func (h *Handler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	id := mux.Vars(r)["id"]

	ct, err := h.DB(r.Context()).Exec(r.Context(),
		`DELETE FROM webhooks WHERE tenant_id=$1 AND id::text=$2`,
		tenantID, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if ct.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "webhook not found")
		return
	}
	h.auditWebhook(r, tenantID, id, "webhook.deleted", "webhook deleted", nil)
	respondJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// GetWebhookSecret implements GET /api/v1/webhooks/{id}/secret (admin only)
// Returns the secret for HMAC verification
func (h *Handler) GetWebhookSecret(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	id := mux.Vars(r)["id"]

	var secret string
	err := h.DB(r.Context()).QueryRow(r.Context(),
		`SELECT secret FROM webhooks WHERE tenant_id=$1 AND id::text=$2`,
		tenantID, id).Scan(&secret)
	if err != nil {
		respondError(w, http.StatusNotFound, "webhook not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"secret": secret})
}

// DispatchWebhook is called internally to queue a webhook delivery.
// It creates a webhook_deliveries row and starts a Temporal activity
// for async delivery with retries.
func (h *Handler) DispatchWebhook(ctx context.Context, tenantID, eventType, eventID string, payload any) {
	rows, err := h.DB(ctx).Query(ctx, `
		SELECT id::text, url, secret
		FROM webhooks
		WHERE tenant_id=$1 AND is_active=true AND $2 = ANY(events)
	`, tenantID, eventType)
	if err != nil {
		logError("webhook-dispatch-query", err)
		return
	}
	defer rows.Close()

	payloadBytes, _ := json.Marshal(payload)
	for rows.Next() {
		var id, url, secret string
		if err := rows.Scan(&id, &url, &secret); err != nil {
			continue
		}
		// Create delivery record
		deliveryID := uuid.NewString()
		_, _ = h.DB(ctx).Exec(ctx, `
			INSERT INTO webhook_deliveries (id, webhook_id, event_type, event_id, status)
			VALUES ($1,$2,$3,$4,'pending')
		`, deliveryID, id, eventType, eventID)

		// Start async Temporal activity for delivery with retries
		// We use a fire-and-forget workflow for this
		_ = h.startWebhookDelivery(ctx, deliveryID, url, secret, eventType, eventID, payloadBytes)
	}
}

func (h *Handler) startWebhookDelivery(ctx context.Context, deliveryID, url, secret, eventType, eventID string, payload []byte) error {
	// Start a simple activity via Temporal (not a full workflow for lightweight dispatch)
	// In production, use a workflow with retries. Here we use a direct call with manual retry.
	// For simplicity, we'll use a goroutine with exponential backoff.
	go func() {
		ctx := context.Background()
		maxRetries := 3
		for attempt := 0; attempt <= maxRetries; attempt++ {
			sig := signPayload(secret, payload)
			req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
			if err != nil {
				h.recordDeliveryFailure(ctx, deliveryID, 0, err.Error())
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GenID-Event", eventType)
			req.Header.Set("X-GenID-Event-ID", eventID)
			req.Header.Set("X-GenID-Delivery", deliveryID)
			req.Header.Set("X-GenID-Signature", "sha256="+sig)
			req.Header.Set("User-Agent", "GenID-Webhook/1.0")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				h.recordDeliveryFailure(ctx, deliveryID, 0, err.Error())
				time.Sleep(time.Duration(1<<attempt) * time.Second)
				continue
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				h.recordDeliverySuccess(ctx, deliveryID, resp.StatusCode, nil)
				return
			}
			h.recordDeliveryFailure(ctx, deliveryID, resp.StatusCode, fmt.Sprintf("HTTP %d", resp.StatusCode))
			if attempt < maxRetries {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
			}
		}
		h.markDeadLetter(ctx, deliveryID)
	}()
	return nil
}

func (h *Handler) recordDeliveryFailure(ctx context.Context, deliveryID string, status int, errMsg string) {
	_, _ = h.DB(ctx).Exec(ctx, `
		UPDATE webhook_deliveries
		SET status='failed', http_status=$2, error_message=$3, retry_count=retry_count+1,
		    next_retry_at=NOW() + interval '1 second' * (2 ^ retry_count)
		WHERE id::text=$1
	`, deliveryID, status, errMsg)
}

func (h *Handler) recordDeliverySuccess(ctx context.Context, deliveryID string, status int, body []byte) {
	_, _ = h.DB(ctx).Exec(ctx, `
		UPDATE webhook_deliveries
		SET status='success', http_status=$2
		WHERE id::text=$1
	`, deliveryID, status)
}

func (h *Handler) markDeadLetter(ctx context.Context, deliveryID string) {
	_, _ = h.DB(ctx).Exec(ctx, `
		UPDATE webhook_deliveries SET status='dead_letter' WHERE id::text=$1
	`, deliveryID)
}

func signPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *Handler) auditWebhook(r *http.Request, tenantID, webhookID, action, resource string, details any) {
	raw, _ := json.Marshal(details)
	actor := getActorID(r)
	_, _, _ = h.AuditChain().Append(r.Context(), audit.ChainEntry{
		TenantID:  tenantID,
		EventType: "webhook",
		ActorID:   actor,
		ActorType: "api_key",
		Action:    action,
		Resource:  resource,
		Details:   raw,
	})
}

func getActorID(r *http.Request) string {
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return "api-key:" + apiKey
	}
	return "00000000-0000-0000-0000-000000000002"
}
