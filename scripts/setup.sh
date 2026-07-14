#!/bin/bash
# ─── ObserveID Reimagined Development Setup Script ────────
set -e

echo "═══════════════════════════════════════════"
echo "  ObserveID Reimagined Development Environment Setup"
echo "═══════════════════════════════════════════"

# Check prerequisites
echo "🔍 Checking prerequisites..."

check_cmd() {
    if ! command -v "$1" &> /dev/null; then
        echo "❌ $1 is not installed. Please install it first."
        exit 1
    fi
    echo "  ✓ $1 found"
}

check_cmd "go"
check_cmd "node"
check_cmd "docker"
check_cmd "npm"
check_cmd "protoc"

echo ""
echo "✅ All prerequisites met."
echo ""

# Install Go tools
echo "📦 Installing Go tools..."
cd backend
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/99designs/gqlgen@latest
cd ..

# Install Node dependencies
echo "📦 Installing frontend dependencies..."
cd frontend
npm install
cd ..

# Generate protobuf
echo "📜 Generating protobuf code..."
make proto

# Setup Docker environment
echo "🐳 Starting infrastructure containers..."
make up

# Wait for databases to be ready
echo "⏳ Waiting for databases..."
sleep 15

# Run database migrations
echo "🗄️  Running database migrations..."
make dev-db

echo ""
echo "═══════════════════════════════════════════"
echo "  ✅ Setup Complete!"
echo "═══════════════════════════════════════════"
echo ""
echo "  📊 Dashboard:   http://localhost:3001"
echo "  🔧 Temporal UI:  http://localhost:8233"
echo "  📈 Grafana:      http://localhost:3000"
echo "  🗄️  Neo4j:       http://localhost:7474"
echo "  📮 Kafka:        localhost:9092"
echo ""
echo "  Run 'make backend' to build Go services"
echo "  Run 'make frontend-dev' to start the frontend"
echo ""
