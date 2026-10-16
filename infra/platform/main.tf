module "platform" {
  source = "../modules/platform"

  apps_domain       = local.cloud.apps_domain
  registry_endpoint = local.cloud.registry_endpoint
  docker_config     = local.cloud.docker_config
  image_digest      = var.image_digest
  github_token      = var.github_token
  pg_admin_uri      = local.cloud.pg_admin_uri
  valkey_enabled    = local.cloud.valkey_enabled
  valkey_uri        = local.cloud.valkey_uri
  trusted_ips       = [local.cloud.vpc_cidr]

  traefik_service_annotations = {
    "service.beta.kubernetes.io/do-loadbalancer-certificate-name"       = local.cloud.certificate_name
    "service.beta.kubernetes.io/do-loadbalancer-tls-ports"              = "443"
    "service.beta.kubernetes.io/do-loadbalancer-protocol"               = "http"
    "service.beta.kubernetes.io/do-loadbalancer-redirect-http-to-https" = "true"
  }
}

data "kubernetes_service_v1" "traefik" {
  metadata {
    name      = "traefik"
    namespace = module.platform.traefik_namespace
  }
}

locals {
  load_balancer_ip = data.kubernetes_service_v1.traefik.status[0].load_balancer[0].ingress[0].ip
}

resource "digitalocean_record" "apps" {
  for_each = toset(["*", "@"])
  domain   = local.cloud.apps_domain
  type     = "A"
  name     = each.key
  value    = local.load_balancer_ip
  ttl      = 300
}
