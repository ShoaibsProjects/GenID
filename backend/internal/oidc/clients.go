package oidc

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ClientRecord represents a registered OAuth2/OIDC client.
type ClientRecord struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret,omitempty"`
	RedirectURIs []string  `json:"redirect_uris"`
	GrantTypes   []string  `json:"grant_types"`
	Scopes       []string  `json:"scopes"`
	IsPublic     bool      `json:"is_public"`
	CreatedAt    time.Time `json:"created_at"`
}

// ─── Client Registration ─────────────────────────────────────

func (p *Provider) RegisterClient(ctx context.Context, name string, redirectURIs, grantTypes, scopes []string, isPublic bool) (*ClientRecord, error) {
	clientID := fmt.Sprintf("oidc-%s-%s", name, uuid.New().String()[:8])
	clientSecret := fmt.Sprintf("secret-%x", uuid.New().String())

	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code", "refresh_token"}
	}
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}

	_, err := p.pgPool.Exec(ctx, `
		INSERT INTO oidc_clients (id, name, client_id, client_secret, redirect_uris, grant_types, scopes, is_public)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (client_id) DO UPDATE SET
			name = EXCLUDED.name, redirect_uris = EXCLUDED.redirect_uris,
			grant_types = EXCLUDED.grant_types, scopes = EXCLUDED.scopes
	`, uuid.New().String(), name, clientID, clientSecret, redirectURIs, grantTypes, scopes, isPublic)

	if err != nil {
		return nil, fmt.Errorf("oidc: register client: %w", err)
	}

	return &ClientRecord{
		Name: name, ClientID: clientID, ClientSecret: clientSecret,
		RedirectURIs: redirectURIs, GrantTypes: grantTypes, Scopes: scopes,
		IsPublic: isPublic, CreatedAt: time.Now(),
	}, nil
}

// GetClient retrieves a client by client_id.
func (p *Provider) GetClient(ctx context.Context, clientID string) (*ClientRecord, error) {
	var id, name, secret string
	var redirectURIs, grantTypes, scopes []string
	var isPublic bool
	var createdAt time.Time

	err := p.pgPool.QueryRow(ctx, `
		SELECT id, name, client_id, client_secret, redirect_uris, grant_types, scopes, is_public, created_at
		FROM oidc_clients WHERE client_id = $1
	`, clientID).Scan(&id, &name, &clientID, &secret, &redirectURIs, &grantTypes, &scopes, &isPublic, &createdAt)

	if err != nil {
		return nil, fmt.Errorf("oidc: client not found: %s", clientID)
	}

	return &ClientRecord{
		ID: id, Name: name, ClientID: clientID, ClientSecret: secret,
		RedirectURIs: redirectURIs, GrantTypes: grantTypes, Scopes: scopes,
		IsPublic: isPublic, CreatedAt: createdAt,
	}, nil
}

// ValidateClient checks client credentials.
func (p *Provider) ValidateClient(ctx context.Context, clientID, clientSecret string) (*ClientRecord, error) {
	client, err := p.GetClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if !client.IsPublic && client.ClientSecret != clientSecret {
		return nil, fmt.Errorf("oidc: invalid client secret")
	}
	return client, nil
}

// ListClients returns all registered OIDC clients.
func (p *Provider) ListClients(ctx context.Context) ([]ClientRecord, error) {
	rows, err := p.pgPool.Query(ctx, `
		SELECT id, name, client_id, redirect_uris, grant_types, scopes, is_public, created_at
		FROM oidc_clients ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []ClientRecord
	for rows.Next() {
		var c ClientRecord
		if err := rows.Scan(&c.ID, &c.Name, &c.ClientID, &c.RedirectURIs, &c.GrantTypes, &c.Scopes, &c.IsPublic, &c.CreatedAt); err != nil {
			continue
		}
		clients = append(clients, c)
	}
	if clients == nil {
		clients = []ClientRecord{}
	}
	return clients, nil
}

// DeleteClient removes a registered OIDC client.
func (p *Provider) DeleteClient(ctx context.Context, clientID string) error {
	_, err := p.pgPool.Exec(ctx, `DELETE FROM oidc_clients WHERE client_id = $1`, clientID)
	return err
}
