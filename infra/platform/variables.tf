variable "image_digest" {
  description = "Digest of the minirail image pushed by make image, read from image.auto.tfvars.json."
  type        = string
}

variable "github_token" {
  description = "Fine-grained token with read-only contents access to the repos you deploy."
  type        = string
  sensitive   = true
}
