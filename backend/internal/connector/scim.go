package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// ─── Universal SCIM Connector ────────────────────────────────
// Connects to any SCIM 2.0-compatible identity provider.
// Used as a universal connector for any system that supports the SCIM 2.0 standard.

type SCIMConnector struct {
	config ConnectorConfig
	client *http.Client
	token  string
}

func NewSCIMConnector() *SCIMConnector {
	return &SCIMConnector{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *SCIMConnector) Type() ConnectorType         { return ConnectorTypeSCIM }
func (c *SCIMConnector) Name() string                { return c.config.Name }
func (c *SCIMConnector) GetStatus(ctx context.Context) ConnectorStatus { return c.config.Status }

func (c *SCIMConnector) Configure(config ConnectorConfig) error {
	if config.Endpoint == "" {
		return fmt.Errorf("scim: endpoint URL is required")
	}
	// Ensure trailing slash for clean path joining
	config.Endpoint = config.Endpoint + "/"
	c.config = config

	// Set up auth
	switch config.AuthType {
	case "basic":
		// username/password used in Authorization header
	case "api_key":
		// api key in Authorization header or custom header
	case "oauth2":
		// client_credentials flow to get token
	case "bearer":
		c.token = config.Password
	case "", "none":
		// no auth
	default:
		return fmt.Errorf("scim: unsupported auth_type: %s", config.AuthType)
	}

	return nil
}

func (c *SCIMConnector) scimRequest(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.config.Endpoint+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/scim+json")
	req.Header.Set("Accept", "application/scim+json,application/json")

	// Set auth
	switch c.config.AuthType {
	case "basic":
		req.SetBasicAuth(c.config.Username, c.config.Password)
	case "api_key":
		if c.config.Properties != nil && c.config.Properties["api_key_header"] != "" {
			req.Header.Set(c.config.Properties["api_key_header"], c.config.Password)
		} else {
			req.Header.Set("Authorization", "Bearer "+c.config.Password)
		}
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.token)
	case "oauth2":
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("scim: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("scim: %s %s returned %d: %s", method, path, resp.StatusCode, string(data))
	}
	return data, nil
}

// ─── Connection ──────────────────────────────────────────────

func (c *SCIMConnector) Connect(ctx context.Context) error {
	if c.config.AuthType == "oauth2" && c.token == "" {
		if err := c.refreshToken(ctx); err != nil {
			c.config.Status = ConnectorStatusError
			return err
		}
	}

	// Test connection by fetching ServiceProviderConfig
	_, err := c.scimRequest(ctx, "GET", "ServiceProviderConfig", nil)
	if err != nil {
		c.config.Status = ConnectorStatusError
		return fmt.Errorf("scim: connection test failed: %w", err)
	}

	c.config.Status = ConnectorStatusConnected
	log.Printf("[SCIM] Connected to %s (%s)", c.config.Name, c.config.Endpoint)
	return nil
}

func (c *SCIMConnector) refreshToken(ctx context.Context) error {
	if c.config.TokenURL == "" {
		c.config.TokenURL = c.config.Endpoint + "token"
	}

	data := url.Values{
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
		"grant_type":    {"client_credentials"},
		"scope":         {"scim"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.TokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("scim: token request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("scim: token decode: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("scim: oauth error: %s", result.Error)
	}

	if result.TokenType != "" {
		c.token = result.TokenType + " " + result.AccessToken
	} else {
		c.token = "Bearer " + result.AccessToken
	}
	return nil
}

func (c *SCIMConnector) Disconnect(ctx context.Context) error {
	c.config.Status = ConnectorStatusDisconnected
	return nil
}

func (c *SCIMConnector) TestConnection(ctx context.Context) error {
	_, err := c.scimRequest(ctx, "GET", "ServiceProviderConfig", nil)
	return err
}

// ─── User Operations ─────────────────────────────────────────

func (c *SCIMConnector) ListUsers(ctx context.Context) ([]ConnectorUser, error) {
	var users []ConnectorUser
	startIndex := 1

	for {
		path := fmt.Sprintf("Users?startIndex=%d&count=100", startIndex)
		body, err := c.scimRequest(ctx, "GET", path, nil)
		if err != nil {
			return users, err
		}

		var result struct {
			Schemas      []string          `json:"schemas"`
			TotalResults int               `json:"totalResults"`
			ItemsPerPage int               `json:"itemsPerPage"`
			StartIndex   int               `json:"startIndex"`
			Resources    []json.RawMessage `json:"Resources"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("scim: decode users: %w", err)
		}

		for _, raw := range result.Resources {
			var su struct {
				ID             string `json:"id"`
				UserName       string `json:"userName"`
				Name           struct {
					GivenName  string `json:"givenName"`
					FamilyName string `json:"familyName"`
					Formatted  string `json:"formatted"`
				} `json:"name"`
				DisplayName    string `json:"displayName"`
				Emails         []struct {
					Value   string `json:"value"`
					Primary bool   `json:"primary"`
					Type    string `json:"type"`
				} `json:"emails"`
				PhoneNumbers []struct {
					Value string `json:"value"`
					Type  string `json:"type"`
				} `json:"phoneNumbers"`
				Active         bool              `json:"active"`
				Department     string            `json:"department"`
				Title          string            `json:"title"`
				EmployeeNumber string            `json:"employeeNumber"`
				ExternalID     string            `json:"externalId"`
				Meta           struct {
					Created  string `json:"created"`
					LastModified string `json:"lastModified"`
				} `json:"meta"`
				Groups []struct {
					Value   string `json:"value"`
					Display string `json:"display"`
				} `json:"groups"`
			}
			if err := json.Unmarshal(raw, &su); err != nil {
				continue
			}

			displayName := su.DisplayName
			if displayName == "" && su.Name.Formatted != "" {
				displayName = su.Name.Formatted
			}
			if displayName == "" {
				displayName = su.UserName
			}

			email := ""
			for _, e := range su.Emails {
				if e.Primary || e.Type == "work" || email == "" {
					email = e.Value
				}
			}

			phone := ""
			mobile := ""
			for _, p := range su.PhoneNumbers {
				switch p.Type {
				case "work", "office":
					phone = p.Value
				case "mobile", "cell":
					mobile = p.Value
				default:
					if phone == "" {
						phone = p.Value
					}
				}
			}

			var groupIDs []string
			for _, g := range su.Groups {
				groupIDs = append(groupIDs, g.Value)
			}

			createdAt, _ := time.Parse(time.RFC3339, su.Meta.Created)
			updatedAt, _ := time.Parse(time.RFC3339, su.Meta.LastModified)

			users = append(users, ConnectorUser{
				ExternalID:  su.ID,
				Username:    su.UserName,
				Email:       email,
				DisplayName: displayName,
				FirstName:   su.Name.GivenName,
				LastName:    su.Name.FamilyName,
				Department:  su.Department,
				Title:       su.Title,
				Phone:       phone,
				Mobile:      mobile,
				EmployeeID:  su.EmployeeNumber,
				Enabled:     su.Active,
				Groups:      groupIDs,
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
			})
		}

		if (startIndex + result.ItemsPerPage) > result.TotalResults {
			break
		}
		startIndex += result.ItemsPerPage
	}

	return users, nil
}

func (c *SCIMConnector) GetUser(ctx context.Context, externalID string) (*ConnectorUser, error) {
	body, err := c.scimRequest(ctx, "GET", "Users/"+url.PathEscape(externalID), nil)
	if err != nil {
		return nil, err
	}
	return parseSCIMUser(body), nil
}

func (c *SCIMConnector) GetUserByUsername(ctx context.Context, username string) (*ConnectorUser, error) {
	encoded := url.QueryEscape(fmt.Sprintf("userName eq \"%s\"", username))
	body, err := c.scimRequest(ctx, "GET", fmt.Sprintf("Users?filter=%s", encoded), nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		TotalResults int               `json:"totalResults"`
		Resources    []json.RawMessage `json:"Resources"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("scim: decode user lookup: %w", err)
	}
	if result.TotalResults == 0 || len(result.Resources) == 0 {
		return nil, fmt.Errorf("scim: user not found: %s", username)
	}

	return parseSCIMUser(result.Resources[0]), nil
}

func parseSCIMUser(raw json.RawMessage) *ConnectorUser {
	var su struct {
		ID          string `json:"id"`
		UserName    string `json:"userName"`
		Name        struct {
			GivenName  string `json:"givenName"`
			FamilyName string `json:"familyName"`
			Formatted  string `json:"formatted"`
		} `json:"name"`
		DisplayName string `json:"displayName"`
		Emails      []struct {
			Value   string `json:"value"`
			Primary bool   `json:"primary"`
		} `json:"emails"`
		PhoneNumbers []struct {
			Value string `json:"value"`
			Type  string `json:"type"`
		} `json:"phoneNumbers"`
		Active         bool   `json:"active"`
		Department     string `json:"department"`
		Title          string `json:"title"`
		EmployeeNumber string `json:"employeeNumber"`
		Meta           struct {
			Created      string `json:"created"`
			LastModified string `json:"lastModified"`
		} `json:"meta"`
	}
	json.Unmarshal(raw, &su)

	email := ""
	if len(su.Emails) > 0 {
		email = su.Emails[0].Value
		for _, e := range su.Emails {
			if e.Primary {
				email = e.Value
			}
		}
	}
	phone := ""
	if len(su.PhoneNumbers) > 0 {
		phone = su.PhoneNumbers[0].Value
	}

	createdAt, _ := time.Parse(time.RFC3339, su.Meta.Created)
	updatedAt, _ := time.Parse(time.RFC3339, su.Meta.LastModified)

	return &ConnectorUser{
		ExternalID:  su.ID,
		Username:    su.UserName,
		Email:       email,
		DisplayName: su.DisplayName,
		FirstName:   su.Name.GivenName,
		LastName:    su.Name.FamilyName,
		Department:  su.Department,
		Title:       su.Title,
		Phone:       phone,
		EmployeeID:  su.EmployeeNumber,
		Enabled:     su.Active,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}
}

func (c *SCIMConnector) CreateUser(ctx context.Context, user ConnectorUser) (string, error) {
	// Build SCIM user payload
	scimUser := map[string]any{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		"userName": user.Username,
		"name": map[string]string{
			"givenName":  user.FirstName,
			"familyName": user.LastName,
			"formatted":  user.DisplayName,
		},
		"displayName": user.DisplayName,
		"active":      user.Enabled,
		"emails": []map[string]any{
			{
				"value":   user.Email,
				"primary": true,
				"type":    "work",
			},
		},
	}

	if user.Department != "" {
		scimUser["department"] = user.Department
	}
	if user.Title != "" {
		scimUser["title"] = user.Title
	}
	if user.EmployeeID != "" {
		scimUser["employeeNumber"] = user.EmployeeID
	}
	if user.Phone != "" {
		scimUser["phoneNumbers"] = []map[string]any{
			{"value": user.Phone, "type": "work"},
		}
	}
	if user.Mobile != "" {
		scimUser["phoneNumbers"] = append(
			scimUser["phoneNumbers"].([]map[string]any),
			map[string]any{"value": user.Mobile, "type": "mobile"},
		)
	}

	body, err := c.scimRequest(ctx, "POST", "Users", scimUser)
	if err != nil {
		return "", err
	}

	var created struct{ ID string }
	json.Unmarshal(body, &created)
	return created.ID, nil
}

func (c *SCIMConnector) UpdateUser(ctx context.Context, externalID string, user ConnectorUser) error {
	scimUser := map[string]any{
		"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
	}

	if user.DisplayName != "" {
		scimUser["displayName"] = user.DisplayName
		scimUser["name"] = map[string]string{
			"givenName":  user.FirstName,
			"familyName": user.LastName,
			"formatted":  user.DisplayName,
		}
	}
	if user.Email != "" {
		scimUser["emails"] = []map[string]any{
			{"value": user.Email, "primary": true, "type": "work"},
		}
	}
	if user.Department != "" {
		scimUser["department"] = user.Department
	}
	if user.Title != "" {
		scimUser["title"] = user.Title
	}

	_, err := c.scimRequest(ctx, "PUT", "Users/"+url.PathEscape(externalID), scimUser)
	return err
}

func (c *SCIMConnector) DeleteUser(ctx context.Context, externalID string) error {
	_, err := c.scimRequest(ctx, "DELETE", "Users/"+url.PathEscape(externalID), nil)
	return err
}

func (c *SCIMConnector) DisableUser(ctx context.Context, externalID string) error {
	_, err := c.scimRequest(ctx, "PATCH", "Users/"+url.PathEscape(externalID), map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{
			{
				"op":    "replace",
				"value": map[string]bool{"active": false},
			},
		},
	})
	return err
}

func (c *SCIMConnector) EnableUser(ctx context.Context, externalID string) error {
	_, err := c.scimRequest(ctx, "PATCH", "Users/"+url.PathEscape(externalID), map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{
			{
				"op":    "replace",
				"value": map[string]bool{"active": true},
			},
		},
	})
	return err
}

// ─── Group Operations ────────────────────────────────────────

func (c *SCIMConnector) ListGroups(ctx context.Context) ([]ConnectorGroup, error) {
	var groups []ConnectorGroup
	startIndex := 1

	for {
		body, err := c.scimRequest(ctx, "GET", fmt.Sprintf("Groups?startIndex=%d&count=100", startIndex), nil)
		if err != nil {
			return groups, err
		}

		var result struct {
			TotalResults int               `json:"totalResults"`
			ItemsPerPage int               `json:"itemsPerPage"`
			Resources    []json.RawMessage `json:"Resources"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("scim: decode groups: %w", err)
		}

		for _, raw := range result.Resources {
			var sg struct {
				ID          string `json:"id"`
				DisplayName string `json:"displayName"`
				ExternalID  string `json:"externalId"`
				Members     []struct {
					Value   string `json:"value"`
					Display string `json:"display"`
				} `json:"members"`
				Meta struct {
					Created      string `json:"created"`
					LastModified string `json:"lastModified"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(raw, &sg); err != nil {
				continue
			}

			var memberIDs []string
			for _, m := range sg.Members {
				memberIDs = append(memberIDs, m.Value)
			}

			createdAt, _ := time.Parse(time.RFC3339, sg.Meta.Created)

			groups = append(groups, ConnectorGroup{
				ExternalID: sg.ID,
				Name:       sg.DisplayName,
				Members:    memberIDs,
				CreatedAt:  createdAt,
			})
		}

		if (startIndex + result.ItemsPerPage) > result.TotalResults {
			break
		}
		startIndex += result.ItemsPerPage
	}

	return groups, nil
}

func (c *SCIMConnector) GetGroup(ctx context.Context, externalID string) (*ConnectorGroup, error) {
	body, err := c.scimRequest(ctx, "GET", "Groups/"+url.PathEscape(externalID), nil)
	if err != nil {
		return nil, err
	}

	var sg struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Members     []struct {
			Value string `json:"value"`
		} `json:"members"`
		Meta struct {
			Created string `json:"created"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &sg); err != nil {
		return nil, fmt.Errorf("scim: decode group: %w", err)
	}

	var memberIDs []string
	for _, m := range sg.Members {
		memberIDs = append(memberIDs, m.Value)
	}

	createdAt, _ := time.Parse(time.RFC3339, sg.Meta.Created)

	return &ConnectorGroup{
		ExternalID: sg.ID,
		Name:       sg.DisplayName,
		Members:    memberIDs,
		CreatedAt:  createdAt,
	}, nil
}

func (c *SCIMConnector) CreateGroup(ctx context.Context, group ConnectorGroup) (string, error) {
	payload := map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
		"displayName": group.Name,
	}
	if group.Description != "" {
		payload["description"] = group.Description
	}

	body, err := c.scimRequest(ctx, "POST", "Groups", payload)
	if err != nil {
		return "", err
	}

	var created struct{ ID string }
	json.Unmarshal(body, &created)
	return created.ID, nil
}

func (c *SCIMConnector) UpdateGroup(ctx context.Context, externalID string, group ConnectorGroup) error {
	_, err := c.scimRequest(ctx, "PUT", "Groups/"+url.PathEscape(externalID), map[string]any{
		"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:Group"},
		"displayName": group.Name,
	})
	return err
}

func (c *SCIMConnector) DeleteGroup(ctx context.Context, externalID string) error {
	_, err := c.scimRequest(ctx, "DELETE", "Groups/"+url.PathEscape(externalID), nil)
	return err
}

func (c *SCIMConnector) AddGroupMember(ctx context.Context, groupID, userID string) error {
	_, err := c.scimRequest(ctx, "PATCH", "Groups/"+url.PathEscape(groupID), map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{
			{
				"op":    "add",
				"path":  "members",
				"value": []map[string]string{{"value": userID}},
			},
		},
	})
	return err
}

func (c *SCIMConnector) RemoveGroupMember(ctx context.Context, groupID, userID string) error {
	_, err := c.scimRequest(ctx, "PATCH", "Groups/"+url.PathEscape(groupID), map[string]any{
		"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:PatchOp"},
		"Operations": []map[string]any{
			{
				"op":   "remove",
				"path": fmt.Sprintf("members[value eq \"%s\"]", userID),
			},
		},
	})
	return err
}
