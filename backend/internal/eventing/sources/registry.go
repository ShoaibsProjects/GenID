package sources

import (
	"encoding/json"
	"fmt"
	"os"
)

// Registry holds the configured external event sources keyed by slug.
type Registry struct {
	sources map[string]*SourceConfig
}

// DefaultRegistry returns the built-in sources so the edge works out of the
// box for the big three: Microsoft Entra ID, Okta, and Jira/ITSM webhooks.
// A JSON file (EVENT_SOURCES_CONFIG) may extend or override these.
func DefaultRegistry() *Registry {
	r := &Registry{sources: map[string]*SourceConfig{}}

	r.Add(&SourceConfig{
		Name:        "entra",
		DisplayName: "microsoft-entra",
		SecretEnv:   "EVENT_SOURCE_ENTRA_SECRET",
		Mapping: FieldMapping{
			EventType:  "eventType",
			IdentityID: "userPrincipalName",
			Severity:   "riskLevel",
		},
		EventTypeMap: map[string]string{
			"SignInFailure":      "auth.failed_login",
			"SignInRiskDetected": "auth.impossible_travel",
			"MfaFailure":         "auth.mfa_failure",
			"PasswordSpray":      "auth.password_spray",
			"AccountLocked":      "account.locked",
			"TokenAnomaly":       "session.anomalous",
		},
		DefaultEventType: "auth.failed_login",
	})

	r.Add(&SourceConfig{
		Name:        "okta",
		DisplayName: "okta",
		SecretEnv:   "EVENT_SOURCE_OKTA_SECRET",
		Mapping: FieldMapping{
			EventType:  "eventType",
			IdentityID: "actor.alternateId",
			Severity:   "severity",
		},
		EventTypeMap: map[string]string{
			"user.session.start_failed":   "auth.failed_login",
			"user.authentication.auth_via_mfa_failed": "auth.mfa_failure",
			"user.account.lock":           "account.locked",
			"policy.evaluate_sign_on.deny": "session.anomalous",
		},
		DefaultEventType: "auth.failed_login",
	})

	r.Add(&SourceConfig{
		Name:        "jira",
		DisplayName: "jira-itsm",
		SecretEnv:   "EVENT_SOURCE_JIRA_SECRET",
		Mapping: FieldMapping{
			EventType:  "webhookEvent",
			IdentityID: "user.emailAddress",
		},
		EventTypeMap: map[string]string{
			"jira:issue_created": "itsm.ticket_opened",
			"jira:issue_updated": "itsm.ticket_updated",
		},
		DefaultEventType: "itsm.ticket_opened",
	})

	return r
}

// LoadFile merges source definitions from a JSON array file into the
// registry. Sources whose name matches an existing entry override it.
func (r *Registry) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read sources config: %w", err)
	}
	var cfgs []SourceConfig
	if err := json.Unmarshal(data, &cfgs); err != nil {
		return fmt.Errorf("parse sources config: %w", err)
	}
	for i := range cfgs {
		if cfgs[i].Name == "" {
			return fmt.Errorf("sources config: entry %d missing name", i)
		}
		cfg := cfgs[i]
		r.Add(&cfg)
	}
	return nil
}

// Add registers or replaces a source.
func (r *Registry) Add(cfg *SourceConfig) {
	if cfg.DisplayName == "" {
		cfg.DisplayName = cfg.Name
	}
	r.sources[cfg.Name] = cfg
}

// Get returns the source for a slug, or nil.
func (r *Registry) Get(name string) *SourceConfig {
	return r.sources[name]
}

// Names lists registered source slugs (for /events/sources discovery).
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.sources))
	for n := range r.sources {
		names = append(names, n)
	}
	return names
}
