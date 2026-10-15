output "console_url" {
  value = module.platform.console_url
}

output "admin_password" {
  value     = module.platform.admin_password
  sensitive = true
}

output "webhook_url" {
  value = module.platform.webhook_url
}

output "webhook_secret" {
  value     = module.platform.webhook_secret
  sensitive = true
}

output "load_balancer_ip" {
  value = local.load_balancer_ip
}
