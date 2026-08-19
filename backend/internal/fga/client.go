package fga

import (
	"context"
	"fmt"
	"os"
	"sync"

	fgaSdk "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

// Client wraps the OpenFGA SDK client with GenID-specific operations.
type Client struct {
	mu        sync.RWMutex
	sdk       client.SdkClient
	storeID   string
	modelID   string
	storeName string
}

// Config holds OpenFGA connection configuration.
type Config struct {
	APIURL    string // e.g., "http://localhost:8080"
	StoreID   string // existing store ID (optional)
	StoreName string // name for new store
	ModelFile string // path to .fga model file (optional)
}

// NewClient creates a new OpenFGA client, optionally creating the store and
// writing the authorization model.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.APIURL == "" {
		cfg.APIURL = os.Getenv("OPENFGA_API_URL")
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "http://localhost:8080"
	}

	sdk, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl: cfg.APIURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create OpenFGA SDK client: %w", err)
	}

	c := &Client{sdk: sdk, storeName: cfg.StoreName}

	// Create or get store
	if cfg.StoreID != "" {
		c.storeID = cfg.StoreID
	} else if cfg.StoreName != "" {
		resp, err := sdk.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: cfg.StoreName}).Execute()
		if err != nil {
			return nil, fmt.Errorf("create store: %w", err)
		}
		c.storeID = resp.Id
	} else {
		c.storeName = "genid"
		stores, err := sdk.ListStores(ctx).Execute()
		if err == nil {
			for _, s := range stores.Stores {
				if s.Name == "genid" {
					c.storeID = s.Id
					break
				}
			}
		}
		if c.storeID == "" {
			resp, err := sdk.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: "genid"}).Execute()
			if err != nil {
				return nil, fmt.Errorf("create default store: %w", err)
			}
			c.storeID = resp.Id
		}
	}

	// Note: Model loading from .fga file requires parsing into TypeDefinition structs.
	// For production, use openfga CLI to write the model: `openfga store write-model -f model.fga`
	// Then set the model ID via config or environment.
	if cfg.ModelFile != "" {
		// Placeholder: model loading from file requires parsing into TypeDefinition[]
		// Use OpenFGA CLI or SDK parser for production
		_ = cfg.ModelFile
	}

	return c, nil
}

// StoreID returns the OpenFGA store ID.
func (c *Client) StoreID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.storeID
}

// ModelID returns the current authorization model ID.
func (c *Client) ModelID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.modelID
}

// SetModelID manually sets the authorization model ID (e.g., after CLI write-model).
func (c *Client) SetModelID(modelID string) {
	c.mu.Lock()
	c.modelID = modelID
	c.mu.Unlock()
}

// WriteTuples writes a batch of relationship tuples.
func (c *Client) WriteTuples(ctx context.Context, tuples []*fgaSdk.TupleKey) error {
	c.mu.RLock()
	modelID := c.modelID
	c.mu.RUnlock()

	if modelID == "" {
		return fmt.Errorf("no authorization model ID set (call SetModelID or write via CLI)")
	}

	writes := make([]fgaSdk.TupleKey, len(tuples))
	for i, t := range tuples {
		writes[i] = *t
	}

	_, err := c.sdk.Write(ctx).Body(client.ClientWriteRequest{
		Writes: writes,
	}).Execute()
	return err
}

// DeleteTuples deletes a batch of relationship tuples.
func (c *Client) DeleteTuples(ctx context.Context, tuples []*fgaSdk.TupleKey) error {
	c.mu.RLock()
	modelID := c.modelID
	c.mu.RUnlock()

	if modelID == "" {
		return fmt.Errorf("no authorization model ID set")
	}

	deletes := make([]fgaSdk.TupleKeyWithoutCondition, len(tuples))
	for i, t := range tuples {
		deletes[i] = *fgaSdk.NewTupleKeyWithoutCondition(t.User, t.Relation, t.Object)
	}

	_, err := c.sdk.Write(ctx).Body(client.ClientWriteRequest{
		Deletes: deletes,
	}).Execute()
	return err
}

// ReadTuples reads tuples matching the given query.
func (c *Client) ReadTuples(ctx context.Context, query *fgaSdk.TupleKey) ([]*fgaSdk.Tuple, error) {
	c.mu.RLock()
	modelID := c.modelID
	c.mu.RUnlock()

	if modelID == "" {
		return nil, fmt.Errorf("no authorization model ID set")
	}

	user := query.User
	relation := query.Relation
	object := query.Object

	resp, err := c.sdk.Read(ctx).Body(client.ClientReadRequest{
		User:     &user,
		Relation: &relation,
		Object:   &object,
	}).Execute()
	if err != nil {
		return nil, err
	}
	result := make([]*fgaSdk.Tuple, len(resp.Tuples))
	for i := range resp.Tuples {
		result[i] = &resp.Tuples[i]
	}
	return result, nil
}

