package connector

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// ─── LDAP / Active Directory Connector ───────────────────────
// Connects to any LDAP-compatible directory (Active Directory, OpenLDAP, FreeIPA, etc.)
// Supports both LDAP and LDAPS (SSL/TLS and STARTTLS).

type LDAPConnector struct {
	config ConnectorConfig
	conn   *ldap.Conn
}

func NewLDAPConnector() *LDAPConnector {
	return &LDAPConnector{}
}

func (c *LDAPConnector) Type() ConnectorType         { return ConnectorTypeLDAP }
func (c *LDAPConnector) Name() string                { return c.config.Name }
func (c *LDAPConnector) GetStatus(ctx context.Context) ConnectorStatus { return c.config.Status }

func (c *LDAPConnector) Configure(config ConnectorConfig) error {
	if config.Endpoint == "" {
		return fmt.Errorf("ldap: endpoint (host:port) is required")
	}
	if config.BaseDN == "" {
		return fmt.Errorf("ldap: base_dn is required")
	}
	if config.Username == "" || config.Password == "" {
		return fmt.Errorf("ldap: username and password are required")
	}
	if config.Domain != "" {
		config.Username = config.Domain + "\\" + config.Username
	}
	c.config = config
	return nil
}

// ─── Connection ──────────────────────────────────────────────

func (c *LDAPConnector) Connect(ctx context.Context) error {
	host := c.config.Endpoint
	host = strings.TrimPrefix(host, "ldap://")
	host = strings.TrimPrefix(host, "ldaps://")

	var err error
	if strings.HasSuffix(c.config.Endpoint, ":636") || strings.HasPrefix(c.config.Endpoint, "ldaps://") {
		c.conn, err = ldap.DialTLS("tcp", host, &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         strings.Split(host, ":")[0],
		})
	} else {
		c.conn, err = ldap.Dial("tcp", host)
		if err == nil {
			// Attempt STARTTLS
			err = c.conn.StartTLS(&tls.Config{
				InsecureSkipVerify: false,
				ServerName:         strings.Split(host, ":")[0],
			})
			if err != nil {
				// Non-TLS is acceptable for some LDAP servers
				log.Printf("[LDAP] STARTTLS failed, continuing without TLS: %v", err)
				err = nil
			}
		}
	}
	if err != nil {
		c.config.Status = ConnectorStatusError
		return fmt.Errorf("ldap: dial failed: %w", err)
	}

	if err := c.conn.Bind(c.config.Username, c.config.Password); err != nil {
		c.conn.Close()
		c.conn = nil
		c.config.Status = ConnectorStatusError
		return fmt.Errorf("ldap: bind failed: %w", err)
	}

	c.config.Status = ConnectorStatusConnected
	log.Printf("[LDAP] Connected to %s (base: %s)", c.config.Name, c.config.BaseDN)
	return nil
}

func (c *LDAPConnector) Disconnect(ctx context.Context) error {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.config.Status = ConnectorStatusDisconnected
	return nil
}

func (c *LDAPConnector) TestConnection(ctx context.Context) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}
	defer c.Disconnect(ctx)
	return nil
}

// ─── LDAP Helpers ────────────────────────────────────────────

func (c *LDAPConnector) search(baseDN, filter string, attributes []string, scope int) ([]*ldap.Entry, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("ldap: not connected")
	}

	searchReq := ldap.NewSearchRequest(
		baseDN,
		scope,
		ldap.NeverDerefAliases,
		0,    // size limit (0 = unlimited)
		0,    // time limit
		false, // types only
		filter,
		attributes,
		nil,
	)

	result, err := c.conn.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("ldap: search failed: %w", err)
	}
	return result.Entries, nil
}

func (c *LDAPConnector) userAttrs() []string {
	return []string{
		"dn", "cn", "sAMAccountName", "userPrincipalName", "mail",
		"givenName", "sn", "displayName", "department", "title",
		"telephoneNumber", "mobile", "manager", "employeeID",
		"streetAddress", "l", "st", "postalCode", "co", "c",
		"company", "physicalDeliveryOfficeName", "distinguishedName",
		"memberOf", "objectCategory", "whenCreated", "whenChanged",
		"userAccountControl", "pwdLastSet", "accountExpires", "lockoutTime",
		"extensionAttribute1", "extensionAttribute2", "extensionAttribute3",
		"extensionAttribute4", "extensionAttribute5",
	}
}

func (c *LDAPConnector) groupAttrs() []string {
	return []string{
		"dn", "cn", "name", "description", "distinguishedName",
		"groupType", "member", "sAMAccountName", "mail",
		"whenCreated", "whenChanged", "objectCategory",
	}
}

