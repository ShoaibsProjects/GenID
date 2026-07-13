package connector

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─── CSV Connector ───────────────────────────────────────────
// Reads identities from a local CSV file.
// One-shot connector: Connect validates, ListUsers returns all rows.

type CSVConnector struct {
	config ConnectorConfig
}

// forbiddenPaths are system paths that CSV connector must never read
var forbiddenPaths = []string{
	"/etc/", "/dev/", "/proc/", "/sys/", "/boot/",
	"/var/log/", "/var/db/", "/private/etc/",
}

func sanitizeCSVPath(raw string) (string, error) {
	clean := filepath.Clean(raw)
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("csv: path traversal detected in %q", raw)
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("csv: invalid path: %w", err)
	}
	for _, forbidden := range forbiddenPaths {
		if strings.HasPrefix(abs, forbidden) {
			return "", fmt.Errorf("csv: access to %s is not allowed", forbidden)
		}
	}
	return abs, nil
}

func NewCSVConnector() *CSVConnector {
	return &CSVConnector{}
}

func (c *CSVConnector) Type() ConnectorType                  { return ConnectorTypeCSV }
func (c *CSVConnector) Name() string                         { return c.config.Name }
func (c *CSVConnector) GetStatus(ctx context.Context) ConnectorStatus { return c.config.Status }

func (c *CSVConnector) Configure(config ConnectorConfig) error {
	if config.Endpoint != "" {
		sanitized, err := sanitizeCSVPath(config.Endpoint)
		if err != nil {
			return err
		}
		config.Endpoint = sanitized
	} else {
		if config.Properties == nil || config.Properties["csv_data"] == "" {
			return fmt.Errorf("csv: endpoint (file path) or properties.csv_data is required")
		}
	}
	c.config = config
	return nil
}

func (c *CSVConnector) Connect(ctx context.Context) error {
	if c.config.Endpoint != "" {
		sanitized, err := sanitizeCSVPath(c.config.Endpoint)
		if err != nil {
			return err
		}
		if _, err := os.Stat(sanitized); err != nil {
			return fmt.Errorf("csv: file not found: %s: %w", sanitized, err)
		}
	}
	c.config.Status = ConnectorStatusConnected
	log.Printf("[CSV] Connected to %s", c.config.Name)
	return nil
}

func (c *CSVConnector) Disconnect(ctx context.Context) error {
	c.config.Status = ConnectorStatusDisconnected
	return nil
}

func (c *CSVConnector) TestConnection(ctx context.Context) error {
	_, err := c.parseCSV()
	return err
}

