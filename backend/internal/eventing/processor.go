package eventing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/observeid/genid/internal/eventbus"
	"github.com/observeid/genid/internal/notify"
	"github.com/observeid/genid/internal/risk"
)

type Processor struct {
	natsBus      *eventbus.NatsBus
	neo4j        neo4j.DriverWithContext
	riskWeights  map[string]float64
	sub          *nats.Subscription
	sessionMgr   *risk.SessionManager
	reviewMgr    *risk.MicroReview
	emailNotifier *notify.EmailNotifier
}

func NewProcessor(bus *eventbus.NatsBus, neo4j neo4j.DriverWithContext) *Processor {
	emailNotifier := notify.NewEmailNotifier()
	p := &Processor{
		natsBus:       bus,
		neo4j:         neo4j,
		emailNotifier: emailNotifier,
		riskWeights: map[string]float64{
			"_decay_rate":             5.0,
			"auth.failed_login":       100.0,
			"auth.mfa_failure":        75.0,
			"auth.impossible_travel":  150.0,
			"auth.password_spray":     125.0,
			"auth.brute_force":        175.0,
			"account.locked":          50.0,
			"session.anomalous":       100.0,
			"entitlement.escalation":  200.0,
			"access.off_hours":        50.0,
			"peer_deviation":          80.0,
			"dormant_account":         60.0,
			"privilege_escalation":    250.0,
			"credential_leaked":       300.0,
		},
		sessionMgr: risk.NewSessionManager(neo4j),
		reviewMgr:  risk.NewMicroReview(neo4j),
	}
	return p
}

// Start begins consuming genid-events as part of a NATS JetStream queue
// group named "risk-processor-q". Queue-group consumers share work: only
// one replica in the group receives each message, which lets the processor
// scale horizontally (N replicas → ~N× throughput) without duplicate
// processing. The durable name is the same as the queue name so the
// consumer state is persisted and resumes after a restart.
func (p *Processor) Start(ctx context.Context) error {
	sub, err := p.natsBus.JetStream().QueueSubscribe("genid-events.>", "risk-processor-q", func(msg *nats.Msg) {
		var evt eventbus.Event
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			log.Printf("[PROCESSOR] unmarshal error: %v", err)
			msg.Ack()
			return
		}

		signal := p.eventToSignal(evt)
		if signal.ScoreDelta == 0 {
			msg.Ack()
			return
		}

		if err := p.processSignal(ctx, signal); err != nil {
			log.Printf("[PROCESSOR] process error: %v", err)
			msg.Nak()
			return
		}

		msg.Ack()
	},
		nats.Durable("risk-processor-q"),
		nats.ManualAck(),
		nats.MaxDeliver(3),
		nats.AckWait(30*time.Second),
	)

	if err != nil {
		return fmt.Errorf("nats subscribe: %w", err)
	}

	p.sub = sub
	log.Printf("[PROCESSOR] started: queue-group=risk-processor-q durable=risk-processor-q subject=genid-events.> (manual-ack, max-deliver=3, ack-wait=30s)")
	<-ctx.Done()
	return nil
}

func (p *Processor) eventToSignal(evt eventbus.Event) RiskSignal {
	delta := p.riskWeights[evt.EventType]

	var meta map[string]any
	_ = json.Unmarshal(evt.Payload, &meta)

	identityID := evt.AggregateID
	if identityID == "" {
		if v, ok := meta["identity_id"].(string); ok {
			identityID = v
		}
	}

	source := "unknown"
	if v, ok := meta["source"].(string); ok {
		source = v
	}

	severity := "medium"
	if v, ok := meta["severity"].(string); ok {
		severity = v
	}

	override := 1.0
	switch severity {
	case "critical":
		override = 2.0
	case "high":
		override = 1.5
	case "low":
		override = 0.5
	}

	return RiskSignal{
		EventType:  evt.EventType,
		Source:     source,
		Severity:   severity,
		IdentityID: identityID,
		Timestamp:  evt.Timestamp,
		ScoreDelta: delta * override,
		Metadata:   evt.Payload,
	}
}