func ldapEntryToConnectorUser(e *ldap.Entry) ConnectorUser {
	accountControl := parseAttrInt(e, "userAccountControl")
	enabled := !isAccountDisabled(accountControl)
	locked := parseAttrInt(e, "lockoutTime") > 0

	return ConnectorUser{
		ExternalID:    e.DN,
		Username:      getAttr(e, "userPrincipalName", getAttr(e, "sAMAccountName", "")),
		Email:         getAttr(e, "mail", ""),
		DisplayName:   getAttr(e, "displayName", getAttr(e, "cn", "")),
		FirstName:     getAttr(e, "givenName", ""),
		LastName:      getAttr(e, "sn", ""),
		Department:    getAttr(e, "department", ""),
		Title:         getAttr(e, "title", ""),
		Phone:         getAttr(e, "telephoneNumber", ""),
		Mobile:        getAttr(e, "mobile", ""),
		Manager:       getAttr(e, "manager", ""),
		EmployeeID:    getAttr(e, "employeeID", ""),
		StreetAddress: getAttr(e, "streetAddress", ""),
		City:          getAttr(e, "l", ""),
		State:         getAttr(e, "st", ""),
		ZipCode:       getAttr(e, "postalCode", ""),
		Country:       getAttr(e, "co", getAttr(e, "c", "")),
		Company:       getAttr(e, "company", ""),
		Enabled:       enabled,
		Locked:        locked,
		Groups:        getAttrs(e, "memberOf"),
		Attributes: map[string]string{
			"dn":                e.DN,
			"sAMAccountName":    getAttr(e, "sAMAccountName", ""),
			"distinguishedName": getAttr(e, "distinguishedName", ""),
			"extensionAttribute1": getAttr(e, "extensionAttribute1", ""),
			"extensionAttribute2": getAttr(e, "extensionAttribute2", ""),
			"extensionAttribute3": getAttr(e, "extensionAttribute3", ""),
			"extensionAttribute4": getAttr(e, "extensionAttribute4", ""),
			"extensionAttribute5": getAttr(e, "extensionAttribute5", ""),
		},
	}
}

// ─── User Operations ─────────────────────────────────────────

func (c *LDAPConnector) ListUsers(ctx context.Context) ([]ConnectorUser, error) {
	userFilter := "(&(objectClass=user)(objectCategory=person))"
	if c.config.Properties != nil && c.config.Properties["user_filter"] != "" {
		userFilter = c.config.Properties["user_filter"]
	}

	entries, err := c.search(c.config.BaseDN, userFilter, c.userAttrs(), ldap.ScopeWholeSubtree)
	if err != nil {
		return nil, err
	}

	var users []ConnectorUser
	for _, e := range entries {
		users = append(users, ldapEntryToConnectorUser(e))
	}
	return users, nil
}

func (c *LDAPConnector) GetUser(ctx context.Context, externalID string) (*ConnectorUser, error) {
	entries, err := c.search(externalID, "(objectClass=user)", c.userAttrs(), ldap.ScopeBaseObject)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("ldap: user not found: %s", externalID)
	}

	user := ldapEntryToConnectorUser(entries[0])
	return &user, nil
}

func (c *LDAPConnector) GetUserByUsername(ctx context.Context, username string) (*ConnectorUser, error) {
	filter := fmt.Sprintf("(|(userPrincipalName=%s)(sAMAccountName=%s)(mail=%s))",
		ldap.EscapeFilter(username), ldap.EscapeFilter(username), ldap.EscapeFilter(username))

	entries, err := c.search(c.config.BaseDN, filter, c.userAttrs(), ldap.ScopeWholeSubtree)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("ldap: user not found: %s", username)
	}

	user := ldapEntryToConnectorUser(entries[0])
	return &user, nil
}

