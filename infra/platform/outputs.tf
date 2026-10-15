output "console_url" {
  value = "https://${local.console_host}"
}

output "admin_password" {
  value     = random_password.admin_password.result
  sensitive = true
}

output "webhook_url" {
  value = "https://${local.console_host}/webhook/github"
}

output "webhook_secret" {
  value     = random_password.webhook_secret.result
  sensitive = true
}

output "load_balancer_ip" {
  value = local.load_balancer_ip
}
