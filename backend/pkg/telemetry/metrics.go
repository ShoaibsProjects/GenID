package telemetry

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ─── Metrics Definitions ──────────────────────────────────

var (
	// Identity Metrics
	IdentityTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "observeid_identities_total",
			Help: "Total number of identities by type and status",
		},
		[]string{"type", "status", "tenant_id"},
	)

	NHITotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "observeid_nhi_total",
			Help: "Total number of non-human identities by type and governance",
		},
		[]string{"type", "is_governed", "tenant_id"},
	)

	// Access Metrics
	AccessCheckTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "observeid_access_check_total",
			Help: "Total access check requests",
		},
		[]string{"decision", "tenant_id"},
	)

	AccessCheckLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "observeid_access_check_latency_ms",
			Help:    "Access check latency in milliseconds",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"tenant_id"},
	)

	// Workflow Metrics
	WorkflowExecutions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "observeid_workflow_executions_total",
			Help: "Total workflow executions by type and status",
		},
		[]string{"workflow_type", "status", "tenant_id"},
	)

	WorkflowExecutionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "observeid_workflow_duration_seconds",
			Help:    "Workflow execution duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"workflow_type", "tenant_id"},
	)

	// CAEP Metrics
	CAEPEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "observeid_caep_events_total",
			Help: "Total CAEP events by type and delivery status",
		},
		[]string{"event_type", "delivery_status", "tenant_id"},
	)

	// Agent Metrics
	AgentKillSwitchTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "observeid_agent_kill_switch_total",
			Help: "Total agent kill switch activations",
		},
		[]string{"tenant_id"},
	)

	AgentAnomalyTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "observeid_agent_anomalies_total",
			Help: "Total agent anomalies detected",
		},
		[]string{"type", "tenant_id"},
	)

	// Audit Metrics
	AuditEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "observeid_audit_events_total",
			Help: "Total audit events by type",
		},
		[]string{"event_type", "tenant_id"},
	)

	// Resource Metrics
	Neo4jQueryLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "observeid_neo4j_query_latency_ms",
			Help:    "Neo4j query latency in milliseconds",
			Buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000, 5000},
		},
		[]string{"query_type", "tenant_id"},
	)

	DLQSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "observeid_dlq_size",
			Help: "Dead letter queue size by topic",
		},
		[]string{"topic", "tenant_id"},
	)

	CedarEvaluationLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "observeid_cedar_evaluation_latency_ms",
			Help:    "Cedar policy evaluation latency in milliseconds",
			Buckets: []float64{0.5, 1, 2, 5, 10, 20, 50, 100},
		},
		[]string{"policy_effect", "tenant_id"},
	)

	CedarDenyRate = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "observeid_cedar_deny_total",
			Help: "Total Cedar deny decisions",
		},
		[]string{"principal_type", "action", "resource_type"},
	)

	OutboxQueueSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "observeid_outbox_queue_size",
			Help: "Current number of pending outbox events",
		},
	)

	OutboxEventsProcessed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "observeid_outbox_events_processed_total",
			Help: "Total outbox events processed successfully",
		},
	)

	OutboxEventsFailed = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "observeid_outbox_events_failed_total",
			Help: "Total outbox events that failed processing",
		},
	)

	OutboxProcessingLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "observeid_outbox_processing_latency_ms",
			Help:    "Outbox event processing latency in milliseconds",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"event_type"},
	)
)

// ─── Initialization ───────────────────────────────────────

func init() {
	prometheus.MustRegister(IdentityTotal)
	prometheus.MustRegister(NHITotal)
	prometheus.MustRegister(AccessCheckTotal)
	prometheus.MustRegister(AccessCheckLatency)
	prometheus.MustRegister(WorkflowExecutions)
	prometheus.MustRegister(WorkflowExecutionDuration)
	prometheus.MustRegister(CAEPEventsTotal)
	prometheus.MustRegister(AgentKillSwitchTotal)
	prometheus.MustRegister(AgentAnomalyTotal)
	prometheus.MustRegister(AuditEventsTotal)
	prometheus.MustRegister(Neo4jQueryLatency)
	prometheus.MustRegister(DLQSize)
	prometheus.MustRegister(CedarEvaluationLatency)
	prometheus.MustRegister(CedarDenyRate)
	prometheus.MustRegister(OutboxQueueSize)
	prometheus.MustRegister(OutboxEventsProcessed)
	prometheus.MustRegister(OutboxEventsFailed)
	prometheus.MustRegister(OutboxProcessingLatency)
}

func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