func (c *LDAPConnector) CreateUser(ctx context.Context, user ConnectorUser) (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("ldap: not connected")
	}

	// Determine OU/container for new users
	userDN := fmt.Sprintf("cn=%s,%s", ldap.EscapeFilter(user.DisplayName), c.config.BaseDN)
	if c.config.Properties != nil && c.config.Properties["user_ou"] != "" {
		userDN = fmt.Sprintf("cn=%s,%s", ldap.EscapeFilter(user.DisplayName), c.config.Properties["user_ou"])
	}

	addReq := ldap.NewAddRequest(userDN, nil)
	addReq.Attribute("objectClass", []string{"top", "person", "organizationalPerson", "user"})
	addReq.Attribute("cn", []string{user.DisplayName})
	addReq.Attribute("sAMAccountName", []string{user.Username})
	addReq.Attribute("userPrincipalName", []string{user.Username})
	addReq.Attribute("displayName", []string{user.DisplayName})

	if user.Email != "" {
		addReq.Attribute("mail", []string{user.Email})
	}
	if user.FirstName != "" {
		addReq.Attribute("givenName", []string{user.FirstName})
	}
	if user.LastName != "" {
		addReq.Attribute("sn", []string{user.LastName})
	}
	if user.Department != "" {
		addReq.Attribute("department", []string{user.Department})
	}

	if err := c.conn.Add(addReq); err != nil {
		return "", fmt.Errorf("ldap: create user failed: %w", err)
	}

	// Set password (AD specific: must use unicodePwd over LDAPS)
	if c.config.Properties["set_password"] == "true" {
		pass := generateTempPassword()
		pwdModify := ldap.NewModifyRequest(userDN, nil)
		pwdModify.Replace("unicodePwd", []string{fmt.Sprintf("\"%s\"", pass)})
		if err := c.conn.Modify(pwdModify); err != nil {
			log.Printf("[LDAP] Warning: could not set password for %s: %v", user.Username, err)
		}
	}

	return userDN, nil
}

func (c *LDAPConnector) UpdateUser(ctx context.Context, externalID string, user ConnectorUser) error {
	if c.conn == nil {
		return fmt.Errorf("ldap: not connected")
	}

	modReq := ldap.NewModifyRequest(externalID, nil)

	if user.DisplayName != "" {
		modReq.Replace("displayName", []string{user.DisplayName})
		modReq.Replace("cn", []string{user.DisplayName})
	}
	if user.FirstName != "" {
		modReq.Replace("givenName", []string{user.FirstName})
	}
	if user.LastName != "" {
		modReq.Replace("sn", []string{user.LastName})
	}
	if user.Email != "" {
		modReq.Replace("mail", []string{user.Email})
	}
	if user.Department != "" {
		modReq.Replace("department", []string{user.Department})
	}
	if user.Title != "" {
		modReq.Replace("title", []string{user.Title})
	}
	if user.Phone != "" {
		modReq.Replace("telephoneNumber", []string{user.Phone})
	}
	if user.Mobile != "" {
		modReq.Replace("mobile", []string{user.Mobile})
	}
	if user.StreetAddress != "" {
		modReq.Replace("streetAddress", []string{user.StreetAddress})
	}
	if user.City != "" {
		modReq.Replace("l", []string{user.City})
	}
	if user.State != "" {
		modReq.Replace("st", []string{user.State})
	}
	if user.Company != "" {
		modReq.Replace("company", []string{user.Company})
	}

	return c.conn.Modify(modReq)
}

func (c *LDAPConnector) DeleteUser(ctx context.Context, externalID string) error {
	if c.conn == nil {
		return fmt.Errorf("ldap: not connected")
	}

	delReq := ldap.NewDelRequest(externalID, nil)
	return c.conn.Del(delReq)
}

func (c *LDAPConnector) DisableUser(ctx context.Context, externalID string) error {
	if c.conn == nil {
		return fmt.Errorf("ldap: not connected")
	}

	// AD: set userAccountControl to disable (0x0002 = ACCOUNTDISABLE)
	modReq := ldap.NewModifyRequest(externalID, nil)
	modReq.Replace("userAccountControl", []string{"514"}) // 512 (normal) + 2 (disabled)
	return c.conn.Modify(modReq)
}

func (c *LDAPConnector) EnableUser(ctx context.Context, externalID string) error {
	if c.conn == nil {
		return fmt.Errorf("ldap: not connected")
	}

	modReq := ldap.NewModifyRequest(externalID, nil)
	modReq.Replace("userAccountControl", []string{"512"}) // normal account
	return c.conn.Modify(modReq)
}

// ─── Group Operations ────────────────────────────────────────

func (c *LDAPConnector) ListGroups(ctx context.Context) ([]ConnectorGroup, error) {
	groupFilter := "(objectClass=group)"
	if c.config.Properties != nil && c.config.Properties["group_filter"] != "" {
		groupFilter = c.config.Properties["group_filter"]
	}

	entries, err := c.search(c.config.BaseDN, groupFilter, c.groupAttrs(), ldap.ScopeWholeSubtree)
	if err != nil {
		return nil, err
	}

	var groups []ConnectorGroup
	for _, e := range entries {
		groups = append(groups, ConnectorGroup{
			ExternalID:  e.DN,
			Name:        getAttr(e, "cn", getAttr(e, "name", "")),
			Description: getAttr(e, "description", ""),
			Type:        "security",
			Members:     getAttrs(e, "member"),
		})
	}
	return groups, nil
}

