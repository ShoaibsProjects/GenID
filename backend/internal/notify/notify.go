package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// ─── Webhook Notifications ────────────────────────────────
// Delivers approval events to Slack / Teams incoming webhooks.
// Both channels get the same structured event; each formatter
// maps it to the channel's expected payload. If no webhook URL
// is configured the event is logged and dropped (never fails
// the caller).

type Notifier struct {
	slackURL string
	teamsURL string
	client   *http.Client
	enabled  bool
}

func New() *Notifier {
	n := &Notifier{
		slackURL: os.Getenv("SLACK_WEBHOOK_URL"),
		teamsURL: os.Getenv("TEAMS_WEBHOOK_URL"),
		client:   &http.Client{Timeout: 10 * time.Second},
	}
	n.enabled = n.slackURL != "" || n.teamsURL != ""
	return n
}

// NewFromURLs is used by tests to inject fake webhook endpoints.
func NewFromURLs(slackURL, teamsURL string) *Notifier {
	n := New()
	n.slackURL = slackURL
	n.teamsURL = teamsURL
	n.enabled = slackURL != "" || teamsURL != ""
	return n
}

// Event is the structured payload every notification carries.
type Event struct {
	Event        string `json:"event"`                  // approval.required / approval.decided / approval.timed_out
	RequestID    string `json:"request_id"`             //
	RequestType  string `json:"request_type"`           // access.request.grant etc
	ApprovalID   string `json:"approval_id"`            //
	Level        int    `json:"level"`                  //
	ApproverRole string `json:"approver_role"`          //
	ApproverID   string `json:"approver_id,omitempty"`  //
	ApproverEmail string `json:"approver_email"`        //
	Status       string `json:"status,omitempty"`       // approved / denied / pending
	Comment      string `json:"comment,omitempty"`      //
	DueAt        string `json:"due_at,omitempty"`       // RFC3339
	DecidedAt    string `json:"decided_at,omitempty"`   // RFC3339
}

// Notify delivers the event to every configured channel.
// Never returns an error to the caller (best-effort); failures
// are logged so the workflow can proceed.
func (n *Notifier) Notify(ctx context.Context, ev Event) error {
	if !n.enabled {
		log.Printf("[notify] no webhook configured, dropping %s for request %s", ev.Event, ev.RequestID)
		return nil
	}
	var errs []error
	if n.slackURL != "" {
		if err := n.post(ctx, n.slackURL, slackPayload(ev)); err != nil {
			errs = append(errs, fmt.Errorf("slack: %w", err))
		}
	}
	if n.teamsURL != "" {
		if err := n.post(ctx, n.teamsURL, teamsPayload(ev)); err != nil {
			errs = append(errs, fmt.Errorf("teams: %w", err))
		}
	}
	if len(errs) > 0 {
		log.Printf("[notify] webhook delivery failed: %v", errs)
		return errs[0]
	}
	return nil
}

func (n *Notifier) post(ctx context.Context, url string, body any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}

// ─── Channel formatters ───────────────────────────────────

func slackPayload(ev Event) map[string]any {
	emoji := map[string]string{
		"approval.required":  "🕓",
		"approval.decided":   "✅",
		"approval.timed_out": "⏰",
	}[ev.Event]
	if ev.Status == "denied" {
		emoji = "⛔"
	}
	return map[string]any{
		"text": fmt.Sprintf("%s %s\n• request `%s` (%s)\n• level %d — *%s*\n%s",
			emoji, ev.Event, short(ev.RequestID), ev.RequestType, ev.Level, ev.ApproverRole, ev.Comment),
	}
}

func teamsPayload(ev Event) map[string]any {
	color := "#FBBF24"
	switch ev.Event {
	case "approval.decided":
		color = "#34D399"
		if ev.Status == "denied" {
			color = "#F87171"
		}
	case "approval.timed_out":
		color = "#F87171"
	}
	return map[string]any{
		"@type":      "MessageCard",
		"@context":   "https://schema.org/extensions",
		"themeColor": color,
		"summary":    ev.Event,
		"title":      fmt.Sprintf("%s (level %d)", ev.Event, ev.Level),
		"text": fmt.Sprintf("request `%s` (%s) • approver **%s**%s\n%s",
			short(ev.RequestID), ev.RequestType, ev.ApproverRole, ev.ApproverEmail, ev.Comment),
	}
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}