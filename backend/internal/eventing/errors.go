package eventing

import "fmt"

var (
	ErrMissingEventType = fmt.Errorf("event_type is required")
	ErrInvalidEvent     = fmt.Errorf("invalid event payload")
	ErrIdentityNotFound = fmt.Errorf("identity not found")
)