// ListUsers returns all users who have the given relation to an object.
func (c *Client) ListUsers(ctx context.Context, objectType, objectID, relation string) ([]string, error) {
	c.mu.RLock()
	modelID := c.modelID
	c.mu.RUnlock()

	if modelID == "" {
		return nil, fmt.Errorf("no authorization model ID set")
	}

	obj := fgaSdk.NewFgaObject(objectType, objectID)
	resp, err := c.sdk.ListUsers(ctx).Body(client.ClientListUsersRequest{
		Object:   *obj,
		Relation: relation,
	}).Execute()
	if err != nil {
		return nil, err
	}

	users := make([]string, 0, len(resp.Users))
	for _, u := range resp.Users {
		if u.Object != nil {
			users = append(users, fmt.Sprintf("%s:%s", u.Object.Type, u.Object.Id))
		} else if u.Userset != nil {
			users = append(users, fmt.Sprintf("%s:%s#%s", u.Userset.Type, u.Userset.Id, u.Userset.Relation))
		}
	}
	return users, nil
}

// Check checks if a user has a relation to an object.
func (c *Client) Check(ctx context.Context, user, relation, object string) (bool, error) {
	c.mu.RLock()
	modelID := c.modelID
	c.mu.RUnlock()

	if modelID == "" {
		return false, fmt.Errorf("no authorization model ID set")
	}

	resp, err := c.sdk.Check(ctx).Body(client.ClientCheckRequest{
		User:     user,
		Relation: relation,
		Object:   object,
	}).Execute()
	if err != nil {
		return false, err
	}
	allowed := false
	if resp.Allowed != nil {
		allowed = *resp.Allowed
	}
	return allowed, nil
}

// GenID-specific tuple helpers using fgaSdk.NewTupleKey constructor

// TupleUserHasRole creates a tuple: user has role.
func TupleUserHasRole(userID, roleID string) *fgaSdk.TupleKey {
	return fgaSdk.NewTupleKey(
		fmt.Sprintf("user:%s", userID),
		"member",
		fmt.Sprintf("role:%s", roleID),
	)
}

// TupleRoleGrantsResource creates a tuple: role grants resource.
func TupleRoleGrantsResource(roleID, resourceID string) *fgaSdk.TupleKey {
	return fgaSdk.NewTupleKey(
		fmt.Sprintf("role:%s", roleID),
		"grants",
		fmt.Sprintf("resource:%s", resourceID),
	)
}

// TupleUserCanAccess creates a tuple: user can access resource (direct).
func TupleUserCanAccess(userID, resourceID string) *fgaSdk.TupleKey {
	return fgaSdk.NewTupleKey(
		fmt.Sprintf("user:%s", userID),
		"can_access",
		fmt.Sprintf("resource:%s", resourceID),
	)
}

// TupleApprovalApprover creates a tuple: user is approver for approval request.
func TupleApprovalApprover(userID, approvalID string) *fgaSdk.TupleKey {
	return fgaSdk.NewTupleKey(
		fmt.Sprintf("user:%s", userID),
		"approver",
		fmt.Sprintf("approval:%s", approvalID),
	)
}

// TupleApprovalDelegate creates a tuple: user delegates approval authority.
func TupleApprovalDelegate(delegatorID, delegateeID, approvalID string) *fgaSdk.TupleKey {
	return fgaSdk.NewTupleKey(
		fmt.Sprintf("user:%s", delegateeID),
		"delegate",
		fmt.Sprintf("approval:%s", approvalID),
	)
}

// TupleApprovalTarget creates a tuple: target identity for approval.
func TupleApprovalTarget(targetID, approvalID string) *fgaSdk.TupleKey {
	return fgaSdk.NewTupleKey(
		fmt.Sprintf("user:%s", targetID),
		"target",
		fmt.Sprintf("approval:%s", approvalID),
	)
}

// TupleApprovalResource creates a tuple: resource for approval.
func TupleApprovalResource(resourceID, approvalID string) *fgaSdk.TupleKey {
	return fgaSdk.NewTupleKey(
		fmt.Sprintf("resource:%s", resourceID),
		"resource",
		fmt.Sprintf("approval:%s", approvalID),
	)
}
