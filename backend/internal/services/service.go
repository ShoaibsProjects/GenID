package services

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"go.temporal.io/sdk/client"

	"github.com/observeid/genid/internal/audit"
	"github.com/observeid/genid/internal/cedar"
	"github.com/observeid/genid/internal/connector"
	"github.com/observeid/genid/internal/oidc"
	"github.com/observeid/genid/internal/outbox"
	"github.com/observeid/genid/internal/stores"
	"github.com/observeid/genid/internal/vault"
	"github.com/observeid/genid/internal/workflow"
)

// Service is the business-logic layer: it owns no HTTP and no raw DB
// concerns (data access lives in *stores.Store). Handlers hold a *Service
// and reach dependencies through the accessors below.
type Service struct {
	store        *stores.Store
	pgPool       *pgxpool.Pool
	neo4j        neo4j.DriverWithContext
	redis        *redis.Client
	temporal     client.Client
	connMgr      *connector.Manager
	provisionEng *connector.ProvisioningEngine
	vault        *vault.Vault
	auditLog     *audit.Store
	auditChain   *audit.Chain
	oidcProvider *oidc.Provider
	cedarEngine  *cedar.CedarEngine
	outbox       *outbox.Outbox
	wfStore      *workflow.Store
}

// NewService wires every dependency the identity domain needs, mirroring
// the original NewIdentityService construction exactly.
func NewService(pgPool *pgxpool.Pool, neo4j neo4j.DriverWithContext, rdb *redis.Client, tc client.Client) *Service {
	connMgr := connector.NewManager(pgPool)
	vaultPath := os.Getenv("VAULT_PATH")
	if vaultPath == "" {
		vaultPath = "/tmp/genid-vault.json"
	}
	vlt, err := vault.NewVault(os.Getenv("VAULT_MASTER_KEY"), vaultPath)
	if err != nil {
		log.Printf("[IDENTITY] Vault initialization failed: %v — continuing with in-memory-only vault", err)
		vlt, _ = vault.NewVault("default-insecure-key-do-not-use-in-production-32chars-min", "")
	}

	// Production-grade vault: prefer Postgres-backed secret storage so secrets
	// survive restarts and are shared across replicas. The PG backend uses
	// the `vault_secrets` table (migration 001) and is tenant-isolated at the
	// DB layer via RLS. Override with VAULT_BACKEND=file to keep legacy JSON-on-disk.
	if backend := os.Getenv("VAULT_BACKEND"); backend == "" || backend == "postgres" || backend == "pg" {
		if pgPool != nil {
			vlt.WithStore(vault.NewPGStore(pgPool))
			log.Printf("[IDENTITY] Vault backend: postgres (vault_secrets table)")
		}
	} else if backend == "file" {
		log.Printf("[IDENTITY] Vault backend: file (%s)", vaultPath)
	}

	// Hand the vault to the connector manager so RegisterSecure can move
	// plaintext connector credentials into AES-256-GCM encrypted storage
	// instead of leaving them in connectors.config JSONB.
	connMgr.WithVault(vlt)
	alog := audit.NewStore(10000)
	achain := audit.NewChain(pgPool)
	oidcProv, err := oidc.NewProvider(pgPool, "http://localhost:8080")
	if err != nil {
		log.Printf("[IDENTITY] OIDC provider initialization failed: %v", err)
	}
	cedarEng := cedar.NewCedarEngine(pgPool)
	if err := cedarEng.LoadPolicies(context.Background(), ""); err != nil {
		log.Printf("[CEDAR] Initial policy load failed: %v", err)
	} else {
		log.Printf("[CEDAR] Loaded %d policies", cedarEng.PolicyCount(""))
	}
	outboxEng := outbox.NewOutbox(pgPool)
	wfStore := workflow.NewStore(pgPool)
	return &Service{
		store:        stores.NewStore(pgPool, neo4j),
		pgPool:       pgPool,
		neo4j:        neo4j,
		redis:        rdb,
		temporal:     tc,
		connMgr:      connMgr,
		provisionEng: connector.NewProvisioningEngine(connMgr),
		vault:        vlt,
		auditLog:     alog,
		auditChain:   achain,
		oidcProvider: oidcProv,
		cedarEngine:  cedarEng,
		outbox:       outboxEng,
		wfStore:      wfStore,
	}
}

// ─── Accessors ────────────────────────────────────────────

// Store returns the data-access layer (PostgreSQL + Neo4j).
func (s *Service) Store() *stores.Store { return s.store }

// AuditStore returns the in-memory audit ring buffer.
func (s *Service) AuditStore() *audit.Store { return s.auditLog }

// WorkflowStore returns the workflow/request persistence store.
func (s *Service) WorkflowStore() *workflow.Store { return s.wfStore }

// AuditChain returns the tamper-evident audit hash-chain ledger.
func (s *Service) AuditChain() *audit.Chain { return s.auditChain }

// SaveVault persists the vault to its backend.
func (s *Service) SaveVault() error { return s.vault.Save() }

// ConnectorManager returns the connector manager (sync + registry).
func (s *Service) ConnectorManager() *connector.Manager { return s.connMgr }

// TemporalClient returns the Temporal workflow client.
func (s *Service) TemporalClient() client.Client { return s.temporal }

// ProvisioningEngine returns the connector provisioning engine.
func (s *Service) ProvisioningEngine() *connector.ProvisioningEngine { return s.provisionEng }

// Pool returns the PostgreSQL connection pool.
func (s *Service) Pool() *pgxpool.Pool { return s.pgPool }

// Neo4j returns the Neo4j driver.
func (s *Service) Neo4j() neo4j.DriverWithContext { return s.neo4j }

// Redis returns the Redis client.
func (s *Service) Redis() *redis.Client { return s.redis }

// CedarEngine returns the Cedar policy evaluation engine.
func (s *Service) CedarEngine() *cedar.CedarEngine { return s.cedarEngine }

// Outbox returns the transactional outbox.
func (s *Service) Outbox() *outbox.Outbox { return s.outbox }

// Vault returns the secret vault.
func (s *Service) Vault() *vault.Vault { return s.vault }

// OIDCProvider returns the OIDC provider.
func (s *Service) OIDCProvider() *oidc.Provider { return s.oidcProvider }

// LoadConnectors warms the connector registry from the database.
func (s *Service) LoadConnectors(ctx context.Context) error {
	configs, err := s.connMgr.LoadAll(ctx)
	if err != nil {
		return err
	}
	logError("connector", fmt.Errorf("loaded %d connectors from database", len(configs)))
	return nil
}

// ConnectorTenantID resolves the tenant for a connector, falling back to the
// default tenant when the config carries none. Mirrors the tenant resolution
// the persist helpers used to do internally.
func (s *Service) ConnectorTenantID(connectorID string) (string, error) {
	cfg, err := s.connMgr.GetConfig(connectorID)
	if err != nil {
		return "", fmt.Errorf("get connector config: %w", err)
	}
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001" // default tenant
	}
	return tenantID, nil
}