func (p *Processor) processSignal(ctx context.Context, signal RiskSignal) error {
	if signal.IdentityID == "" {
		log.Printf("[PROCESSOR] skipping signal: no identity_id")
		return nil
	}

	session := p.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func() {
		_ = session.Close(ctx)
	}()

	decayRate := 1.0
	if v, ok := p.riskWeights["_decay_rate"]; ok {
		decayRate = v
	}

	query := `
		MATCH (i) WHERE (i:Identity OR i:NonHumanIdentity) AND (i.uuid = $identityId OR i.email = $identityId)
		WITH i, coalesce(i.risk_score, 0) AS currentScore,
		     duration.between(coalesce(i.risk_last_updated, datetime()), datetime()) AS elapsed
		WITH i, currentScore, elapsed.hours AS hoursElapsed
		WITH i, currentScore, hoursElapsed,
		     CASE
		       WHEN currentScore - ($decayRate * hoursElapsed) < 0 THEN 0
		       ELSE currentScore - ($decayRate * hoursElapsed)
		     END AS decayedScore
		WITH i, currentScore, decayedScore, hoursElapsed,
		     CASE
		       WHEN decayedScore + $delta > 1000 THEN 1000
		       WHEN decayedScore + $delta < 0 THEN 0
		       ELSE decayedScore + $delta
		     END AS newScore,
		     CASE
		       WHEN coalesce(i.risk_dynamic, 0) - ($decayRate * hoursElapsed) + $delta > 1000 THEN 1000
		       WHEN coalesce(i.risk_dynamic, 0) - ($decayRate * hoursElapsed) + $delta < 0 THEN 0
		       ELSE coalesce(i.risk_dynamic, 0) - ($decayRate * hoursElapsed) + $delta
		     END AS newDynamicScore
		SET i.risk_score = newScore,
		    i.risk_dynamic = newDynamicScore,
		    i.risk_band = CASE
		       WHEN newScore >= 800 THEN 'critical'
		       WHEN newScore >= 600 THEN 'high'
		       WHEN newScore >= 300 THEN 'elevated'
		       WHEN newScore >= 100 THEN 'low'
		       ELSE 'minimal'
		    END,
		    i.risk_last_event = $eventType,
		    i.risk_last_source = $source,
		    i.risk_last_severity = $severity,
		    i.risk_event_count = coalesce(i.risk_event_count, 0) + 1,
		    i.risk_last_updated = datetime(),
		    i.updated_at = datetime()
		RETURN currentScore, newScore
	`

	result, err := session.Run(ctx, query, map[string]any{
		"identityId": signal.IdentityID,
		"delta":      math.Round(signal.ScoreDelta*100) / 100,
		"decayRate":  decayRate,
		"eventType":  signal.EventType,
		"source":     signal.Source,
		"severity":   signal.Severity,
	})
	if err != nil {
		return fmt.Errorf("neo4j update: %w", err)
	}

	if result.Next(ctx) {
		rec := result.Record()
		oldScore, _ := rec.Get("currentScore")
		newScore, _ := rec.Get("newScore")
		newBand := "minimal"
		if ns, ok := toFloat64(newScore); ok {
			newBand = scoreToBand(ns)
		}
		log.Printf("[PROCESSOR] identity=%s event=%s risk: %v → %v (band=%s)",
			signal.IdentityID, signal.EventType, oldScore, newScore, newBand)

		_ = p.publishRiskUpdated(ctx, signal.IdentityID, oldScore, newScore)

		p.handleRiskActions(ctx, signal.IdentityID, newBand, signal.EventType)

		return nil
	}

	log.Printf("[PROCESSOR] identity not found: %s", signal.IdentityID)
	return nil
}

func (p *Processor) publishRiskUpdated(ctx context.Context, identityID string, oldScore, newScore any) error {
	updatedEvent := eventbus.Event{
		ID:          fmt.Sprintf("risk-updated-%s-%d", identityID, time.Now().UnixNano()),
		EventType:   "identity.events.risk.updated",
		AggregateID: identityID,
		TenantID:    "00000000-0000-0000-0000-000000000001",
		Payload: json.RawMessage(fmt.Sprintf(`{"identity_id":"%s","old_score":%v,"new_score":%v}`,
			identityID, oldScore, newScore)),
		Timestamp: time.Now().UTC(),
	}
	return p.natsBus.Publish(ctx, updatedEvent)
}

func (p *Processor) applyDecay(currentScore, hoursSinceLastEvent, decayRate float64) float64 {
	if hoursSinceLastEvent <= 0 {
		return currentScore
	}
	decayed := currentScore - (decayRate * hoursSinceLastEvent)
	if decayed < 0 {
		return 0
	}
	return decayed
}

