package stores

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// Store is the single data-access layer: PostgreSQL for the relational
// identity/connector model, Neo4j for the access graph. It owns no business
// logic and no HTTP concerns — every method is a pure DB operation.
type Store struct {
	pg    *pgxpool.Pool
	neo4j neo4j.DriverWithContext
}

// NewStore wires the two graph/database connections the identity model needs.
func NewStore(pg *pgxpool.Pool, neo4jDriver neo4j.DriverWithContext) *Store {
	return &Store{pg: pg, neo4j: neo4jDriver}
}

// Pool returns the PostgreSQL connection pool.
func (s *Store) Pool() *pgxpool.Pool { return s.pg }

// Neo4j returns the Neo4j driver.
func (s *Store) Neo4j() neo4j.DriverWithContext { return s.neo4j }
