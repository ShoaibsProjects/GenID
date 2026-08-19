package notify

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
	"text/template"
	"time"
)

// EmailConfig holds SMTP configuration
type EmailConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
}

// EmailNotifier handles email notifications
type EmailNotifier struct {
	config   EmailConfig
	enabled  bool
	client   *smtp.Client
	templates map[string]*template.Template
}

// NewEmailNotifier creates a new email notifier from environment variables
func NewEmailNotifier() *EmailNotifier {
	host := os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")

	enabled := host != "" && user != "" && pass != ""
	port := 587
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}

	from := user
	if from == "" {
		from = "noreply@genid.local"
	}

	en := &EmailNotifier{
		config: EmailConfig{
			Host:     host,
			Port:     port,
			User:     user,
			Password: pass,
			From:     from,
		},
		enabled:  enabled,
		templates: make(map[string]*template.Template),
	}

	if enabled {
		en.initTemplates()
		log.Printf("[email] notifier initialized: host=%s port=%d user=%s", host, port, user)
	} else {
		log.Printf("[email] notifier disabled: missing SMTP config")
	}

	return en
}

// Enabled returns whether the email notifier is configured and enabled
func (e *EmailNotifier) Enabled() bool {
	return e.enabled
}

// initTemplates initializes email templates
func (e *EmailNotifier) initTemplates() {
	// Risk alert template
	riskTmpl := `Subject: {{.Subject}}
From: {{.From}}
To: {{.To}}
Content-Type: text/html; charset="UTF-8"

<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); color: white; padding: 30px; border-radius: 8px 8px 0 0; text-align: center; }
        .content { background: #f9f9f9; padding: 30px; border-radius: 0 0 8px 8px; }
        .risk-badge { display: inline-block; padding: 8px 16px; border-radius: 20px; font-weight: bold; font-size: 14px; }
        .risk-critical { background: #fee2e2; color: #dc2626; }
        .risk-high { background: #fed7aa; color: #ea580c; }
        .risk-elevated { background: #fef3c7; color: #d97706; }
        .risk-low { background: #dbeafe; color: #2563eb; }
        .risk-minimal { background: #d1fae5; color: #059669; }
        .details { background: white; padding: 20px; border-radius: 8px; margin: 20px 0; }
        .detail-row { display: flex; justify-content: space-between; padding: 10px 0; border-bottom: 1px solid #eee; }
        .detail-label { font-weight: 600; color: #666; }
        .detail-value { font-family: monospace; }
        .footer { text-align: center; color: #999; font-size: 12px; margin-top: 20px; }
        .button { display: inline-block; background: #667eea; color: white; padding: 12px 24px; border-radius: 6px; text-decoration: none; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🛡️ GenID Risk Alert</h1>
        </div>
        <div class="content">
            <p>Hello,</p>
            <p>A risk alert has been triggered for an identity in your organization.</p>
            
            <div class="details">
                <div class="detail-row">
                    <span class="detail-label">Identity:</span>
                    <span class="detail-value">{{.IdentityEmail}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Risk Band:</span>
                    <span class="detail-value"><span class="risk-badge risk-{{.RiskBand}}">{{.RiskBand | upper}}</span></span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Risk Score:</span>
                    <span class="detail-value">{{.RiskScore}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Trigger Event:</span>
                    <span class="detail-value">{{.EventType}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Event Source:</span>
                    <span class="detail-value">{{.Source}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Severity:</span>
                    <span class="detail-value">{{.Severity | upper}}</span>
                </div>
                <div class="detail-row">
                    <span class="detail-label">Timestamp:</span>
                    <span class="detail-value">{{.Timestamp}}</span>
                </div>
            </div>

            {{if .Reasons}}
            <h3>Risk Factors</h3>
            <ul>
            {{range .Reasons}}
                <li>{{.}}</li>
            {{end}}
            </ul>
            {{end}}

            {{if .Recommendations}}
            <h3>Recommended Actions</h3>
            <ul>
            {{range .Recommendations}}
                <li>{{.}}</li>
            {{end}}
            </ul>
            {{end}}

            <a href="{{.DashboardURL}}" class="button">View in GenID Dashboard</a>

            <div class="footer">
                <p>This is an automated alert from GenID Identity Governance Platform</p>
                <p>If you believe this is an error, please contact your security team.</p>
            </div>
        </div>
    </div>
</body>
</html>`

	tmpl, err := template.New("riskAlert").Funcs(template.FuncMap{
		"upper": strings.ToUpper,
	}).Parse(riskTmpl)
	if err != nil {
		log.Printf("[email] template parse error: %v", err)
		return
	}
	e.templates["riskAlert"] = tmpl
}

// RiskAlertData holds data for risk alert emails
type RiskAlertData struct {
	IdentityEmail    string
	RiskBand         string
	RiskScore        float64
	EventType        string
	Source           string
	Severity         string
	Timestamp        string
	Reasons          []string
	Recommendations  []string
	DashboardURL     string
	Subject          string
	From             string
	To               string
}

// SendRiskAlert sends a risk alert email
func (e *EmailNotifier) SendRiskAlert(ctx context.Context, data RiskAlertData) error {
	if !e.enabled {
		log.Printf("[email] notifier disabled, dropping risk alert for %s", data.IdentityEmail)
		return nil
	}

	// Set defaults
	if data.Subject == "" {
		data.Subject = fmt.Sprintf("[GenID] Risk Alert: %s - %s", data.RiskBand, data.IdentityEmail)
	}
	if data.From == "" {
		data.From = e.config.From
	}

	// Render template
	var buf bytes.Buffer
	if tmpl, ok := e.templates["riskAlert"]; ok {
		if err := tmpl.Execute(&buf, data); err != nil {
			return fmt.Errorf("template execution failed: %w", err)
		}
	} else {
		return fmt.Errorf("riskAlert template not found")
	}

	// Send email
	return e.sendEmail(data.To, buf.Bytes())
}

// sendEmail sends an email via SMTP
func (e *EmailNotifier) sendEmail(to string, msg []byte) error {
	auth := smtp.PlainAuth("", e.config.User, e.config.Password, e.config.Host)
	addr := fmt.Sprintf("%s:%d", e.config.Host, e.config.Port)

	// Split recipients
	recipients := strings.Split(to, ",")
	for i, r := range recipients {
		recipients[i] = strings.TrimSpace(r)
	}

	return smtp.SendMail(addr, auth, e.config.From, recipients, msg)
}

// SendTestEmail sends a test email
func (e *EmailNotifier) SendTestEmail(ctx context.Context, to string) error {
	if !e.enabled {
		return fmt.Errorf("email notifier not enabled")
	}

	data := RiskAlertData{
		IdentityEmail:   to,
		RiskBand:        "test",
		RiskScore:       0,
		EventType:       "test",
		Source:          "manual",
		Severity:        "info",
		Timestamp:       time.Now().Format(time.RFC3339),
		Reasons:         []string{"This is a test email from GenID"},
		Recommendations: []string{"No action required"},
		DashboardURL:    "http://localhost:3001",
		Subject:         "[GenID] Test Email",
		From:            e.config.From,
		To:              to,
	}

	return e.SendRiskAlert(ctx, data)
}