# GenID · Deployment Guide

> **Target:** Oracle Cloud **Always Free** Ampere A1.Flex VM (Linux/Ubuntu) + **Cloudflare Tunnel**.
> Monthly cost: **$0**. Every public-facing port in `docker-compose.yml` is bound to `127.0.0.1`
> — the only ingress is Cloudflare's authenticated tunnel. No firewall inbound rules needed.

This guide assumes you (the operator) can run a few commands on the VM. I cannot provision the VM,
sign in to Cloudflare, or paste SSH keys from here — those steps must be done by hand; everything
else is copy/paste.

---

## 0. What ends up running

| Container        | Port (127.0.0.1 only) | Purpose |
|------------------|-----------------------|---------|
| `genid-postgres` | 5433                  | PostgreSQL 16 + RLS + init.sql |
| `genid-neo4j`    | 7474 / 7687           | Neo4j 5 (identity graph) |
| `genid-redis`    | 6379                  | JTI revocation + locks |
| `genid-nats`     | 4222 / 8222           | NATS JetStream (events) |
| `genid-temporal` | 7233 / 8233           | Durable workflows (kill switch) |
| `genid-temporal-ui` | 8234              | Temporal Web UI |
| `genid-grafana`  | 3000                  | Observability |
| `genid-otel`     | 4317 / 4318           | OpenTelemetry collector |
| `genid-identity-service` | 8080          | Go API gateway + in-process Temporal worker |
| `genid-frontend` | 3001                  | Next.js 14 static-export (nginx) |

**Only** `api.YOUR_DOMAIN` and `app.YOUR_DOMAIN` (via Cloudflare Tunnel) reach the internet.

---

## 1. Provision the Oracle VM (manual — once)

1. Sign in to **Oracle Cloud Console → Compute → Instances → Create instance**.
   - Shape: **VM.Standard.A1.Flex** · **4 OCPU · 24 GB RAM** (Always Free).
   - Image: **Canonical Ubuntu 22.04**.
   - Add SSH public key (generate one with `ssh-keygen -t ed25519` locally).
2. After boot, grab the **Public IP** and SSH in:
   ```bash
   ssh -i ~/.ssh/genid_oracle ubuntu@<PUBLIC_IP>
   ```

## 2. Hardened base setup on the VM

```bash
# OS config: Neo4j needs higher map count, Temporal needs sane vm limits
sudo sysctl -w vm.max_map_count=524288
echo 'vm.max_map_count=524288' | sudo tee -a /etc/sysctl.conf

# Docker (official repo — don't use apt's old Docker)
sudo apt update && sudo apt install -y ca-certificates curl gnupg lsb-release git
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list
sudo apt update && sudo apt install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo usermod -aG docker $USER && newgrp docker

# Cloudflare Tunnel
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://pkg.cloudflare.com/cloudflare-main.gpg | sudo tee /etc/apt/keyrings/cloudflare-main.gpg >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/cloudflare-main.gpg] https://pkg.cloudflare.com/cloudflared $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/cloudflared.list
sudo apt update && sudo apt install -y cloudflared
```

## 3. Pull the repo + secrets

```bash
git clone https://github.com/ShoaibsProjects/GenID.git
cd GenID

# Generate production secrets (do NOT reuse dev values)
printf 'VAULT_MASTER_KEY=%s\n' "$(openssl rand -hex 32)"  >> backend/.env
printf 'JWT_SIGNING_KEY=%s\n'  "$(openssl rand -hex 32)"   >> backend/.env
printf 'API_KEYS=admin:%s\n'    "$(openssl rand -hex 24)"   >> backend/.env
printf 'NEO4J_PASSWORD=%s\n'   "$(openssl rand -hex 16)"   >> backend/.env
# now edit infrastructure/docker-compose.yml lines 31 and 234 to use the new NEO4J_PASSWORD
# (and backend/.env NEO4J_PASSWORD) — keep them identical across the three places
```

> Keep `MASTER_KEY` (gateway admin) and `VAULT_MASTER_KEY` (AES-256 vault) **different**.

## 4. Bring up the stack

```bash
cd infrastructure
docker compose up -d
docker compose ps        # every service should be 'healthy' (~60s for first boot)
docker compose logs -f identity-service
```

Sanity check (from the VM):
```bash
curl -s http://127.0.0.1:8080/health
curl -s http://127.0.0.1:8080/ready
```

## 5. Wire Cloudflare Tunnel (public ingress)

```bash
cloudflared tunnel login                              # browser one-time
cloudflared tunnel create genid                        # prints <TUNNEL_ID>
sudo cp ./infrastructure/cloudflare/config.yml /root/.cloudflared/config.yml
# edit /root/.cloudflared/config.yml:
#   replace <TUNNEL_ID>  with the id printed above
#   move /root/.cloudflared/<TUNNEL_ID>.json (auto-created) to match credentials-file
#   replace YOUR_DOMAIN     with your domain, e.g. example.com
# then add Cloudflare DNS records (automatic):
cloudflared tunnel route dns genid api.YOUR_DOMAIN
cloudflared tunnel route dns genid app.YOUR_DOMAIN

sudo cloudflared service install                      # start on boot
sudo systemctl status cloudflared
```

You're live:
- API → `https://api.YOUR_DOMAIN`
- UI  → `https://app.YOUR_DOMAIN`

Cloudflare WAF, HTTPS (TLS 1.3) and DDoS protection are on by default for the zone.

## 6. Verify end-to-end (from your laptop, not the VM)

```bash
# Health
curl -s https://api.YOUR_DOMAIN/health

# Authenticated call — paste your real API key from step 3
curl -s -H "X-API-Key: <ADMIN_KEY>" \
  https://api.YOUR_DOMAIN/api/v1/identities
```

If you used the demo seed (`infrastructure/postgres/init.sql` ships the GenID demo tenant +
the Cedar policies), expect a populated identity list. The Temporal kill-switch UI is at
`http://127.0.0.1:8234` **on the VM** (since it's bound to localhost); tunnel it with
`ssh -L 8234:127.0.0.1:8234 -i ~/.ssh/genid_oracle ubuntu@<PUBLIC_IP>` for a browser view.

## 7. Day-2 operations

| Need | Command |
|------|---------|
| Tail logs | `docker compose logs -f --tail=200 identity-service` |
| Restart stack | `docker compose restart` |
| Rebuild after code change | `docker compose build identity-service frontend && docker compose up -d` |
| Hot-reload Cedar policies | drop new `policies/*.cedar`; the engine picks them up without restart |
| Rotate API key | change `API_KEYS` in `backend/.env`; `docker compose up -d identity-service` |
| Backup Postgres | `docker exec genid-postgres pg_dump -U observeid observeid > backup-$(date +%F).sql` |
| Tunnel health | `sudo systemctl status cloudflared` |

## 8. Cost model

| Item | Cost |
|------|------|
| Oracle A1.Flex (Always Free) | **$0** |
| Cloudflare Tunnel | **$0** (unlimited) |
| Cloudflare-managed DNS + TLS + WAF | **$0** |
| Domain registration | existing or ~$10/yr |

Total: **$0/month** minus the domain.

---

## Post-deploy checklist

- [ ] `docker compose ps` shows all services healthy
- [ ] `curl https://api.YOUR_DOMAIN/health` returns 200 from your laptop
- [ ] Frontend loads at `https://app.YOUR_DOMAIN`
- [ ] Rotate all secrets if you used dev `.env` defaults anywhere
- [ ] Confirm `ufw status` allows only 22/tcp (everything else is tunnel-only)
