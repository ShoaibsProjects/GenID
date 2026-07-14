# ─── ObserveID — Infrastructure as Code ────────────────────
# Provisions free-tier managed services for production.
#
# Prerequisites:
#   1. Neon account: https://neon.tech (free: 512MB, 1 project)
#   2. Upstash account: https://upstash.com (free: 10K cmds/day Redis, 10K msgs/day Kafka)
#   3. Terraform >= 1.5 installed
#
# Usage:
#   terraform init
#   terraform plan -var-file="terraform.tfvars"
#   terraform apply -var-file="terraform.tfvars"
#
# Not provisioned by Terraform (manual setup):
#   - Neo4j AuraDB Free: https://console.neo4j.cloud (free: 50K nodes, 175K rels)
#   - Temporal Cloud: https://temporal.io (free: 10K executions/mo)
#   - Fly.io: https://fly.io (free: 3 shared-cpu-1x, 256MB RAM)
#   - Cloudflare Pages: Already configured via GitHub Actions

terraform {
  required_version = ">= 1.5"
  required_providers {
    neon = {
      source  = "neon-tech/neon"
      version = "~> 0.3"
    }
  }
}

provider "neon" {
  api_key = var.neon_api_key
}

# ─── Neon PostgreSQL Project ──────────────────────────────
resource "neon_project" "observeid" {
  name      = "observeid-production"
  region_id = "aws-us-east-1"

  history_retention_seconds = 604800 # 7 days (free tier)
}

# Main branch / production database
resource "neon_branch" "main" {
  project_id = neon_project.observeid.id
  name       = "main"
  parent_id = neon_project.observeid.default_branch_id
}

# ─── Output: Connection Strings ───────────────────────────
output "neon_project_id" {
  description = "Neon project ID"
  value       = neon_project.observeid.id
}

output "neon_connection_string" {
  description = "PostgreSQL connection string (set as Fly.io secret DATABASE_URL)"
  value       = "postgresql://${neon_project.observeid.database_user}:${neon_project.observeid.database_password}@${neon_project.observeid.database_host}/${neon_project.observeid.database_name}?sslmode=require"
  sensitive   = true
}

output "neon_host" {
  description = "Neon PostgreSQL host"
  value       = neon_project.observeid.database_host
}

# ─── Upstash Resources (via HTTP API) ─────────────────────
# Upstash has no Terraform provider. These are documented steps.
# Run these commands after `terraform apply`:

# Redis:
#   curl -X POST https://api.upstash.com/v1/redis/database \
#     -H "Authorization: Bearer $UPSTASH_API_KEY" \
#     -d '{"name": "observeid-production", "region": "us-east-1", "type": "free"}'
#
# Kafka:
#   curl -X POST https://api.upstash.com/v1/kafka/cluster \
#     -H "Authorization: Bearer $UPSTASH_API_KEY" \
#     -d '{"name": "observeid-production", "region": "us-east-1", "multi_zone": false}'
#
# Kafka Topic:
#   curl -X POST https://api.upstash.com/v1/kafka/topic/identity.events \
#     -H "Authorization: Bearer $UPSTASH_API_KEY" \
#     -d '{"partitions": 1, "retention_ms": 604800000}'

# ─── Summary ──────────────────────────────────────────────
# After terraform apply, set these as Fly.io secrets:
#
#   fly secrets set \
#     DATABASE_URL="$(terraform output -raw neon_connection_string)" \
#     NEO4J_URI="neo4j+s://xxx.databases.neo4j.io" \
#     NEO4J_USER="neo4j" \
#     NEO4J_PASSWORD="<aura-password>" \
#     REDIS_ADDR="xxx.upstash.io:6379" \
#     REDIS_PASSWORD="<upstash-token>" \
#     REDIS_TLS="true" \
#     TEMPORAL_HOST="xxx.tmprl.cloud:7233" \
#     VAULT_MASTER_KEY="<hex-key>"
