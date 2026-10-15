variable "image_tag" {
  description = "Tag of the minirail image pushed by make image."
  type        = string
}

variable "github_token" {
  description = "Fine-grained token with read-only contents access to the repos you deploy."
  type        = string
  sensitive   = true
}
