locals {
  apps_domain = "apps.${var.domain}"
  name        = replace(local.apps_domain, ".", "-")
}

resource "digitalocean_vpc" "main" {
  name   = local.name
  region = var.region
}

data "digitalocean_kubernetes_versions" "current" {
  version_prefix = var.kubernetes_version_prefix
}

resource "digitalocean_kubernetes_cluster" "main" {
  name                             = local.name
  region                           = var.region
  version                          = data.digitalocean_kubernetes_versions.current.latest_version
  vpc_uuid                         = digitalocean_vpc.main.id
  destroy_all_associated_resources = true

  node_pool {
    name       = "default"
    size       = "s-2vcpu-4gb"
    auto_scale = true
    min_nodes  = 2
    max_nodes  = 4
  }

  lifecycle {
    # DOKS auto-upgrades patch versions; without this every new release would plan a cluster replacement.
    ignore_changes = [version]
  }
}

resource "digitalocean_container_registry" "main" {
  name                   = local.name
  subscription_tier_slug = var.registry_tier
  region                 = var.region
}

resource "digitalocean_container_registry_docker_credentials" "main" {
  registry_name = digitalocean_container_registry.main.name
  write         = true
}

resource "digitalocean_domain" "apps" {
  name = local.apps_domain
}

resource "digitalocean_certificate" "apps" {
  name       = local.name
  type       = "lets_encrypt"
  domains    = ["*.${local.apps_domain}", local.apps_domain]
  depends_on = [digitalocean_domain.apps]

  lifecycle {
    create_before_destroy = true
  }
}

resource "digitalocean_database_cluster" "postgres" {
  name                 = "${local.name}-pg"
  engine               = "pg"
  version              = var.postgres_version
  size                 = "db-s-1vcpu-1gb"
  region               = var.region
  node_count           = 1
  private_network_uuid = digitalocean_vpc.main.id
}

resource "digitalocean_database_firewall" "postgres" {
  cluster_id = digitalocean_database_cluster.postgres.id

  rule {
    type  = "k8s"
    value = digitalocean_kubernetes_cluster.main.id
  }
}

resource "digitalocean_database_cluster" "valkey" {
  count                = var.enable_valkey ? 1 : 0
  name                 = "${local.name}-valkey"
  engine               = "valkey"
  version              = "8"
  size                 = "db-s-1vcpu-1gb"
  region               = var.region
  node_count           = 1
  private_network_uuid = digitalocean_vpc.main.id
}

resource "digitalocean_database_firewall" "valkey" {
  count      = var.enable_valkey ? 1 : 0
  cluster_id = digitalocean_database_cluster.valkey[0].id

  rule {
    type  = "k8s"
    value = digitalocean_kubernetes_cluster.main.id
  }
}
