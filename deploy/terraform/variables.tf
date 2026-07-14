# ─── Input Variables ──────────────────────────────────────

variable "neon_api_key" {
  description = "Neon API key (https://console.neon.tech/app/settings/api-keys)"
  type        = string
  sensitive   = true
}
