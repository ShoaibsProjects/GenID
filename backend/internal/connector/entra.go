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

// ─── Entra ID (Microsoft Graph) Connector ─────────────────────
// Connects to Microsoft Entra ID using the Microsoft Graph API (v1.0).
// Supports OAuth2 client credentials flow.

type EntraConnector struct {
	config   ConnectorConfig
	client   *http.Client
	token    string
	expires  time.Time
}

func NewEntraConnector() *EntraConnector {
	return &EntraConnector{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *EntraConnector) Type() ConnectorType         { return ConnectorTypeEntraID }
func (c *EntraConnector) Name() string                { return c.config.Name }
func (c *EntraConnector) GetStatus(ctx context.Context) ConnectorStatus { return c.config.Status }

func (c *EntraConnector) Configure(config ConnectorConfig) error {
	if config.ClientID == "" || config.ClientSecret == "" || config.TenantName == "" {
		return fmt.Errorf("entra: client_id, client_secret, and tenant_name are required")
	}
	if config.Endpoint == "" {
		config.Endpoint = "https://graph.microsoft.com/v1.0"
	}
	if config.TokenURL == "" {
		config.TokenURL = fmt.Sprintf(
			"https://login.microsoftonline.com/%s/oauth2/v2.0/token",
			config.TenantName,
		)
	}
	c.config = config
	return nil
}

func (c *EntraConnector) ensureToken(ctx context.Context) error {
	if c.token != "" && time.Now().Before(c.expires) {
		return nil
	}

	data := url.Values{
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
		"scope":         {"https://graph.microsoft.com/.default"},
		"grant_type":    {"client_credentials"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.TokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return fmt.Errorf("entra: token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("entra: token request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("entra: token decode: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("entra: oauth error: %s", result.Error)
	}

	c.token = result.AccessToken
	c.expires = time.Now().Add(time.Duration(result.ExpiresIn-60) * time.Second)
	return nil
}

func (c *EntraConnector) graphGet(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	u := c.config.Endpoint + path
	if params != nil {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("ConsistencyLevel", "eventual")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entra: get %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("entra: get %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *EntraConnector) graphPost(ctx context.Context, path string, payload any) ([]byte, error) {
	if err := c.ensureToken(ctx); err != nil {
		return nil, err
	}

	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.Endpoint+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("entra: post %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("entra: post %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *EntraConnector) graphPatch(ctx context.Context, path string, payload any) error {
	if err := c.ensureToken(ctx); err != nil {
		return err
	}

	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "PATCH", c.config.Endpoint+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("entra: patch %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("entra: patch %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return nil
}

func (c *EntraConnector) graphDelete(ctx context.Context, path string) error {
	if err := c.ensureToken(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", c.config.Endpoint+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("entra: delete %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("entra: delete %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return nil
}

// ─── Connection ──────────────────────────────────────────────

func (c *EntraConnector) Connect(ctx context.Context) error {
	if err := c.ensureToken(ctx); err != nil {
		c.config.Status = ConnectorStatusError
		return err
	}
	c.config.Status = ConnectorStatusConnected
	log.Printf("[ENTRA] Connected to %s (tenant: %s)", c.config.Name, c.config.TenantName)
	return nil
}

func (c *EntraConnector) Disconnect(ctx context.Context) error {
	c.token = ""
	c.config.Status = ConnectorStatusDisconnected
	return nil
}

func (c *EntraConnector) TestConnection(ctx context.Context) error {
	_, err := c.graphGet(ctx, "/users", url.Values{"$top": {"1"}, "$select": {"id"}})
	return err
}

// ─── User Operations ─────────────────────────────────────────

func (c *EntraConnector) ListUsers(ctx context.Context) ([]ConnectorUser, error) {
	var users []ConnectorUser
	nextLink := "/users?$top=100&$select=id,userPrincipalName,displayName,givenName,surname,department,jobTitle,mobilePhone,businessPhones,streetAddress,city,state,postalCode,country,employeeId,company,mail,userType,accountEnabled,createdDateTime,lastPasswordChangeDateTime&$expand=manager($select=id)"

	for nextLink != "" {
		body, err := c.graphGet(ctx, nextLink, nil)
		if err != nil {
			return users, err
		}

		var result struct {
			Value    []json.RawMessage `json:"value"`
			NextLink string            `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("entra: decode users: %w", err)
		}

		for _, raw := range result.Value {
			var u struct {
				ID               string `json:"id"`
				UserPrincipalName string `json:"userPrincipalName"`
				DisplayName      string `json:"displayName"`
				GivenName        string `json:"givenName"`
				Surname          string `json:"surname"`
				Department       string `json:"department"`
				JobTitle         string `json:"jobTitle"`
				MobilePhone      string `json:"mobilePhone"`
				BusinessPhones   []string `json:"businessPhones"`
				StreetAddress    string `json:"streetAddress"`
				City             string `json:"city"`
				State            string `json:"state"`
				PostalCode       string `json:"postalCode"`
				Country          string `json:"country"`
				EmployeeID       string `json:"employeeId"`
				Company          string `json:"company"`
				Mail             string `json:"mail"`
				UserType         string `json:"userType"`
				AccountEnabled   bool   `json:"accountEnabled"`
				CreatedDateTime  string `json:"createdDateTime"`
				Manager          struct {
					ID string `json:"id"`
				} `json:"manager"`
			}
			if err := json.Unmarshal(raw, &u); err != nil {
				continue
			}

			phone := ""
			if len(u.BusinessPhones) > 0 {
				phone = u.BusinessPhones[0]
			}

			email := u.Mail
			if email == "" {
				email = u.UserPrincipalName
			}

			createdAt, _ := time.Parse(time.RFC3339, u.CreatedDateTime)

			users = append(users, ConnectorUser{
				ExternalID:    u.ID,
				Username:      u.UserPrincipalName,
				Email:         email,
				DisplayName:   u.DisplayName,
				FirstName:     u.GivenName,
				LastName:      u.Surname,
				Department:    u.Department,
				Title:         u.JobTitle,
				Phone:         phone,
				Mobile:        u.MobilePhone,
				StreetAddress: u.StreetAddress,
				City:          u.City,
				State:         u.State,
				ZipCode:       u.PostalCode,
				Country:       u.Country,
				EmployeeID:    u.EmployeeID,
				Company:       u.Company,
				Enabled:       u.AccountEnabled,
				Manager:       u.Manager.ID,
				CreatedAt:     createdAt,
			})
		}

		if result.NextLink != "" {
			// Strip the base URL to get the relative path
			if u, err := url.Parse(result.NextLink); err == nil {
				nextLink = u.RequestURI()
			} else {
				nextLink = ""
			}
		} else {
			nextLink = ""
		}
	}

	return users, nil
}

func (c *EntraConnector) GetUser(ctx context.Context, externalID string) (*ConnectorUser, error) {
	body, err := c.graphGet(ctx, "/users/"+url.PathEscape(externalID), url.Values{
		"$select": {"id,userPrincipalName,displayName,givenName,surname,department,jobTitle,mail,accountEnabled,employeeId,company,mobilePhone,businessPhones,streetAddress,city,state,postalCode,country,createdDateTime"}})
	if err != nil {
		return nil, err
	}

	var u struct {
		ID               string   `json:"id"`
		UserPrincipalName string  `json:"userPrincipalName"`
		DisplayName      string   `json:"displayName"`
		GivenName        string   `json:"givenName"`
		Surname          string   `json:"surname"`
		Department       string   `json:"department"`
		JobTitle         string   `json:"jobTitle"`
		Mail             string   `json:"mail"`
		AccountEnabled   bool     `json:"accountEnabled"`
		EmployeeID       string   `json:"employeeId"`
		Company          string   `json:"company"`
		MobilePhone      string   `json:"mobilePhone"`
		BusinessPhones   []string `json:"businessPhones"`
		StreetAddress    string   `json:"streetAddress"`
		City             string   `json:"city"`
		State            string   `json:"state"`
		PostalCode       string   `json:"postalCode"`
		Country          string   `json:"country"`
		CreatedDateTime  string   `json:"createdDateTime"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("entra: decode user: %w", err)
	}

	email := u.Mail
	if email == "" {
		email = u.UserPrincipalName
	}
	phone := ""
	if len(u.BusinessPhones) > 0 {
		phone = u.BusinessPhones[0]
	}
	createdAt, _ := time.Parse(time.RFC3339, u.CreatedDateTime)

	return &ConnectorUser{
		ExternalID:    u.ID,
		Username:      u.UserPrincipalName,
		Email:         email,
		DisplayName:   u.DisplayName,
		FirstName:     u.GivenName,
		LastName:      u.Surname,
		Department:    u.Department,
		Title:         u.JobTitle,
		Phone:         phone,
		Mobile:        u.MobilePhone,
		StreetAddress: u.StreetAddress,
		City:          u.City,
		State:         u.State,
		ZipCode:       u.PostalCode,
		Country:       u.Country,
		EmployeeID:    u.EmployeeID,
		Company:       u.Company,
		Enabled:       u.AccountEnabled,
		CreatedAt:     createdAt,
	}, nil
}

func (c *EntraConnector) GetUserByUsername(ctx context.Context, username string) (*ConnectorUser, error) {
	body, err := c.graphGet(ctx, "/users/"+url.PathEscape(username), nil)
	if err != nil {
		// Try filtering by userPrincipalName
		body2, err2 := c.graphGet(ctx, "/users", url.Values{
			"$filter": {fmt.Sprintf("userPrincipalName eq '%s'", username)},
			"$top":    {"1"},
		})
		if err2 != nil {
			return nil, fmt.Errorf("entra: user not found: %s", username)
		}
		var result struct {
			Value []json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(body2, &result); err != nil || len(result.Value) == 0 {
			return nil, fmt.Errorf("entra: user not found: %s", username)
		}
		// Re-fetch using the found ID
		var first struct{ ID string }
		json.Unmarshal(result.Value[0], &first)
		return c.GetUser(ctx, first.ID)
	}

	var u struct{ ID string }
	json.Unmarshal(body, &u)
	return c.GetUser(ctx, u.ID)
}

func (c *EntraConnector) CreateUser(ctx context.Context, user ConnectorUser) (string, error) {
	payload := map[string]any{
		"accountEnabled":    user.Enabled,
		"displayName":       user.DisplayName,
		"mailNickname":      user.Username,
		"userPrincipalName": user.Username,
		"givenName":         user.FirstName,
		"surname":           user.LastName,
		"department":        user.Department,
		"jobTitle":          user.Title,
		"mail":              user.Email,
		"mobilePhone":       user.Mobile,
		"employeeId":        user.EmployeeID,
		"company":           user.Company,
		"passwordProfile": map[string]any{
			"forceChangePasswordNextSignIn": true,
			"password":                      generateTempPassword(),
		},
	}

	body, err := c.graphPost(ctx, "/users", payload)
	if err != nil {
		return "", err
	}

	var created struct{ ID string }
	if err := json.Unmarshal(body, &created); err != nil {
		return "", fmt.Errorf("entra: decode created user: %w", err)
	}
	return created.ID, nil
}

func (c *EntraConnector) UpdateUser(ctx context.Context, externalID string, user ConnectorUser) error {
	payload := map[string]any{}
	setIf(&payload, "displayName", user.DisplayName)
	setIf(&payload, "givenName", user.FirstName)
	setIf(&payload, "surname", user.LastName)
	setIf(&payload, "department", user.Department)
	setIf(&payload, "jobTitle", user.Title)
	setIf(&payload, "mail", user.Email)
	setIf(&payload, "mobilePhone", user.Mobile)
	setIf(&payload, "employeeId", user.EmployeeID)
	setIf(&payload, "company", user.Company)
	setIf(&payload, "streetAddress", user.StreetAddress)
	setIf(&payload, "city", user.City)
	setIf(&payload, "state", user.State)
	setIf(&payload, "postalCode", user.ZipCode)
	setIf(&payload, "country", user.Country)

	externalID = url.PathEscape(externalID)
	if user.Username != "" {
		externalID = url.PathEscape(user.Username)
	}
	return c.graphPatch(ctx, "/users/"+externalID, payload)
}

func (c *EntraConnector) DeleteUser(ctx context.Context, externalID string) error {
	return c.graphDelete(ctx, "/users/"+url.PathEscape(externalID))
}

func (c *EntraConnector) DisableUser(ctx context.Context, externalID string) error {
	return c.graphPatch(ctx, "/users/"+url.PathEscape(externalID), map[string]any{
		"accountEnabled": false,
	})
}

func (c *EntraConnector) EnableUser(ctx context.Context, externalID string) error {
	return c.graphPatch(ctx, "/users/"+url.PathEscape(externalID), map[string]any{
		"accountEnabled": true,
	})
}

// ─── Group Operations ────────────────────────────────────────

func (c *EntraConnector) ListGroups(ctx context.Context) ([]ConnectorGroup, error) {
	var groups []ConnectorGroup
	nextLink := "/groups?$top=100&$select=id,displayName,description,groupTypes,mailEnabled,securityEnabled,visibility,createdDateTime&$expand=members($select=id)"

	for nextLink != "" {
		body, err := c.graphGet(ctx, nextLink, nil)
		if err != nil {
			return groups, err
		}

		var result struct {
			Value    []json.RawMessage `json:"value"`
			NextLink string            `json:"@odata.nextLink"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("entra: decode groups: %w", err)
		}

		for _, raw := range result.Value {
			var g struct {
				ID            string         `json:"id"`
				DisplayName   string         `json:"displayName"`
				Description   string         `json:"description"`
				GroupTypes    []string       `json:"groupTypes"`
				MailEnabled   bool           `json:"mailEnabled"`
				SecurityEnabled bool         `json:"securityEnabled"`
				Visibility    string         `json:"visibility"`
				CreatedDateTime string       `json:"createdDateTime"`
				Members       []struct {
					ID string `json:"id"`
				} `json:"members"`
			}
			if err := json.Unmarshal(raw, &g); err != nil {
				continue
			}

			groupType := "distribution"
			if g.SecurityEnabled {
				groupType = "security"
			}
			for _, t := range g.GroupTypes {
				if t == "Unified" {
					groupType = "microsoft_365"
				}
			}

			var memberIDs []string
			for _, m := range g.Members {
				memberIDs = append(memberIDs, m.ID)
			}

			createdAt, _ := time.Parse(time.RFC3339, g.CreatedDateTime)

			groups = append(groups, ConnectorGroup{
				ExternalID:  g.ID,
				Name:        g.DisplayName,
				Description: g.Description,
				Type:        groupType,
				Scope:       g.Visibility,
				Members:     memberIDs,
				CreatedAt:   createdAt,
			})
		}

		if result.NextLink != "" {
			if u, err := url.Parse(result.NextLink); err == nil {
				nextLink = u.RequestURI()
			} else {
				nextLink = ""
			}
		} else {
			nextLink = ""
		}
	}

	return groups, nil
}

func (c *EntraConnector) GetGroup(ctx context.Context, externalID string) (*ConnectorGroup, error) {
	body, err := c.graphGet(ctx, "/groups/"+url.PathEscape(externalID), url.Values{
		"$select": {"id,displayName,description,groupTypes,mailEnabled,securityEnabled,visibility,createdDateTime"},
		"$expand": {"members($select=id)"},
	})
	if err != nil {
		return nil, err
	}

	var g struct {
		ID            string   `json:"id"`
		DisplayName   string   `json:"displayName"`
		Description   string   `json:"description"`
		GroupTypes    []string `json:"groupTypes"`
		MailEnabled   bool     `json:"mailEnabled"`
		SecurityEnabled bool   `json:"securityEnabled"`
		Visibility    string   `json:"visibility"`
		CreatedDateTime string `json:"createdDateTime"`
	}
	if err := json.Unmarshal(body, &g); err != nil {
		return nil, fmt.Errorf("entra: decode group: %w", err)
	}

	groupType := "distribution"
	if g.SecurityEnabled {
		groupType = "security"
	}
	for _, t := range g.GroupTypes {
		if t == "Unified" {
			groupType = "microsoft_365"
		}
	}

	createdAt, _ := time.Parse(time.RFC3339, g.CreatedDateTime)

	return &ConnectorGroup{
		ExternalID:  g.ID,
		Name:        g.DisplayName,
		Description: g.Description,
		Type:        groupType,
		Scope:       g.Visibility,
		CreatedAt:   createdAt,
	}, nil
}

func (c *EntraConnector) CreateGroup(ctx context.Context, group ConnectorGroup) (string, error) {
	groupTypes := []string{}
	if group.Type == "microsoft_365" {
		groupTypes = append(groupTypes, "Unified")
	}

	payload := map[string]any{
		"displayName":     group.Name,
		"description":     group.Description,
		"mailEnabled":     false,
		"securityEnabled": group.Type == "security" || group.Type == "microsoft_365",
		"groupTypes":      groupTypes,
	}

	body, err := c.graphPost(ctx, "/groups", payload)
	if err != nil {
		return "", err
	}

	var created struct{ ID string }
	json.Unmarshal(body, &created)
	return created.ID, nil
}

func (c *EntraConnector) UpdateGroup(ctx context.Context, externalID string, group ConnectorGroup) error {
	payload := map[string]any{}
	setIf(&payload, "displayName", group.Name)
	setIf(&payload, "description", group.Description)
	return c.graphPatch(ctx, "/groups/"+url.PathEscape(externalID), payload)
}

func (c *EntraConnector) DeleteGroup(ctx context.Context, externalID string) error {
	return c.graphDelete(ctx, "/groups/"+url.PathEscape(externalID))
}

func (c *EntraConnector) AddGroupMember(ctx context.Context, groupID, userID string) error {
	payload := map[string]string{
		"@odata.id": fmt.Sprintf("https://graph.microsoft.com/v1.0/directoryObjects/%s", userID),
	}
	_, err := c.graphPost(ctx, "/groups/"+url.PathEscape(groupID)+"/members/$ref", payload)
	return err
}

func (c *EntraConnector) RemoveGroupMember(ctx context.Context, groupID, userID string) error {
	return c.graphDelete(ctx, "/groups/"+url.PathEscape(groupID)+"/members/"+url.PathEscape(userID)+"/$ref")
}

// ─── Delta Sync (Microsoft Graph Delta Query) ─────────────────

func (c *EntraConnector) ListUsersDelta(ctx context.Context, deltaToken string) ([]ConnectorUser, string, error) {
	path := "/users/delta"
	if deltaToken != "" {
		path += "?$deltatoken=" + url.QueryEscape(deltaToken)
	}

	var users []ConnectorUser
	body, err := c.graphGet(ctx, path, nil)
	if err != nil {
		return nil, "", fmt.Errorf("entra: delta query: %w", err)
	}

	var resp struct {
		Value     []json.RawMessage `json:"value"`
		DeltaLink string            `json:"@odata.deltaLink"`
		NextLink  string            `json:"@odata.nextLink"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, "", fmt.Errorf("entra: delta decode: %w", err)
	}

	for _, raw := range resp.Value {
		var u struct {
			ID              string   `json:"id"`
			UserPrincipalName string `json:"userPrincipalName"`
			DisplayName     string   `json:"displayName"`
			GivenName       string   `json:"givenName"`
			Surname         string   `json:"surname"`
			Department      string   `json:"department"`
			JobTitle        string   `json:"jobTitle"`
			Mail            string   `json:"mail"`
			AccountEnabled  bool     `json:"accountEnabled"`
			CreatedDateTime string   `json:"createdDateTime"`
			EmployeeID      string   `json:"employeeId"`
			Company         string   `json:"company"`
			MobilePhone     string   `json:"mobilePhone"`
			BusinessPhones  []string `json:"businessPhones"`
			Manager         struct{ ID string } `json:"manager"`
			Removed         json.RawMessage `json:"@removed"`
		}
		if err := json.Unmarshal(raw, &u); err != nil {
			continue
		}

		// Skip deleted/removed users in delta results
		if len(u.Removed) > 0 {
			continue
		}

		phone := ""
		if len(u.BusinessPhones) > 0 {
			phone = u.BusinessPhones[0]
		}
		email := u.Mail
		if email == "" {
			email = u.UserPrincipalName
		}
		createdAt, _ := time.Parse(time.RFC3339, u.CreatedDateTime)

		users = append(users, ConnectorUser{
			ExternalID:    u.ID,
			Username:      u.UserPrincipalName,
			Email:         email,
			DisplayName:   u.DisplayName,
			FirstName:     u.GivenName,
			LastName:      u.Surname,
			Department:    u.Department,
			Title:         u.JobTitle,
			Phone:         phone,
			Mobile:        u.MobilePhone,
			EmployeeID:    u.EmployeeID,
			Company:       u.Company,
			Enabled:       u.AccountEnabled,
			Manager:       u.Manager.ID,
			CreatedAt:     createdAt,
		})
	}

	newToken := extractDeltaToken(resp.DeltaLink)
	return users, newToken, nil
}

// extractDeltaToken extracts the $deltatoken parameter from a Microsoft Graph delta link.
func extractDeltaToken(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}
	return u.Query().Get("$deltatoken")
}

// ─── Schema Discovery ────────────────────────────────────────

func (c *EntraConnector) DiscoverSchema(ctx context.Context) (*SchemaResult, error) {
	return &SchemaResult{
		ObjectType: "User",
		Count:      32,
		Attributes: []AttributeSchema{
			{Name: "external_id", Type: "string", Required: true, Description: "Object ID in Microsoft Graph"},
			{Name: "username", Type: "string", Required: true, Description: "User Principal Name (UPN)"},
			{Name: "email", Type: "string", Required: true, Description: "Primary email address"},
			{Name: "display_name", Type: "string", Required: true, Description: "Display name"},
			{Name: "first_name", Type: "string", Required: false, Description: "Given name"},
			{Name: "last_name", Type: "string", Required: false, Description: "Surname"},
			{Name: "department", Type: "string", Required: false, Description: "Department"},
			{Name: "title", Type: "string", Required: false, Description: "Job title"},
			{Name: "employee_id", Type: "string", Required: false, Description: "Employee ID"},
			{Name: "manager", Type: "string", Required: false, Description: "Manager's object ID"},
			{Name: "phone", Type: "string", Required: false, Description: "Business phone"},
			{Name: "mobile", Type: "string", Required: false, Description: "Mobile phone"},
			{Name: "enabled", Type: "boolean", Required: true, Description: "Account enabled status"},
			{Name: "locked", Type: "boolean", Required: false, Description: "Account locked"},
			{Name: "company", Type: "string", Required: false, Description: "Company name"},
			{Name: "division", Type: "string", Required: false, Description: "Division"},
			{Name: "cost_center", Type: "string", Required: false, Description: "Cost center"},
			{Name: "street_address", Type: "string", Required: false, Description: "Street address"},
			{Name: "city", Type: "string", Required: false, Description: "City"},
			{Name: "state", Type: "string", Required: false, Description: "State/Province"},
			{Name: "zip_code", Type: "string", Required: false, Description: "Postal code"},
			{Name: "country", Type: "string", Required: false, Description: "Country"},
			{Name: "groups", Type: "string_list", Required: false, MultiValued: true, Description: "Group object IDs"},
			{Name: "roles", Type: "string_list", Required: false, MultiValued: true, Description: "Directory roles"},
			{Name: "created_at", Type: "datetime", Required: false, Description: "Created timestamp"},
			{Name: "updated_at", Type: "datetime", Required: false, Description: "Updated timestamp"},
			{Name: "attributes.saml_account_name", Type: "string", Required: false, Description: "On-premises SAM account name"},
			{Name: "attributes.proxy_addresses", Type: "string_list", Required: false, Description: "Proxy addresses"},
			{Name: "attributes.usage_location", Type: "string", Required: false, Description: "Usage location"},
			{Name: "attributes.office_location", Type: "string", Required: false, Description: "Office location"},
			{Name: "attributes.preferred_language", Type: "string", Required: false, Description: "Preferred language"},
			{Name: "attributes.user_type", Type: "string", Required: false, Description: "User type (member/guest)"},
		},
	}, nil
}

// ─── Helpers ─────────────────────────────────────────────────

func setIf(m *map[string]any, key, val string) {
	if val != "" {
		(*m)[key] = val
	}
}

func generateTempPassword() string {
	return "ObserveID_" + fmt.Sprintf("%d", time.Now().UnixNano())[:10] + "!Aa1"
}
