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
