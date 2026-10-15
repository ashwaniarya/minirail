output "console_url" {
  value = module.platform.console_url
}

output "admin_password" {
  value     = module.platform.admin_password
  sensitive = true
}