func (c *CSVConnector) parseCSV() ([]ConnectorUser, error) {
	var reader *csv.Reader

	if c.config.Endpoint != "" {
		f, err := os.Open(c.config.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("csv: open %s: %w", c.config.Endpoint, err)
		}
		defer f.Close()
		reader = csv.NewReader(f)
	} else if c.config.Properties != nil && c.config.Properties["csv_data"] != "" {
		reader = csv.NewReader(strings.NewReader(c.config.Properties["csv_data"]))
	} else {
		return nil, fmt.Errorf("csv: no file path or embedded data")
	}

	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv: parse error: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("csv: file must have a header row and at least one data row")
	}

	header := make([]string, len(records[0]))
	for i, h := range records[0] {
		header[i] = strings.TrimSpace(strings.ToLower(h))
	}

	colIndex := make(map[string]int, len(header))
	for i, h := range header {
		colIndex[h] = i
	}

	get := func(row []string, keys ...string) string {
		for _, k := range keys {
			if idx, ok := colIndex[k]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
		}
		return ""
	}

	now := time.Now()
	var users []ConnectorUser

	for lineIdx, row := range records[1:] {
		email := get(row, "email", "e-mail", "mail")
		displayName := get(row, "display_name", "displayname", "name", "full_name", "fullname")

		if email == "" && displayName == "" {
			continue
		}

		externalID := get(row, "external_id", "id", "user_id", "employee_id")
		if externalID == "" {
			externalID = fmt.Sprintf("csv-%d", lineIdx+1)
		}

		enabled := true
		enabledStr := get(row, "enabled", "active", "status")
		if enabledStr != "" {
			if enabledStr == "false" || enabledStr == "0" || enabledStr == "inactive" || enabledStr == "disabled" {
				enabled = false
			}
		}

		user := ConnectorUser{
			ExternalID:    externalID,
			Username:      get(row, "username", "user_name", "login", "samaccount_name"),
			Email:         email,
			DisplayName:   displayName,
			FirstName:     get(row, "first_name", "firstname", "given_name", "givenname"),
			LastName:      get(row, "last_name", "lastname", "family_name", "familyname", "surname"),
			Department:    get(row, "department", "dept"),
			Manager:       get(row, "manager"),
			Title:         get(row, "title", "job_title", "jobtitle"),
			Phone:         get(row, "phone", "telephone", "phone_number"),
			Mobile:        get(row, "mobile", "mobile_phone", "cell"),
			StreetAddress: get(row, "street_address", "streetaddress", "street", "address"),
			City:          get(row, "city"),
			State:         get(row, "state", "region"),
			ZipCode:       get(row, "zip_code", "zipcode", "zip", "postal_code"),
			Country:       get(row, "country"),
			EmployeeID:    get(row, "employee_id", "employeeid", "emp_id"),
			CostCenter:    get(row, "cost_center", "costcenter"),
			Division:      get(row, "division", "div"),
			Company:       get(row, "company"),
			Enabled:       enabled,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		// Groups — split by semicolon or pipe
		groupsStr := get(row, "groups", "group", "member_of")
		if groupsStr != "" {
			sep := ";"
			if !strings.Contains(groupsStr, ";") && strings.Contains(groupsStr, "|") {
				sep = "|"
			}
			for _, g := range strings.Split(groupsStr, sep) {
				g = strings.TrimSpace(g)
				if g != "" {
					user.Groups = append(user.Groups, g)
				}
			}
		}

		// Additional attributes — capture any column not in the known set
		known := map[string]bool{
			"external_id": true, "id": true, "email": true, "e-mail": true, "mail": true,
			"display_name": true, "displayname": true, "name": true, "full_name": true, "fullname": true,
			"username": true, "user_name": true, "login": true,
			"first_name": true, "firstname": true, "given_name": true, "givenname": true,
			"last_name": true, "lastname": true, "family_name": true, "familyname": true, "surname": true,
			"department": true, "dept": true, "manager": true,
			"title": true, "job_title": true, "jobtitle": true,
			"phone": true, "telephone": true, "phone_number": true,
			"mobile": true, "mobile_phone": true, "cell": true,
			"street_address": true, "streetaddress": true, "street": true, "address": true,
			"city": true, "state": true, "region": true,
			"zip_code": true, "zipcode": true, "zip": true, "postal_code": true,
			"country": true,
			"employee_id": true, "employeeid": true, "emp_id": true,
			"cost_center": true, "costcenter": true,
			"division": true, "div": true, "company": true,
			"enabled": true, "active": true, "status": true,
			"groups": true, "group": true, "member_of": true,
			"type": true, "source": true,
		}

		attrs := make(map[string]string)
		for h, idx := range colIndex {
			if !known[h] && idx < len(row) {
				val := strings.TrimSpace(row[idx])
				if val != "" {
					attrs[h] = val
				}
			}
		}
		if len(attrs) > 0 {
			user.Attributes = attrs
		}

		users = append(users, user)
	}

	return users, nil
}

// ─── User Operations ─────────────────────────────────────────

func (c *CSVConnector) ListUsers(ctx context.Context) ([]ConnectorUser, error) {
	return c.parseCSV()
}

