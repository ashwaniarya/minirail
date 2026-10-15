output "cluster_name" {
  value = digitalocean_kubernetes_cluster.main.name
}

output "registry_endpoint" {
  value = digitalocean_container_registry.main.endpoint
}

output "docker_config" {
  value     = digitalocean_container_registry_docker_credentials.main.docker_credentials
  sensitive = true
}

output "certificate_name" {
  value = digitalocean_certificate.apps.name
}

output "apps_domain" {
  value = digitalocean_domain.apps.name
}

output "vpc_cidr" {
  value = digitalocean_vpc.main.ip_range
}

output "pg_admin_uri" {
  value     = digitalocean_database_cluster.postgres.private_uri
  sensitive = true
}

output "valkey_enabled" {
  value = var.enable_valkey
}

output "valkey_uri" {
  value     = one(digitalocean_database_cluster.valkey[*].private_uri)
  sensitive = true
}
