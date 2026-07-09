package connector

import (
	"context"
	"log"
	"sync"
	"time"
)

// ─── Provisioning Engine ─────────────────────────────────────
// Handles IAM Lifecycle Management (LCM): user/group/account lifecycle
// operations across connected systems.

// ProvisioningAction represents the type of lifecycle action to perform.
type ProvisioningAction string

const (
	ActionCreateUser      ProvisioningAction = "create_user"
	ActionUpdateUser      ProvisioningAction = "update_user"
	ActionDeleteUser      ProvisioningAction = "delete_user"
	ActionEnableUser      ProvisioningAction = "enable_user"
	ActionDisableUser     ProvisioningAction = "disable_user"
	ActionCreateGroup     ProvisioningAction = "create_group"
	ActionUpdateGroup     ProvisioningAction = "update_group"
	ActionDeleteGroup     ProvisioningAction = "delete_group"
	ActionAddToGroup      ProvisioningAction = "add_to_group"
	ActionRemoveFromGroup ProvisioningAction = "remove_from_group"
	ActionAssignRole      ProvisioningAction = "assign_role"
	ActionRevokeRole      ProvisioningAction = "revoke_role"
	ActionFullSync        ProvisioningAction = "full_sync"
)

// ProvisioningStatus represents the result of a provisioning operation.
type ProvisioningStatus string

const (
	ProvisioningPending    ProvisioningStatus = "pending"
	ProvisioningInProgress ProvisioningStatus = "in_progress"
	ProvisioningSuccess    ProvisioningStatus = "success"
	ProvisioningFailed     ProvisioningStatus = "failed"
	ProvisioningSkipped    ProvisioningStatus = "skipped"
)

// ProvisioningResult holds the outcome of a single provisioning operation.
type ProvisioningResult struct {
	ID            string              `json:"id"`
	ConnectorID   string              `json:"connector_id"`
	ConnectorName string              `json:"connector_name"`
	Action        ProvisioningAction  `json:"action"`
	Status        ProvisioningStatus  `json:"status"`
	ExternalID    string              `json:"external_id,omitempty"`
	Subject       string              `json:"subject,omitempty"` // the user/group being provisioned
	Error         string              `json:"error,omitempty"`
	StartedAt     time.Time           `json:"started_at"`
	CompletedAt   time.Time           `json:"completed_at,omitempty"`
}

// ProvisioningEngine manages lifecycle operations across connectors.
type ProvisioningEngine struct {
	mu        sync.RWMutex
	manager   *Manager
	history   []ProvisioningResult
}

func NewProvisioningEngine(manager *Manager) *ProvisioningEngine {
	return &ProvisioningEngine{
		manager: manager,
		history: make([]ProvisioningResult, 0),
	}
}

// ─── User Lifecycle ─────────────────────────────────────────

// ProvisionUser creates a user across all target connectors.
func (e *ProvisioningEngine) ProvisionUser(ctx context.Context, user ConnectorUser, connectorIDs []string) []ProvisioningResult {
	var results []ProvisioningResult
	for _, cid := range connectorIDs {
		result := e.provisionUserToConnector(ctx, cid, user, ActionCreateUser)
		results = append(results, result)
	}
	e.appendHistory(results)
	return results
}

// UpdateUser updates a user across connectors.
func (e *ProvisioningEngine) UpdateUser(ctx context.Context, externalID string, user ConnectorUser, connectorIDs []string) []ProvisioningResult {
	var results []ProvisioningResult
	for _, cid := range connectorIDs {
		result := e.doProvision(ctx, cid, ActionUpdateUser, func(conn Connector) error {
			return conn.UpdateUser(ctx, externalID, user)
		})
		result.Subject = user.Username
		result.ExternalID = externalID
		results = append(results, result)
	}
	e.appendHistory(results)
	return results
}

