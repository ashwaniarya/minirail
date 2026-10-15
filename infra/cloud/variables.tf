variable "domain" {
  description = "Domain you own. Apps live under apps.<domain>, which must be delegated to ns1/ns2/ns3.digitalocean.com."
  type        = string
}

variable "region" {
  type    = string
  default = "blr1"
}

variable "kubernetes_version_prefix" {
  description = "Empty means the newest DOKS version."
  type        = string
  default     = ""
}

variable "registry_tier" {
  description = "basic holds 5 repositories: minirail plus 4 apps. professional is unlimited."
  type        = string
  default     = "basic"
}

variable "postgres_version" {
  type    = string
  default = "17"
}

variable "enable_valkey" {
  type    = bool
  default = false
}
