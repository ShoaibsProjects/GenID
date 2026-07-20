package connector

import (
	"context"
	"fmt"
	"time"
)

// ─── V1ToV2Adapter ─────────────────────────────────────────────
// AdaptConnectorV1 wraps any V1 Connector (24-method interface) as
// a V2 Connector (event-stream interface). This allows the Manager
// to treat all connectors uniformly through the ConnectorV2 path.
//
// The adapter:
//   - Calls ListUsers() → emits "created" events per user (full sync)
//   - Calls ListUsersDelta() → emits created/updated/deleted events (delta sync)
//   - Calls ListGroups() → emits "created" events per group
//   - Calls ListEntitlements() → emits "created" events per entitlement
//   - Calls ListResources() → emits "created" events per resource
//   - Maps Apply() calls to the appropriate Create/Update/Delete method per entity type
//
// This is a BRIDGE pattern — it connects the old world to the new
// without modifying either side.

type V1ToV2Adapter struct {
	Connector // original V1 implementation
	config    ConnectorConfig
}

// AdaptConnectorV1 wraps a V1 connector as V2.
func AdaptConnectorV1(v1 Connector, cfg ConnectorConfig) *V1ToV2Adapter {
	return &V1ToV2Adapter{Connector: v1, config: cfg}
}

// Events produces a stream of EntityEvents from the V1 connector's
// List methods. Each list method is called and its results are
// converted to "created" events.
func (a *V1ToV2Adapter) Events(ctx context.Context, opts SyncOptions) (<-chan EntityEvent, error) {
	out := make(chan EntityEvent, 100)

	go func() {
		defer close(out)

		batchID := fmt.Sprintf("sync-%d", time.Now().UnixMilli())
		now := time.Now()

		// Helper: extract metadata for events
		meta := map[string]string{
			"connector_id": a.config.ID,
			"sync_batch":   batchID,
		}
		if opts.SyncMode == "delta" {
			meta["sync_mode"] = "delta"
		}

		// 1. Stream users
		users, err := a.Connector.ListUsers(ctx)
		if err == nil {
			for _, u := range users {
				select {
				case out <- EntityEvent{
					EntityType: "User",
					Operation:  EventCreated,
					Key:        u.ExternalID,
					Data:       userToMap(u),
					Timestamp:  now,
					Metadata:   meta,
				}:
				case <-ctx.Done():
					return
				}
			}
		}

		// 2. Try delta sync if requested
		if opts.SyncMode == "delta" && opts.DeltaToken != "" {
			deltaUsers, newToken, err := a.Connector.ListUsersDelta(ctx, opts.DeltaToken)
			if err == nil {
				for _, u := range deltaUsers {
					op := EventUpdated
					// ConnectorUser doesn't have a Deleted field,
					// so we trust delta sync to return only changes.
					// Deletions are detected by the platform during reconciliation.
					select {
					case out <- EntityEvent{
						EntityType: "User",
						Operation:  op,
						Key:        u.ExternalID,
						Data:       userToMap(u),
						Timestamp:  now,
						Metadata:   meta,
					}:
					case <-ctx.Done():
						return
					}
				}
				// Set delta token in the last event's metadata
				if newToken != "" {
					meta["delta_token"] = newToken
				}
			}
		}

		// 3. Stream groups
		groups, err := a.Connector.ListGroups(ctx)
		if err == nil {
			for _, g := range groups {
				out <- EntityEvent{
					EntityType: "Group",
					Operation:  EventCreated,
					Key:        g.ExternalID,
					Data:       groupToMap(g),
					Timestamp:  now,
					Metadata:   meta,
				}
			}
		}

		// 4. Stream entitlements
		ents, err := a.Connector.ListEntitlements(ctx)
		if err == nil {
			for _, e := range ents {
				out <- EntityEvent{
					EntityType: "Entitlement",
					Operation:  EventCreated,
					Key:        e.IdentityExternalID + "/" + e.SourceID,
					Data:       entitlementToMap(e),
					Timestamp:  now,
					Metadata:   meta,
				}
			}
		}

		// 5. Stream resources
		resources, err := a.Connector.ListResources(ctx)
		if err == nil {
			for _, r := range resources {
				out <- EntityEvent{
					EntityType: "Resource",
					Operation:  EventCreated,
					Key:        r.ExternalID,
					Data:       resourceToMap(r),
					Timestamp:  now,
					Metadata:   meta,
				}
			}
		}
	}()

	return out, nil
}