// DeleteUser deletes a user across connectors.
func (e *ProvisioningEngine) DeleteUser(ctx context.Context, externalID, username string, connectorIDs []string) []ProvisioningResult {
	var results []ProvisioningResult
	for _, cid := range connectorIDs {
		result := e.doProvision(ctx, cid, ActionDeleteUser, func(conn Connector) error {
			return conn.DeleteUser(ctx, externalID)
		})
		result.Subject = username
		result.ExternalID = externalID
		results = append(results, result)
	}
	e.appendHistory(results)
	return results
}

// DisableUser disables a user across connectors (for suspension).
func (e *ProvisioningEngine) DisableUser(ctx context.Context, externalID string, connectorIDs []string) []ProvisioningResult {
	var results []ProvisioningResult
	for _, cid := range connectorIDs {
		result := e.doProvision(ctx, cid, ActionDisableUser, func(conn Connector) error {
			return conn.DisableUser(ctx, externalID)
		})
		result.ExternalID = externalID
		results = append(results, result)
	}
	e.appendHistory(results)
	return results
}

func (e *ProvisioningEngine) provisionUserToConnector(ctx context.Context, connectorID string, user ConnectorUser, action ProvisioningAction) ProvisioningResult {
	return e.doProvision(ctx, connectorID, action, func(conn Connector) error {
		// Check if user already exists by username
		existing, err := conn.GetUserByUsername(ctx, user.Username)
		if err == nil && existing != nil {
			// User exists — update instead
			user.ExternalID = existing.ExternalID
			return conn.UpdateUser(ctx, existing.ExternalID, user)
		}
		// Create new user
		extID, err := conn.CreateUser(ctx, user)
		if err != nil {
			return err
		}
		user.ExternalID = extID

		// Add to groups if specified
		for _, groupID := range user.Groups {
			if gErr := conn.AddGroupMember(ctx, groupID, extID); gErr != nil {
				log.Printf("[PROVISION] Warning: could not add %s to group %s: %v", user.Username, groupID, gErr)
			}
		}
		return nil
	})
}

// ─── Group Lifecycle ────────────────────────────────────────

// ProvisionGroup creates a group across connectors.
func (e *ProvisioningEngine) ProvisionGroup(ctx context.Context, group ConnectorGroup, connectorIDs []string) []ProvisioningResult {
	var results []ProvisioningResult
	for _, cid := range connectorIDs {
		result := e.doProvision(ctx, cid, ActionCreateGroup, func(conn Connector) error {
			extID, err := conn.CreateGroup(ctx, group)
			if err != nil {
				return err
			}
			group.ExternalID = extID
			return nil
		})
		result.Subject = group.Name
		results = append(results, result)
	}
	e.appendHistory(results)
	return results
}

// DeleteGroup deletes a group across connectors.
func (e *ProvisioningEngine) DeleteGroup(ctx context.Context, externalID string, connectorIDs []string) []ProvisioningResult {
	var results []ProvisioningResult
	for _, cid := range connectorIDs {
		result := e.doProvision(ctx, cid, ActionDeleteGroup, func(conn Connector) error {
			return conn.DeleteGroup(ctx, externalID)
		})
		result.ExternalID = externalID
		results = append(results, result)
	}
	e.appendHistory(results)
	return results
}

// AddUserToGroup adds a user to a group across connectors.
func (e *ProvisioningEngine) AddUserToGroup(ctx context.Context, groupID, userID string, connectorIDs []string) []ProvisioningResult {
	var results []ProvisioningResult
	for _, cid := range connectorIDs {
		result := e.doProvision(ctx, cid, ActionAddToGroup, func(conn Connector) error {
			return conn.AddGroupMember(ctx, groupID, userID)
		})
		results = append(results, result)
	}
	e.appendHistory(results)
	return results
}

