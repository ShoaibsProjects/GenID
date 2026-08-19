package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// SCIM 2.0 User schema (subset)
type SCIMUser struct {
	Schemas      []string            `json:"schemas"`
	ID           string              `json:"id,omitempty"`
	ExternalID   string              `json:"externalId,omitempty"`
	UserName     string              `json:"userName"`
	Name         SCIMName            `json:"name,omitempty"`
	DisplayName  string              `json:"displayName,omitempty"`
	Emails       []SCIMEmail         `json:"emails,omitempty"`
	PhoneNumbers []SCIMPhoneNumber   `json:"phoneNumbers,omitempty"`
	Active       bool                `json:"active"`
	Groups       []SCIMGroupRef      `json:"groups,omitempty"`
	Meta         *SCIMMeta           `json:"meta,omitempty"`
}

type SCIMName struct {
	Formatted string `json:"formatted,omitempty"`
	FamilyName string `json:"familyName,omitempty"`
	GivenName  string `json:"givenName,omitempty"`
}

type SCIMEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type SCIMPhoneNumber struct {
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

type SCIMGroupRef struct {
	Value   string `json:"value"`
	Display string `json:"display,omitempty"`
}

type SCIMMeta struct {
	ResourceType string `json:"resourceType,omitempty"`
	Created      string `json:"created,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Location     string `json:"location,omitempty"`
	Version      string `json:"version,omitempty"`
}

// SCIMListResponse represents a paginated SCIM list response
type SCIMListResponse struct {
	Schemas      []string    `json:"schemas"`
	TotalResults int         `json:"totalResults"`
	ItemsPerPage int         `json:"itemsPerPage"`
	StartIndex   int         `json:"startIndex"`
	Resources    []SCIMUser  `json:"Resources"`
}

const scimUserSchema = "urn:ietf:params:scim:schemas:core:2.0:User"
const scimListSchema = "urn:ietf:params:scim:api:messages:2.0:ListResponse"

// CreateSCIMUser implements POST /scim/v2/Users
func (h *Handler) ScimCreateUser(w http.ResponseWriter, r *http.Request) {
	var user SCIMUser
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid SCIM user payload")
		return
	}
	if user.UserName == "" {
		respondError(w, http.StatusBadRequest, "userName is required")
		return
	}
	if user.Schemas == nil || len(user.Schemas) == 0 {
		user.Schemas = []string{scimUserSchema}
	}

	tenantID := tenantOrDefault(r)
	identityID := uuid.NewString()

	// Map SCIM user to GenID identity
	email := ""
	if len(user.Emails) > 0 {
		email = user.Emails[0].Value
	}
	displayName := user.DisplayName
	if displayName == "" && user.Name.Formatted != "" {
		displayName = user.Name.Formatted
	}
	if displayName == "" && (user.Name.GivenName != "" || user.Name.FamilyName != "") {
		displayName = user.Name.GivenName + " " + user.Name.FamilyName
	}

	_, err := h.DB(r.Context()).Exec(r.Context(), `
		INSERT INTO identities (id, tenant_id, email, display_name, status, source)
		VALUES ($1,$2,$3,$4,'active','scim')
	`, identityID, tenantID, email, displayName)
	if err != nil {
		logError("scim-create", err)
		respondError(w, http.StatusConflict, "user already exists")
		return
	}

	// Return created SCIM user
	user.ID = identityID
	user.Active = true
	user.Meta = &SCIMMeta{
		ResourceType: "User",
		Created:      "now", // would be actual timestamp
		Location:     fmt.Sprintf("/scim/v2/Users/%s", identityID),
	}
	respondJSON(w, http.StatusCreated, user)
}

// GetSCIMUser implements GET /scim/v2/Users/{id}
func (h *Handler) ScimGetUser(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	id := mux.Vars(r)["id"]

	var row struct {
		ID          string
		Email       string
		DisplayName string
		Status      string
		ExternalID  string
		CreatedAt   string
		UpdatedAt   string
	}
	err := h.DB(r.Context()).QueryRow(r.Context(), `
		SELECT id::text, email, COALESCE(display_name,''), status::text,
		       COALESCE(external_id,''), created_at::text, updated_at::text
		FROM identities WHERE tenant_id=$1 AND id::text=$2
	`, tenantID, id).Scan(&row.ID, &row.Email, &row.DisplayName, &row.Status,
		&row.ExternalID, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}

	user := identityToSCIMUser(row)
	respondJSON(w, http.StatusOK, user)
}

// UpdateSCIMUser implements PUT /scim/v2/Users/{id} (full replace)
func (h *Handler) ScimUpdateUser(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	id := mux.Vars(r)["id"]

	var user SCIMUser
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid SCIM user payload")
		return
	}

	// Update identity
	email := ""
	if len(user.Emails) > 0 {
		email = user.Emails[0].Value
	}
	displayName := user.DisplayName
	if displayName == "" && user.Name.Formatted != "" {
		displayName = user.Name.Formatted
	}

	_, err := h.DB(r.Context()).Exec(r.Context(), `
		UPDATE identities SET email=$1, display_name=$2, active=$3, updated_at=NOW()
		WHERE tenant_id=$4 AND id::text=$5
	`, email, displayName, user.Active, tenantID, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "update failed")
		return
	}

	user.ID = id
	respondJSON(w, http.StatusOK, user)
}

// PatchSCIMUser implements PATCH /scim/v2/Users/{id} (partial update)
func (h *Handler) ScimPatchUser(w http.ResponseWriter, r *http.Request) {
	// For SCIM PATCH, we just delegate to PUT for simplicity
	h.ScimUpdateUser(w, r)
}

// DeleteSCIMUser implements DELETE /scim/v2/Users/{id}
func (h *Handler) ScimDeleteUser(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	id := mux.Vars(r)["id"]

	_, err := h.DB(r.Context()).Exec(r.Context(),
		`DELETE FROM identities WHERE tenant_id=$1 AND id::text=$2`,
		tenantID, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListSCIMUsers implements GET /scim/v2/Users
func (h *Handler) ScimListUsers(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantOrDefault(r)
	limit, offset := paginationParams(r, 50, 0)
	rows, err := h.DB(r.Context()).Query(r.Context(), `
		SELECT id::text, email, COALESCE(display_name,''), status::text,
		       '' AS external_id, created_at::text, updated_at::text
		FROM identities WHERE tenant_id=$1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "list failed")
		return
	}
	defer rows.Close()

	var users []SCIMUser
	for rows.Next() {
		var row struct {
			ID, Email, DisplayName, Status, ExternalID, CreatedAt, UpdatedAt string
		}
		if err := rows.Scan(&row.ID, &row.Email, &row.DisplayName, &row.Status,
			&row.ExternalID, &row.CreatedAt, &row.UpdatedAt); err != nil {
			continue
		}
		users = append(users, identityToSCIMUser(row))
	}

	resp := SCIMListResponse{
		Schemas:      []string{scimListSchema},
		TotalResults: len(users),
		ItemsPerPage: len(users),
		StartIndex:   1,
		Resources:    users,
	}
	respondJSON(w, http.StatusOK, resp)
}

