locals {
  postgres_host = "${kubernetes_service_v1.postgres.metadata[0].name}.${kubernetes_service_v1.postgres.metadata[0].namespace}.svc.cluster.local"
}

module "platform" {
  source = "../modules/platform"

  apps_domain = "localhost"
  scheme      = "http"
  # The registry container listens on 5000; 5050 is only its host port. k3s mirrors both names for pulls.
  registry_endpoint = "k3d-minirail-registry:5000"
  registry_insecure = true
  docker_config     = jsonencode({ auths = {} })
  image_digest      = var.image_digest
  github_token      = var.github_token
  pg_admin_uri      = "postgres://postgres:${random_password.postgres.result}@${local.postgres_host}:5432/postgres?sslmode=disable"
  trusted_ips       = []
}