func (c *CSVConnector) GetUser(ctx context.Context, externalID string) (*ConnectorUser, error) {
	users, err := c.parseCSV()
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.ExternalID == externalID {
			return &u, nil
		}
	}
	return nil, fmt.Errorf("csv: user %s not found", externalID)
}

func (c *CSVConnector) GetUserByUsername(ctx context.Context, username string) (*ConnectorUser, error) {
	users, err := c.parseCSV()
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.Username == username || u.Email == username {
			return &u, nil
		}
	}
	return nil, fmt.Errorf("csv: user %s not found", username)
}

func (c *CSVConnector) CreateUser(ctx context.Context, user ConnectorUser) (string, error) {
	return "", ErrNotSupported
}

func (c *CSVConnector) UpdateUser(ctx context.Context, externalID string, user ConnectorUser) error {
	return ErrNotSupported
}

func (c *CSVConnector) DeleteUser(ctx context.Context, externalID string) error {
	return ErrNotSupported
}

func (c *CSVConnector) DisableUser(ctx context.Context, externalID string) error {
	return ErrNotSupported
}

func (c *CSVConnector) EnableUser(ctx context.Context, externalID string) error {
	return ErrNotSupported
}

// ─── Group Operations ────────────────────────────────────────

func (c *CSVConnector) ListGroups(ctx context.Context) ([]ConnectorGroup, error) {
	// Derive groups from user rows
	users, err := c.parseCSV()
	if err != nil {
		return nil, err
	}
	groupSet := make(map[string]bool)
	for _, u := range users {
		for _, g := range u.Groups {
			groupSet[g] = true
		}
	}
	var groups []ConnectorGroup
	for name := range groupSet {
		groups = append(groups, ConnectorGroup{
			ExternalID: name,
			Name:       name,
		})
	}
	if groups == nil {
		return nil, ErrNotSupported
	}
	return groups, nil
}

func (c *CSVConnector) GetGroup(ctx context.Context, externalID string) (*ConnectorGroup, error) {
	return nil, ErrNotSupported
}

func (c *CSVConnector) CreateGroup(ctx context.Context, group ConnectorGroup) (string, error) {
	return "", ErrNotSupported
}

func (c *CSVConnector) UpdateGroup(ctx context.Context, externalID string, group ConnectorGroup) error {
	return ErrNotSupported
}

func (c *CSVConnector) DeleteGroup(ctx context.Context, externalID string) error {
	return ErrNotSupported
}

func (c *CSVConnector) AddGroupMember(ctx context.Context, groupID, userID string) error {
	return ErrNotSupported
}

func (c *CSVConnector) RemoveGroupMember(ctx context.Context, groupID, userID string) error {
	return ErrNotSupported
}

// ─── Delta / Incremental Sync ────────────────────────────────

func (c *CSVConnector) ListUsersDelta(ctx context.Context, deltaToken string) ([]ConnectorUser, string, error) {
	return nil, "", ErrDeltaNotSupported
}

// ─── Entitlements (not supported for CSV) ────────────────────

func (c *CSVConnector) ListEntitlements(ctx context.Context) ([]ConnectorEntitlement, error) {
	return nil, ErrNotSupported
}

// ─── Resources (not supported for CSV) ───────────────────────

func (c *CSVConnector) ListResources(ctx context.Context) ([]ConnectorResource, error) {
	return nil, ErrNotSupported
}

// ─── Schema Discovery ────────────────────────────────────────