func (c *LDAPConnector) GetGroup(ctx context.Context, externalID string) (*ConnectorGroup, error) {
	entries, err := c.search(externalID, "(objectClass=group)", c.groupAttrs(), ldap.ScopeBaseObject)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("ldap: group not found: %s", externalID)
	}

	e := entries[0]
	return &ConnectorGroup{
		ExternalID:  e.DN,
		Name:        getAttr(e, "cn", ""),
		Description: getAttr(e, "description", ""),
		Type:        "security",
		Members:     getAttrs(e, "member"),
	}, nil
}

func (c *LDAPConnector) CreateGroup(ctx context.Context, group ConnectorGroup) (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("ldap: not connected")
	}

	groupDN := fmt.Sprintf("cn=%s,%s", ldap.EscapeFilter(group.Name), c.config.BaseDN)
	if c.config.Properties != nil && c.config.Properties["group_ou"] != "" {
		groupDN = fmt.Sprintf("cn=%s,%s", ldap.EscapeFilter(group.Name), c.config.Properties["group_ou"])
	}

	addReq := ldap.NewAddRequest(groupDN, nil)
	addReq.Attribute("objectClass", []string{"top", "group"})
	addReq.Attribute("cn", []string{group.Name})
	addReq.Attribute("sAMAccountName", []string{group.Name})

	if group.Description != "" {
		addReq.Attribute("description", []string{group.Description})
	}

	if err := c.conn.Add(addReq); err != nil {
		return "", fmt.Errorf("ldap: create group failed: %w", err)
	}
	return groupDN, nil
}

func (c *LDAPConnector) UpdateGroup(ctx context.Context, externalID string, group ConnectorGroup) error {
	if c.conn == nil {
		return fmt.Errorf("ldap: not connected")
	}

	modReq := ldap.NewModifyRequest(externalID, nil)
	if group.Name != "" {
		modReq.Replace("cn", []string{group.Name})
	}
	if group.Description != "" {
		modReq.Replace("description", []string{group.Description})
	}
	return c.conn.Modify(modReq)
}

func (c *LDAPConnector) DeleteGroup(ctx context.Context, externalID string) error {
	if c.conn == nil {
		return fmt.Errorf("ldap: not connected")
	}
	return c.conn.Del(ldap.NewDelRequest(externalID, nil))
}

func (c *LDAPConnector) AddGroupMember(ctx context.Context, groupID, userID string) error {
	if c.conn == nil {
		return fmt.Errorf("ldap: not connected")
	}

	modReq := ldap.NewModifyRequest(groupID, nil)
	modReq.Add("member", []string{userID})
	return c.conn.Modify(modReq)
}

func (c *LDAPConnector) RemoveGroupMember(ctx context.Context, groupID, userID string) error {
	if c.conn == nil {
		return fmt.Errorf("ldap: not connected")
	}

	modReq := ldap.NewModifyRequest(groupID, nil)
	modReq.Delete("member", []string{userID})
	return c.conn.Modify(modReq)
}

// ─── LDAP Attribute Helpers ──────────────────────────────────

func getAttr(e *ldap.Entry, name, fallback string) string {
	if v := e.GetAttributeValue(name); v != "" {
		return v
	}
	return fallback
}

func getAttrs(e *ldap.Entry, name string) []string {
	return e.GetAttributeValues(name)
}

func parseAttrInt(e *ldap.Entry, name string) int {
	v := e.GetAttributeValue(name)
	if v == "" {
		return 0
	}
	var i int
	fmt.Sscanf(v, "%d", &i)
	return i
}

func parseAttrTime(e *ldap.Entry, name string) time.Time {
	v := e.GetAttributeValue(name)
	if v == "" {
		return time.Time{}
	}
	// AD uses FileTime (Windows NT time format)
	// LDAP uses generalizedTime
	t, err := time.Parse("20060102150405Z", v)
	if err == nil {
		return t
	}
	// Try RFC3339
	t, err = time.Parse(time.RFC3339, v)
	if err == nil {
		return t
	}
	return time.Time{}
}

// isAccountDisabled checks AD userAccountControl for the ACCOUNTDISABLE flag.
func isAccountDisabled(uac int) bool {
	return uac&0x0002 != 0
}
