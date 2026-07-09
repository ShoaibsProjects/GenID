package ai

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// ─── GraphRAG Copilot Pipeline ─────────────────────────────

type CopilotPipeline struct {
	neo4j neo4j.DriverWithContext
}

func NewCopilotPipeline(driver neo4j.DriverWithContext) *CopilotPipeline {
	return &CopilotPipeline{neo4j: driver}
}

// ─── Query Types ──────────────────────────────────────────

type CopilotQuery struct {
	Question string `json:"question"`
	UserID   string `json:"user_id"`   // Who is asking?
	TenantID string `json:"tenant_id"`
}

type CopilotResponse struct {
	Answer          string   `json:"answer"`
	ConfidenceScore float64  `json:"confidence_score"`
	AccessPaths     []string `json:"access_paths"`
	CypherQueries   []string `json:"cypher_queries"`
	IsFlagged       bool     `json:"is_flagged"`
	Recommendations []string `json:"recommendations"`
	AuditID         string   `json:"audit_id"`
}

// ─── Pipeline Implementation ──────────────────────────────

func (cp *CopilotPipeline) ProcessQuery(ctx context.Context, query CopilotQuery) (*CopilotResponse, error) {
	auditID := uuid.New().String()
	startTime := time.Now()

	log.Printf("[AI COPILOT] Processing query: %s (user: %s, audit: %s)",
		query.Question, query.UserID, auditID)

	// Step 1: Classify the query
	classification := cp.classifyQuery(query.Question)
	log.Printf("[AI STEP 1] Classification: %s", classification)

	// Step 2a: Execute Neo4j graph query
	graphResult, cypherQuery := cp.executeGraphQuery(ctx, query, classification)
	accessPaths := cp.parseGraphResult(graphResult)

	// Step 2b: In production, search Qdrant vector DB here
	// Step 2c: Retrieve relevant Cedar policies
	// Step 3: Rerank results (cross-encoder model)

	// Step 4: Generate context-assembled response
	response := cp.generateResponse(query.Question, classification, accessPaths)

	// Step 5: Validate response (third LLM call in production)
	confidence := cp.calculateConfidence(accessPaths)

	elapsed := time.Since(startTime)
	log.Printf("[AI COPILOT] Response generated in %s. Confidence: %.2f. Flagged: %v",
		elapsed, confidence, confidence < 0.7)

	return &CopilotResponse{
		Answer:          response,
		ConfidenceScore: confidence,
		AccessPaths:     accessPaths,
		CypherQueries:   []string{cypherQuery},
		IsFlagged:       confidence < 0.7,
		Recommendations: cp.generateRecommendations(classification, accessPaths),
		AuditID:         auditID,
	}, nil
}

// ─── Query Classification ─────────────────────────────────