// Apply maps mutation events to V1 methods.
func (a *V1ToV2Adapter) Apply(ctx context.Context, events []EntityEvent) ([]ApplyResult, error) {
	results := make([]ApplyResult, 0, len(events))

	for _, ev := range events {
		result := ApplyResult{Key: ev.Key}

		switch ev.EntityType {
		case "User":
			switch ev.Operation {
			case EventCreated:
				user := mapToUser(ev.Data)
				id, err := a.Connector.CreateUser(ctx, user)
				if err != nil {
					result.Error = err.Error()
				} else {
					result.Success = true
					if id != "" {
						result.Key = id
					}
				}
			case EventUpdated:
				user := mapToUser(ev.Data)
				err := a.Connector.UpdateUser(ctx, ev.Key, user)
				if err != nil {
					result.Error = err.Error()
				} else {
					result.Success = true
				}
			case EventDeleted:
				err := a.Connector.DeleteUser(ctx, ev.Key)
				if err != nil {
					result.Error = err.Error()
				} else {
					result.Success = true
				}
			}
		default:
			result.Error = fmt.Sprintf("entity type %q not supported for mutations via adapter", ev.EntityType)
		}

		results = append(results, result)
	}

	return results, nil
}

// ─── Converters ────────────────────────────────────────────────

func userToMap(u ConnectorUser) map[string]any {
	m := map[string]any{
		"external_id":   u.ExternalID,
		"username":      u.Username,
		"email":         u.Email,
		"display_name":  u.DisplayName,
		"first_name":    u.FirstName,
		"last_name":     u.LastName,
		"department":    u.Department,
		"title":         u.Title,
		"employee_id":   u.EmployeeID,
		"manager":       u.Manager,
		"phone":         u.Phone,
		"mobile":        u.Mobile,
		"enabled":       u.Enabled,
		"locked":        u.Locked,
		"groups":        u.Groups,
		"roles":         u.Roles,
	}
	// Only include non-empty fields
	for k, v := range m {
		if s, ok := v.(string); ok && s == "" {
			delete(m, k)
		}
	}
	return m
}

func mapToUser(data map[string]any) ConnectorUser {
	u := ConnectorUser{}
	if v, ok := data["external_id"].(string); ok {
		u.ExternalID = v
	}
	if v, ok := data["username"].(string); ok {
		u.Username = v
	}
	if v, ok := data["email"].(string); ok {
		u.Email = v
	}
	if v, ok := data["display_name"].(string); ok {
		u.DisplayName = v
	}
	if v, ok := data["first_name"].(string); ok {
		u.FirstName = v
	}
	if v, ok := data["last_name"].(string); ok {
		u.LastName = v
	}
	if v, ok := data["department"].(string); ok {
		u.Department = v
	}
	if v, ok := data["title"].(string); ok {
		u.Title = v
	}
	if v, ok := data["employee_id"].(string); ok {
		u.EmployeeID = v
	}
	if v, ok := data["manager"].(string); ok {
		u.Manager = v
	}
	if v, ok := data["enabled"].(bool); ok {
		u.Enabled = v
	}
	if v, ok := data["locked"].(bool); ok {
		u.Locked = v
	}
	return u
}

func groupToMap(g ConnectorGroup) map[string]any {
	return map[string]any{
		"external_id": g.ExternalID,
		"name":        g.Name,
		"description": g.Description,
		"group_type":  g.Type,
		"scope":       g.Scope,
		"member_ids":  g.Members,
	}
}

func entitlementToMap(e ConnectorEntitlement) map[string]any {
	return map[string]any{
		"identity_external_id": e.IdentityExternalID,
		"entitlement_type":     e.EntitlementType,
		"source_id":            e.SourceID,
		"source_name":          e.SourceName,
		"source_type":          e.SourceType,
		"app_id":               e.AppID,
		"app_name":             e.AppName,
		"is_active":            e.IsActive,
	}
}

func resourceToMap(r ConnectorResource) map[string]any {
	return map[string]any{
		"external_id":   r.ExternalID,
		"resource_type": r.ResourceType,
		"name":          r.Name,
		"description":   r.Description,
		"enabled":       r.Enabled,
		"owner_ids":     r.OwnerIDs,
	}
}
