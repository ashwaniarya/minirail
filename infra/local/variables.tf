variable "image_digest" {
  description = "Digest of the minirail image pushed by make local-image, read from image.auto.tfvars.json."
  type        = string
}

variable "github_token" {
  description = "Token with read-only contents access; only private repos need it."
  type        = string
  default     = ""
  sensitive   = true
}
