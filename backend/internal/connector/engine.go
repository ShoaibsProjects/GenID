package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type UniversalConnector struct {
	Manifest ConnectorManifest
}

func (uc *UniversalConnector) FetchUsers(ctx context.Context) ([]map[string]interface{}, error) {
	ep, ok := uc.Manifest.Endpoints["users"]
	if !ok {
		return nil, fmt.Errorf("no users endpoint defined in manifest")
	}

	method := ep.Method
	if method == "" {
		method = "GET"
	}

	req, err := http.NewRequestWithContext(ctx, method, ep.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch users: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch users: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch users: HTTP %d", resp.StatusCode)
	}

	var users []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("fetch users: decode response: %w", err)
	}

	return users, nil
}