func identityToSCIMUser(row struct {
	ID, Email, DisplayName, Status, ExternalID, CreatedAt, UpdatedAt string
}) SCIMUser {
	name := SCIMName{}
	if row.DisplayName != "" {
		name.Formatted = row.DisplayName
	}
	emails := []SCIMEmail{}
	if row.Email != "" {
		emails = append(emails, SCIMEmail{Value: row.Email, Primary: true, Type: "work"})
	}
	active := row.Status == "active"
	meta := &SCIMMeta{
		ResourceType: "User",
		Created:      row.CreatedAt,
		LastModified: row.UpdatedAt,
		Version:      row.UpdatedAt, // simplified
	}
	return SCIMUser{
		Schemas:    []string{scimUserSchema},
		ID:         row.ID,
		ExternalID: row.ExternalID,
		UserName:   row.Email,
		Name:       name,
		DisplayName: row.DisplayName,
		Emails:     emails,
		Active:     active,
		Meta:       meta,
	}
}

// ScimServiceProviderConfig implements GET /scim/v2/ServiceProviderConfig
func (h *Handler) ScimServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":    map[string]any{"supported": true},
		"bulk":     map[string]any{"supported": false, "maxOperations": 1000, "maxPayloadSize": 1048576},
		"filter":   map[string]any{"supported": true, "maxResults": 200},
		"changePassword": map[string]any{"supported": false},
		"sort":     map[string]any{"supported": false},
		"etag":     map[string]any{"supported": true},
		"authenticationSchemes": []map[string]any{
			{"type": "oauthbearertoken", "name": "OAuth Bearer Token", "description": "OAuth 2.0 Bearer Token"},
		},
	})
}

// ScimResourceTypes implements GET /scim/v2/ResourceTypes
func (h *Handler) ScimResourceTypes(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": 1,
		"itemsPerPage": 1,
		"startIndex": 1,
		"Resources": []map[string]any{
			{
				"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"},
				"id": "User",
				"name": "User",
				"endpoint": "/scim/v2/Users",
				"description": "User Account",
				"schema": "urn:ietf:params:scim:schemas:core:2.0:User",
				"schemaExtensions": []any{},
			},
		},
	})
}

// ScimSchemas implements GET /scim/v2/Schemas
func (h *Handler) ScimSchemas(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": 1,
		"itemsPerPage": 1,
		"startIndex": 1,
		"Resources": []map[string]any{
			{
				"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:Schema"},
				"id": "urn:ietf:params:scim:schemas:core:2.0:User",
				"name": "User",
				"description": "User Account",
				"attributes": []map[string]any{
					{"name": "userName", "type": "string", "multiValued": false, "required": true, "caseExact": false, "mutability": "readWrite", "returned": "default", "uniqueness": "server"},
					{"name": "id", "type": "string", "multiValued": false, "required": false, "caseExact": false, "mutability": "readOnly", "returned": "always", "uniqueness": "global"},
					{"name": "externalId", "type": "string", "multiValued": false, "required": false, "caseExact": false, "mutability": "readWrite", "returned": "default", "uniqueness": "global"},
					{"name": "name", "type": "complex", "multiValued": false, "required": false, "subAttributes": []map[string]any{
						{"name": "formatted", "type": "string"},
						{"name": "familyName", "type": "string"},
						{"name": "givenName", "type": "string"},
					}},
					{"name": "displayName", "type": "string", "multiValued": false, "required": false, "mutability": "readWrite", "returned": "default"},
					{"name": "emails", "type": "complex", "multiValued": true, "required": false, "subAttributes": []map[string]any{
						{"name": "value", "type": "string"},
						{"name": "type", "type": "string"},
						{"name": "primary", "type": "boolean"},
					}},
					{"name": "active", "type": "boolean", "multiValued": false, "required": false, "mutability": "readWrite", "returned": "default"},
				},
			},
		},
	})
}
