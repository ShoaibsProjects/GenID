package stores

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/observeid/genid/internal/connector"
)

// PersistSyncedUsers upserts a connector's synced user roster into
// connector_identities. tenantID must be resolved by the caller from the
// connector config (default tenant fallback lives at the call site).
func (s *Store) PersistSyncedUsers(ctx context.Context, tenantID, connectorID string, users []connector.ConnectorUser) (int, int, error) {
	if len(users) == 0 {
		return 0, 0, nil
	}
	created, updated := 0, 0

	tx, err := s.pg.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, user := range users {
		var raw []byte
		if user.Attributes != nil {
			if raw, _ = json.Marshal(user.Attributes); raw == nil {
				raw = []byte("{}")
			}
		}
		if raw == nil {
			raw = []byte("{}")
		}

		groups := user.Groups
		if groups == nil {
			groups = []string{}
		}
		roles := user.Roles
		if roles == nil {
			roles = []string{}
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO connector_identities
				(tenant_id, connector_id, external_id, username, email, display_name,
				 first_name, last_name, department, title, employee_id, manager_id,
				 phone, mobile, street_address, city, state, zip_code, country,
				 cost_center, division, company, enabled, locked,
				 groups, roles, raw_attributes, last_synced_at)
			VALUES
				($1, $2, $3, $4, $5, $6,
				 $7, $8, $9, $10, $11, $12,
				 $13, $14, $15, $16, $17, $18, $19,
				 $20, $21, $22, $23, $24,
				 $25, $26, $27, NOW())
			ON CONFLICT (connector_id, external_id) DO UPDATE SET
				username      = EXCLUDED.username,
				email         = EXCLUDED.email,
				display_name  = EXCLUDED.display_name,
				first_name    = EXCLUDED.first_name,
				last_name     = EXCLUDED.last_name,
				department    = EXCLUDED.department,
				title         = EXCLUDED.title,
				employee_id   = EXCLUDED.employee_id,
				manager_id    = EXCLUDED.manager_id,
				phone         = EXCLUDED.phone,
				mobile        = EXCLUDED.mobile,
				street_address = EXCLUDED.street_address,
				city          = EXCLUDED.city,
				state         = EXCLUDED.state,
				zip_code      = EXCLUDED.zip_code,
				country       = EXCLUDED.country,
				cost_center   = EXCLUDED.cost_center,
				division      = EXCLUDED.division,
				company       = EXCLUDED.company,
				enabled       = EXCLUDED.enabled,
				locked        = EXCLUDED.locked,
				groups        = EXCLUDED.groups,
				roles         = EXCLUDED.roles,
				raw_attributes = EXCLUDED.raw_attributes,
				last_synced_at = NOW()
		`, tenantID, connectorID, user.ExternalID, user.Username, user.Email, user.DisplayName,
			user.FirstName, user.LastName, user.Department, user.Title, user.EmployeeID, user.Manager,
			user.Phone, user.Mobile, user.StreetAddress, user.City, user.State, user.ZipCode, user.Country,
			user.CostCenter, user.Division, user.Company, user.Enabled, user.Locked,
			groups, roles, raw)

		if err != nil {
			return created, updated, fmt.Errorf("upsert user %s: %w", user.ExternalID, err)
		}
		if tag.Insert() {
			created++
		} else {
			updated++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return created, updated, fmt.Errorf("commit: %w", err)
	}
	return created, updated, nil
}

// PersistSyncedGroups upserts a connector's synced groups into
// connector_groups. tenantID must be resolved by the caller from the
// connector config (default tenant fallback lives at the call site).
func (s *Store) PersistSyncedGroups(ctx context.Context, tenantID, connectorID string, groups []connector.ConnectorGroup) (int, int, error) {
	if len(groups) == 0 {
		return 0, 0, nil
	}
	created, updated := 0, 0

	tx, err := s.pg.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, group := range groups {
		members := group.Members
		if members == nil {
			members = []string{}
		}

		var raw []byte
		if group.Attributes != nil {
			raw, _ = json.Marshal(group.Attributes)
		}
		if raw == nil {
			raw = []byte("{}")
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO connector_groups
				(tenant_id, connector_id, external_id, name, description, group_type, scope, member_ids, raw_attributes, last_synced_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
			ON CONFLICT (connector_id, external_id) DO UPDATE SET
				name            = EXCLUDED.name,
				description     = EXCLUDED.description,
				group_type      = EXCLUDED.group_type,
				scope           = EXCLUDED.scope,
				member_ids      = EXCLUDED.member_ids,
				raw_attributes  = EXCLUDED.raw_attributes,
				last_synced_at  = NOW()
		`, tenantID, connectorID, group.ExternalID, group.Name, group.Description,
			group.Type, group.Scope, members, raw)

		if err != nil {
			return created, updated, fmt.Errorf("upsert group %s: %w", group.ExternalID, err)
		}
		if tag.Insert() {
			created++
		} else {
			updated++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return created, updated, fmt.Errorf("commit: %w", err)
	}
	return created, updated, nil
}

// PersistSyncedEntitlements replaces a connector's entitlements
// (delete-then-insert) in connector_entitlements. tenantID must be resolved
// by the caller from the connector config.
func (s *Store) PersistSyncedEntitlements(ctx context.Context, tenantID, connectorID string, entitlements []connector.ConnectorEntitlement) error {
	if len(entitlements) == 0 {
		return nil
	}

	tx, err := s.pg.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM connector_entitlements WHERE connector_id = $1`, connectorID); err != nil {
		return fmt.Errorf("delete existing entitlements: %w", err)
	}

	for _, e := range entitlements {
		raw, _ := json.Marshal(e.RawAttributes)
		if raw == nil {
			raw = []byte("{}")
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO connector_entitlements
				(tenant_id, connector_id, identity_external_id, entitlement_type, source_id,
				 source_name, source_type, app_id, app_name, assigned_at, is_active, raw_attributes, last_synced_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		`, tenantID, connectorID, e.IdentityExternalID, e.EntitlementType,
			e.SourceID, e.SourceName, e.SourceType,
			e.AppID, e.AppName, e.AssignedAt, e.IsActive, raw); err != nil {
			return fmt.Errorf("insert entitlement: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// PersistSyncedResources replaces a connector's resources
// (delete-then-insert) in connector_resources. tenantID must be resolved by
// the caller from the connector config.
func (s *Store) PersistSyncedResources(ctx context.Context, tenantID, connectorID string, resources []connector.ConnectorResource) error {
	if len(resources) == 0 {
		return nil
	}

	tx, err := s.pg.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM connector_resources WHERE connector_id = $1`, connectorID); err != nil {
		return fmt.Errorf("delete existing resources: %w", err)
	}

	for _, res := range resources {
		raw, _ := json.Marshal(res.Attributes)
		if raw == nil {
			raw = []byte("{}")
		}
		owners := res.OwnerIDs
		if owners == nil {
			owners = []string{}
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO connector_resources
				(tenant_id, connector_id, external_id, resource_type, name, description, enabled, owner_ids, raw_attributes, last_synced_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
			ON CONFLICT (connector_id, external_id) DO UPDATE SET
				name            = EXCLUDED.name,
				description     = EXCLUDED.description,
				resource_type   = EXCLUDED.resource_type,
				enabled         = EXCLUDED.enabled,
				owner_ids       = EXCLUDED.owner_ids,
				raw_attributes  = EXCLUDED.raw_attributes,
				last_synced_at  = NOW()
		`, tenantID, connectorID, res.ExternalID, res.ResourceType,
			res.Name, res.Description, res.Enabled, owners, raw); err != nil {
			return fmt.Errorf("upsert resource %s: %w", res.ExternalID, err)
		}
	}

	return tx.Commit(ctx)
}

// PersistSyncedPermissions replaces a connector's permission catalog
// (delete-then-insert) in connector_permissions. tenantID must be resolved
// by the caller from the connector config.
func (s *Store) PersistSyncedPermissions(ctx context.Context, tenantID, connectorID string, permissions []connector.ConnectorPermission) error {
	if len(permissions) == 0 {
		return nil
	}

	tx, err := s.pg.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM connector_permissions WHERE connector_id = $1`, connectorID); err != nil {
		return fmt.Errorf("delete existing permissions: %w", err)
	}

	for _, p := range permissions {
		raw, _ := json.Marshal(p.RawAttributes)
		if raw == nil {
			raw = []byte("{}")
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO connector_permissions
				(tenant_id, connector_id, permission_id, name, permission_type,
				 app_id, app_name, description, is_admin, raw_attributes, last_synced_at)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
			ON CONFLICT (connector_id, permission_id) DO UPDATE SET
				name            = EXCLUDED.name,
				permission_type = EXCLUDED.permission_type,
				app_id          = EXCLUDED.app_id,
				app_name        = EXCLUDED.app_name,
				description     = EXCLUDED.description,
				is_admin        = EXCLUDED.is_admin,
				raw_attributes  = EXCLUDED.raw_attributes,
				last_synced_at  = NOW()
		`, tenantID, connectorID, p.PermissionID, p.Name, p.PermissionType,
			p.AppID, p.AppName, p.Description, p.IsAdmin, raw); err != nil {
			return fmt.Errorf("upsert permission %s: %w", p.PermissionID, err)
		}
	}

	return tx.Commit(ctx)
}
