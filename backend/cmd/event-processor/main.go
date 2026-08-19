package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/observeid/genid/internal/eventbus"
	"github.com/observeid/genid/internal/eventing"
	"github.com/observeid/genid/internal/eventing/sources"
)

func main() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	neo4jURI := os.Getenv("NEO4J_URI")
	if neo4jURI == "" {
		neo4jURI = "bolt://localhost:7687"
	}
	neo4jUser := os.Getenv("NEO4J_USER")
	if neo4jUser == "" {
		neo4jUser = "neo4j"
	}
	neo4jPass := os.Getenv("NEO4J_PASSWORD")
	if neo4jPass == "" {
		neo4jPass = "observeid123"
	}

	bus, err := eventbus.NewNatsBus(natsURL)
	if err != nil {
		log.Fatalf("[EVENT-PROCESSOR] failed to connect to NATS: %v", err)
	}
	defer bus.Close()

	driver, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPass, ""))
	if err != nil {
		log.Fatalf("[EVENT-PROCESSOR] failed to connect to Neo4j: %v", err)
	}
	defer driver.Close(context.Background())

	proc := eventing.NewProcessor(bus, driver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ─── Azure Service Bus edge adapter (optional) ───────────────
	// Active only when AZURE_SERVICEBUS_CONNECTION_STRING and
	// AZURE_SERVICEBUS_QUEUE are set. See ADR-001: brokers at the edge.
	if cs := os.Getenv("AZURE_SERVICEBUS_CONNECTION_STRING"); cs != "" {
		queue := os.Getenv("AZURE_SERVICEBUS_QUEUE")
		adapter, err := sources.NewAzureSBAdapter(sources.AzureSBConfig{
			ConnectionString: cs,
			Queue:            queue,
			Subscription:     os.Getenv("AZURE_SERVICEBUS_SUBSCRIPTION"),
			SourceName:       os.Getenv("AZURE_SERVICEBUS_SOURCE"),
		}, sources.DefaultRegistry(), bus)
		if err != nil {
			log.Printf("[EVENT-PROCESSOR] Azure SB adapter disabled: %v", err)
		} else {
			defer adapter.Close(context.Background())
			go func() {
				if err := adapter.Run(ctx); err != nil {
					log.Printf("[EVENT-PROCESSOR] Azure SB adapter stopped: %v", err)
				}
			}()
			log.Printf("[EVENT-PROCESSOR] Azure SB adapter listening on queue=%s", queue)
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("[EVENT-PROCESSOR] shutting down...")
		cancel()
		proc.Stop()
	}()

	log.Println("[EVENT-PROCESSOR] starting...")
	if err := proc.Start(ctx); err != nil {
		log.Fatalf("[EVENT-PROCESSOR] fatal: %v", err)
	}
}
