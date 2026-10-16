variable "apps_domain" {
  description = "Apps live at <app>.<apps_domain>, the dashboard at console.<apps_domain>."
  type        = string
}

variable "registry_endpoint" {
  description = "Registry the control image and app images live in, without a trailing slash."
  type        = string
}

variable "docker_config" {
  description = "Docker config JSON with pull and push credentials for the registry."
  type        = string
  sensitive   = true
}

variable "image_digest" {
  description = "Digest of the minirail image pushed by make image, read from image.auto.tfvars.json."
  type        = string

  validation {
    condition     = can(regex("^sha256:[0-9a-f]{64}$", var.image_digest))
    error_message = "image_digest must be a sha256 digest; run make image first."
  }
}

variable "github_token" {
  description = "Fine-grained token with read-only contents access to the repos you deploy."
  type        = string
  sensitive   = true
}

variable "pg_admin_uri" {
  description = "Postgres admin URL the control plane creates app roles and databases with."
  type        = string
  sensitive   = true
}

variable "valkey_enabled" {
  description = "Offer the shared Valkey to apps."
  type        = bool
  default     = false
}

variable "valkey_uri" {
  description = "Valkey URL handed to apps as REDIS_URL when valkey_enabled is true."
  type        = string
  sensitive   = true
  default     = null
}

variable "trusted_ips" {
  description = "Proxies Traefik trusts for X-Forwarded-* headers."
  type        = list(string)
}

variable "traefik_service_annotations" {
  description = "Annotations on the Traefik LoadBalancer Service, such as the cloud load balancer settings."
  type        = map(string)
  default     = {}
}