func (p *Processor) handleRiskActions(ctx context.Context, identityID, band, eventType string) {
	// Get identity details for email
	identityEmail, riskScore := p.getIdentityDetails(ctx, identityID)
	
	switch band {
	case "critical":
		log.Printf("[PROCESSOR] CRITICAL: terminating all sessions for %s", identityID)
		if count, err := p.sessionMgr.TerminateAllSessions(ctx, identityID, "critical_risk_auto_revoke"); err != nil {
			log.Printf("[PROCESSOR] session termination error: %v", err)
		} else {
			log.Printf("[PROCESSOR] terminated %d sessions for %s", count, identityID)
		}
		_, _ = p.reviewMgr.TriggerReview(ctx, risk.ReviewTrigger{
			IdentityID:  identityID,
			TriggerType: "critical_risk",
			RiskScore:   800,
			RiskBand:    band,
			Description: fmt.Sprintf("Auto-triggered: critical risk detected via %s", eventType),
		})
		p.sendRiskEmail(ctx, identityEmail, band, riskScore, eventType, "critical", []string{
			"Critical risk threshold exceeded",
			"Immediate session termination executed",
			"Automated review triggered",
		}, []string{
			"Verify identity legitimacy immediately",
			"Investigate recent activity logs",
			"Consider temporary account suspension",
			"Review privileged access assignments",
		})
	case "high":
		_, _ = p.reviewMgr.TriggerReview(ctx, risk.ReviewTrigger{
			IdentityID:  identityID,
			TriggerType: "high_risk",
			RiskScore:   600,
			RiskBand:    band,
			Description: fmt.Sprintf("Review required: high risk detected via %s", eventType),
		})
		p.sendRiskEmail(ctx, identityEmail, band, riskScore, eventType, "high", []string{
			"High risk threshold exceeded",
			"Automated review triggered",
			"Elevated monitoring active",
		}, []string{
			"Review recent access patterns",
			"Verify MFA compliance",
			"Check for privilege escalation",
			"Schedule security review",
		})
	case "elevated":
		_, _ = p.reviewMgr.TriggerReview(ctx, risk.ReviewTrigger{
			IdentityID:  identityID,
			TriggerType: "elevated_risk",
			RiskScore:   300,
			RiskBand:    band,
			Description: fmt.Sprintf("Monitoring: elevated risk detected via %s", eventType),
		})
		// Optional: send email for elevated risk (less urgent)
		p.sendRiskEmail(ctx, identityEmail, band, riskScore, eventType, "elevated", []string{
			"Elevated risk detected",
			"Increased monitoring active",
		}, []string{
			"Monitor for further risk increases",
			"Review access patterns weekly",
		})
	}
}

// getIdentityDetails fetches identity email and risk score from Neo4j
func (p *Processor) getIdentityDetails(ctx context.Context, identityID string) (string, float64) {
	session := p.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func() { _ = session.Close(ctx) }()

	query := `
		MATCH (i) WHERE (i:Identity OR i:NonHumanIdentity) AND (i.uuid = $identityId OR i.email = $identityId)
		RETURN i.email, coalesce(i.risk_score, 0) AS riskScore
		LIMIT 1
	`

	result, err := session.Run(ctx, query, map[string]any{"identityId": identityID})
	if err != nil {
		log.Printf("[PROCESSOR] getIdentityDetails error: %v", err)
		return identityID, 0
	}

	if result.Next(ctx) {
		rec := result.Record()
		email, _ := rec.Get("email")
		score, _ := rec.Get("riskScore")
		emailStr := ""
		if email != nil {
			emailStr = email.(string)
		}
		scoreFloat := 0.0
		if s, ok := toFloat64(score); ok {
			scoreFloat = s
		}
		if emailStr == "" {
			emailStr = identityID
		}
		return emailStr, scoreFloat
	}

	return identityID, 0
}

// sendRiskEmail sends a risk alert email via the email notifier
func (p *Processor) sendRiskEmail(ctx context.Context, identityEmail, band string, riskScore float64, eventType, severity string, reasons, recommendations []string) {
	if p.emailNotifier == nil || !p.emailNotifier.Enabled() {
		return
	}

	// Get admin email from environment or use identity email
	toEmail := identityEmail
	if adminEmail := os.Getenv("ADMIN_EMAIL"); adminEmail != "" {
		toEmail = adminEmail
	}

	data := notify.RiskAlertData{
		IdentityEmail:   identityEmail,
		RiskBand:        band,
		RiskScore:       riskScore,
		EventType:       eventType,
		Source:          "event-processor",
		Severity:        severity,
		Timestamp:       time.Now().Format(time.RFC3339),
		Reasons:         reasons,
		Recommendations: recommendations,
		DashboardURL:    os.Getenv("DASHBOARD_URL"),
		To:              toEmail,
	}

	if err := p.emailNotifier.SendRiskAlert(ctx, data); err != nil {
		log.Printf("[PROCESSOR] email send error: %v", err)
	} else {
		log.Printf("[PROCESSOR] risk alert email sent to %s for identity %s (band=%s)", toEmail, identityEmail, band)
	}
}

func scoreToBand(score float64) string {
	switch {
	case score >= 800:
		return "critical"
	case score >= 600:
		return "high"
	case score >= 300:
		return "elevated"
	case score >= 100:
		return "low"
	default:
		return "minimal"
	}
}

func (p *Processor) SetRiskWeight(eventType string, weight float64) {
	p.riskWeights[eventType] = weight
}

func (p *Processor) RiskWeight(eventType string) float64 {
	return p.riskWeights[eventType]
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

func (p *Processor) Stop() {
	if p.sub != nil {
		_ = p.sub.Unsubscribe()
	}
}