// RemoveUserFromGroup removes a user from a group across connectors.
func (e *ProvisioningEngine) RemoveUserFromGroup(ctx context.Context, groupID, userID string, connectorIDs []string) []ProvisioningResult {
	var results []ProvisioningResult
	for _, cid := range connectorIDs {
		result := e.doProvision(ctx, cid, ActionRemoveFromGroup, func(conn Connector) error {
			return conn.RemoveGroupMember(ctx, groupID, userID)
		})
		results = append(results, result)
	}
	e.appendHistory(results)
	return results
}

// ─── Lifecycle Templates ────────────────────────────────────

// LCMRequest is a full lifecycle management request.
type LCMRequest struct {
	Action        ProvisioningAction `json:"action"`
	ConnectorIDs  []string           `json:"connector_ids"`
	User          *ConnectorUser     `json:"user,omitempty"`
	Group         *ConnectorGroup    `json:"group,omitempty"`
	ExternalID    string             `json:"external_id,omitempty"`
	GroupID       string             `json:"group_id,omitempty"`
	UserID        string             `json:"user_id,omitempty"`
}

// ExecuteLCM executes a lifecycle management request.
func (e *ProvisioningEngine) ExecuteLCM(ctx context.Context, req LCMRequest) []ProvisioningResult {
	switch req.Action {
	case ActionCreateUser:
		if req.User != nil {
			return e.ProvisionUser(ctx, *req.User, req.ConnectorIDs)
		}
	case ActionUpdateUser:
		if req.User != nil {
			return e.UpdateUser(ctx, req.ExternalID, *req.User, req.ConnectorIDs)
		}
	case ActionDeleteUser:
		return e.DeleteUser(ctx, req.ExternalID, "", req.ConnectorIDs)
	case ActionDisableUser:
		return e.DisableUser(ctx, req.ExternalID, req.ConnectorIDs)
	case ActionCreateGroup:
		if req.Group != nil {
			return e.ProvisionGroup(ctx, *req.Group, req.ConnectorIDs)
		}
	case ActionDeleteGroup:
		return e.DeleteGroup(ctx, req.ExternalID, req.ConnectorIDs)
	case ActionAddToGroup:
		return e.AddUserToGroup(ctx, req.GroupID, req.UserID, req.ConnectorIDs)
	case ActionRemoveFromGroup:
		return e.RemoveUserFromGroup(ctx, req.GroupID, req.UserID, req.ConnectorIDs)
	}
	return []ProvisioningResult{{
		Action: req.Action,
		Status: ProvisioningSkipped,
		Error:  "invalid or missing parameters",
	}}
}

// ─── Internal ───────────────────────────────────────────────

func (e *ProvisioningEngine) doProvision(ctx context.Context, connectorID string, action ProvisioningAction, fn func(Connector) error) ProvisioningResult {
	start := time.Now()
	result := ProvisioningResult{
		ConnectorID: connectorID,
		Action:      action,
		StartedAt:   start,
		Status:      ProvisioningInProgress,
	}

	conn, err := e.manager.GetConnector(connectorID)
	if err != nil {
		result.Status = ProvisioningFailed
		result.Error = err.Error()
		result.CompletedAt = time.Now()
		return result
	}

	config, _ := e.manager.GetConfig(connectorID)
	result.ConnectorName = config.Name

	if err := fn(conn); err != nil {
		result.Status = ProvisioningFailed
		result.Error = err.Error()
	} else {
		result.Status = ProvisioningSuccess
	}

	result.CompletedAt = time.Now()
	return result
}

func (e *ProvisioningEngine) appendHistory(results []ProvisioningResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.history = append(e.history, results...)
	// Keep last 1000 entries
	if len(e.history) > 1000 {
		e.history = e.history[len(e.history)-1000:]
	}
}

// GetHistory returns recent provisioning history.
func (e *ProvisioningEngine) GetHistory() []ProvisioningResult {
	e.mu.RLock()
	defer e.mu.RUnlock()
	cp := make([]ProvisioningResult, len(e.history))
	copy(cp, e.history)
	return cp
}