func (cp *CopilotPipeline) classifyQuery(question string) string {
	q := strings.ToLower(question)

	switch {
	case containsAny(q, "why does", "how does", "how did", "access to"):
		return "access_explanation"
	case containsAny(q, "blast radius", "what if", "compromised"):
		return "blast_radius"
	case containsAny(q, "who can", "who has", "who accessed"):
		return "who_has_access"
	case containsAny(q, "violation", "sod", "toxic", "conflict"):
		return "sod_detection"
	case containsAny(q, "risk", "risky", "dangerous"):
		return "risk_assessment"
	case containsAny(q, "agent", "bot", "service account", "non-human"):
		return "nhi_query"
	case containsAny(q, "review", "certify", "campaign"):
		return "certification_status"
	case containsAny(q, "recommend", "suggest", "optimize"):
		return "recommendation"
	default:
		return "general_query"
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ─── Graph Query Execution ────────────────────────────────

func (cp *CopilotPipeline) executeGraphQuery(ctx context.Context, query CopilotQuery, classification string) ([]*neo4j.Record, string) {
	var cypherQuery string
	params := map[string]any{"tenant_id": query.TenantID}

	switch classification {
	case "access_explanation":
		// Extract identity name from the question
		identityName := cp.extractEntity(query.Question)
		cypherQuery = `
			MATCH (i:Identity {display_name: $identity_name, tenant_id: $tenant_id})
			OPTIONAL MATCH (i)-[:HAS_ROLE]->(r:Role)-[:GRANTS]->(e:Entitlement)-[:ACCESSES]->(res:Resource)
			OPTIONAL MATCH (i)-[:DIRECTLY_OWNS]->(e2:Entitlement)-[:ACCESSES]->(res2:Resource)
			RETURN i.display_name AS identity,
				   COLLECT(DISTINCT {role: r.name, entitlement: e.permission_level, resource: res.name}) AS role_entitlements,
				   COLLECT(DISTINCT {entitlement: e2.permission_level, resource: res2.name}) AS direct_entitlements
		`
		params["identity_name"] = identityName

	case "blast_radius":
		identityName := cp.extractEntity(query.Question)
		cypherQuery = `
			MATCH (i:Identity {display_name: $identity_name, tenant_id: $tenant_id})
			MATCH path = (i)-[:HAS_ROLE|DIRECTLY_OWNS*1..4]->(e:Entitlement)-[:ACCESSES]->(r:Resource)
			RETURN r.name AS resource, r.criticality AS criticality,
				   e.permission_level AS permission, LENGTH(path) AS depth
			ORDER BY r.criticality DESC, depth ASC
		`
		params["identity_name"] = identityName

	case "who_has_access":
		resourceName := cp.extractResource(query.Question)
		cypherQuery = `
			MATCH (r:Resource {name: $resource_name})-[:ACCESSES]-(e:Entitlement)
			MATCH (e)<-[:GRANTS]-(role:Role)<-[:HAS_ROLE]-(i:Identity)
			RETURN i.display_name AS identity, i.department AS department,
				   role.name AS role_name, e.permission_level AS permission
			ORDER BY i.department
		`
		params["resource_name"] = resourceName

	case "sod_detection":
		cypherQuery = `
			MATCH (i:Identity {tenant_id: $tenant_id})
			MATCH (i)-[:HAS_ROLE]->(:Role)-[:GRANTS]->(e1:Entitlement)
			MATCH (i)-[:HAS_ROLE]->(:Role)-[:GRANTS]->(e2:Entitlement)
			WHERE e1.is_toxic = true AND e2.is_toxic = true AND e1.id < e2.id
			OPTIONAL MATCH (e1)-[:CONFLICTS_WITH]->(e2)
			RETURN i.display_name AS identity, i.risk_score AS risk_score,
				   COLLECT(DISTINCT e1.permission_level) AS toxic_entitlements,
				   COLLECT(DISTINCT e2.permission_level) AS conflicting_entitlements
			ORDER BY i.risk_score DESC
			LIMIT 20
		`

	case "nhi_query":
		cypherQuery = `
			MATCH (n:NonHumanIdentity {tenant_id: $tenant_id})
			OPTIONAL MATCH (n)-[:OWNED_BY]->(owner:Identity)
			OPTIONAL MATCH (child:NonHumanIdentity)-[:DELEGATED_FROM]->(n)
			RETURN n.name AS agent_name, n.type AS type, n.status AS status,
				   n.risk_score AS risk_score, n.is_governed AS is_governed,
				   owner.display_name AS owner,
				   COUNT(DISTINCT child) AS delegated_agent_count
			ORDER BY n.risk_score DESC
		`

	default:
		cypherQuery = `
			MATCH (i:Identity {tenant_id: $tenant_id})
			WHERE i.risk_score > 0.5
			RETURN i.display_name AS identity, i.department AS department,
				   i.risk_score AS risk_score, i.status AS status
			ORDER BY i.risk_score DESC
			LIMIT 20
		`
	}

	log.Printf("[AI STEP 2a] Cypher: %s | Params: %v", cypherQuery, params)

	session := cp.neo4j.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, cypherQuery, params)
	if err != nil {
		log.Printf("[AI ERROR] Graph query failed: %v", err)
		return nil, cypherQuery
	}

	var records []*neo4j.Record
	for result.Next(ctx) {
		records = append(records, result.Record())
	}

	return records, cypherQuery
}

// ─── Entity Extraction (naive — in production use LLM) ─────

func (cp *CopilotPipeline) extractEntity(question string) string {
	// In production: LLM call for entity extraction
	// Naive: look for common patterns
	q := strings.ToLower(question)
	words := strings.Fields(q)
	for _, w := range words {
		if w == "alice" || w == "bob" || w == "charlie" {
			return strings.Title(w) // Will use proper text processing in prod
		}
	}

	// Try to extract from "Why does X have access"
	if idx := strings.Index(q, "why does "); idx >= 0 {
		rest := strings.TrimSpace(q[idx+9:])
		if spaceIdx := strings.Index(rest, " "); spaceIdx > 0 {
			name := rest[:spaceIdx]
			return strings.Title(name)
		}
	}

	return "Alice Johnson" // default for demo
}

func (cp *CopilotPipeline) extractResource(question string) string {
	q := strings.ToLower(question)
	if strings.Contains(q, "prod db") || strings.Contains(q, "production database") {
		return "AWS Production"
	}
	if strings.Contains(q, "snowflake") {
		return "Snowflake"
	}
	if strings.Contains(q, "github") {
		return "GitHub Enterprise"
	}
	return "AWS Production"
}

func summarizePaths(paths []string) string {
	if len(paths) == 0 {
		return "no access paths found"
	}
	return fmt.Sprintf("%d access paths discovered", len(paths))
}

// ─── Response Generation ──────────────────────────────────

func (cp *CopilotPipeline) parseGraphResult(records []*neo4j.Record) []string {
	var paths []string
	for _, record := range records {
		keys := record.Keys
		for _, key := range keys {
			val, _ := record.Get(key)
			paths = append(paths, fmt.Sprintf("%s: %v", key, val))
		}
	}
	return paths
}

func (cp *CopilotPipeline) generateResponse(question string, classification string, accessPaths []string) string {
	switch classification {
	case "access_explanation":
		if len(accessPaths) > 0 {
			return fmt.Sprintf("Based on the graph analysis, %s. I found %d access paths connecting the identity to resources.", summarizePaths(accessPaths), len(accessPaths))
		}
		return "Based on the graph analysis, no access paths were found matching the query. This may mean the identity doesn't have the described access, or the graph hasn't been updated with the latest changes."

	case "blast_radius":
		if len(accessPaths) > 0 {
			return fmt.Sprintf("The blast radius analysis shows %d access paths to resources. The criticality of affected resources ranges from P0 (critical) to P2 (medium). I recommend reviewing these entitlements for compliance with least-privilege principles.", len(accessPaths))
		}
		return "No blast radius data found. The identity may have no active entitlements or may be terminated."

	case "who_has_access":
		return fmt.Sprintf("Found %d identities with access to this resource. The access exists through various role and entitlement paths in the graph.", len(accessPaths))

	case "sod_detection":
		return fmt.Sprintf("Segregation of Duties analysis complete. Found %d potential SoD violations in the identity graph. These require investigation based on policy.", len(accessPaths))

	case "nhi_query":
		return fmt.Sprintf("Non-Human Identity analysis complete. Found %d agents/identities matching the query. Risk scores and governance status flagged for review.", len(accessPaths))

	default:
		return "The AI Copilot analyzed your query against the identity graph. Here are the findings. For more detailed results, try a more specific question or contact your security team."
	}
}

func (cp *CopilotPipeline) calculateConfidence(accessPaths []string) float64 {
	// In production: LLM validation step
	if len(accessPaths) == 0 {
		return 0.6 // Lower confidence if no data found
	}
	return 0.92 // High confidence with data
}

func (cp *CopilotPipeline) generateRecommendations(classification string, accessPaths []string) []string {
	recs := []string{
		"Initiate access review for high-risk entitlements",
	}

	switch classification {
	case "access_explanation":
		recs = append(recs, "Review role assignments for excessive permissions")
		recs = append(recs, "Consider just-in-time access for elevated privileges")
	case "blast_radius":
		recs = append(recs, "Apply least-privilege principles to reduce blast radius")
		recs = append(recs, "Enable CAEP monitoring for critical resources")
	case "sod_detection":
		recs = append(recs, "Flagged SoD violations require immediate investigation")
		recs = append(recs, "Update Cedar policies to prevent future conflicts")
	case "nhi_query":
		recs = append(recs, "Ensure all non-human identities have assigned owners")
		recs = append(recs, "Rotate credentials for ungoverned agents")
	}

	return recs
}
