package enrichment

// EvaluateDeviceTrust maps the X-Device-Trust header value to the
// normalized trust tier. Header-only for the MVP — real MDM/JAMF
// integration is Phase 5.
func EvaluateDeviceTrust(headerValue string) string {
	switch headerValue {
	case "managed", "unmanaged":
		return headerValue
	default:
		return "unknown"
	}
}