func (c *CSVConnector) DiscoverSchema(ctx context.Context) (*SchemaResult, error) {
	users, err := c.parseCSV()
	if err != nil {
		// Return default schema even if no data
		return c.defaultSchema(), nil
	}

	attrs := []AttributeSchema{
		{Name: "external_id", Type: "string", Required: false, Description: "Unique row identifier"},
		{Name: "username", Type: "string", Required: false, Description: "Login / username"},
		{Name: "email", Type: "string", Required: true, Description: "Primary email address"},
		{Name: "display_name", Type: "string", Required: true, Description: "Full display name"},
		{Name: "first_name", Type: "string", Required: false, Description: "Given name"},
		{Name: "last_name", Type: "string", Required: false, Description: "Family name"},
		{Name: "department", Type: "string", Required: false, Description: "Department"},
		{Name: "title", Type: "string", Required: false, Description: "Job title"},
		{Name: "employee_id", Type: "string", Required: false, Description: "Employee ID"},
		{Name: "manager", Type: "string", Required: false, Description: "Manager name or email"},
		{Name: "phone", Type: "string", Required: false, Description: "Phone number"},
		{Name: "mobile", Type: "string", Required: false, Description: "Mobile phone"},
		{Name: "street_address", Type: "string", Required: false, Description: "Street address"},
		{Name: "city", Type: "string", Required: false, Description: "City"},
		{Name: "state", Type: "string", Required: false, Description: "State / region"},
		{Name: "zip_code", Type: "string", Required: false, Description: "Postal / zip code"},
		{Name: "country", Type: "string", Required: false, Description: "Country"},
		{Name: "cost_center", Type: "string", Required: false, Description: "Cost center"},
		{Name: "division", Type: "string", Required: false, Description: "Division"},
		{Name: "company", Type: "string", Required: false, Description: "Company name"},
		{Name: "groups", Type: "string_list", Required: false, MultiValued: true, Description: "Group memberships (semicolon-separated)"},
		{Name: "enabled", Type: "boolean", Required: false, Description: "Whether the account is active"},
	}

	// Add any extra columns from the CSV
	if len(users) > 0 && users[0].Attributes != nil {
		existing := map[string]bool{"external_id": true, "username": true, "email": true, "display_name": true,
			"first_name": true, "last_name": true, "department": true, "title": true, "employee_id": true,
			"manager": true, "phone": true, "mobile": true, "street_address": true, "city": true,
			"state": true, "zip_code": true, "country": true, "cost_center": true, "division": true,
			"company": true, "groups": true, "enabled": true}
		for k := range users[0].Attributes {
			if !existing[k] {
				attrs = append(attrs, AttributeSchema{
					Name: k, Type: "string", Required: false, Description: "Custom attribute",
				})
			}
		}
	}

	return &SchemaResult{
		ObjectType: "User",
		Count:      len(attrs),
		Attributes: attrs,
	}, nil
}

func (c *CSVConnector) defaultSchema() *SchemaResult {
	return &SchemaResult{
		ObjectType: "User",
		Count:      22,
		Attributes: []AttributeSchema{
			{Name: "external_id", Type: "string", Required: false},
			{Name: "username", Type: "string", Required: false},
			{Name: "email", Type: "string", Required: true},
			{Name: "display_name", Type: "string", Required: true},
			{Name: "first_name", Type: "string", Required: false},
			{Name: "last_name", Type: "string", Required: false},
			{Name: "department", Type: "string", Required: false},
			{Name: "title", Type: "string", Required: false},
			{Name: "employee_id", Type: "string", Required: false},
			{Name: "manager", Type: "string", Required: false},
			{Name: "phone", Type: "string", Required: false},
			{Name: "mobile", Type: "string", Required: false},
			{Name: "street_address", Type: "string", Required: false},
			{Name: "city", Type: "string", Required: false},
			{Name: "state", Type: "string", Required: false},
			{Name: "zip_code", Type: "string", Required: false},
			{Name: "country", Type: "string", Required: false},
			{Name: "cost_center", Type: "string", Required: false},
			{Name: "division", Type: "string", Required: false},
			{Name: "company", Type: "string", Required: false},
			{Name: "groups", Type: "string_list", Required: false, MultiValued: true},
			{Name: "enabled", Type: "boolean", Required: false},
		},
	}
}
