mock_provider "digitalocean" {
  override_data {
    target = data.digitalocean_kubernetes_cluster.main
    values = {
      endpoint    = "https://cluster.example.com"
      kube_config = [{ token = "token", cluster_ca_certificate = "Y2E=" }]
    }
  }
}

mock_provider "kubernetes" {
  override_data {
    target = data.kubernetes_service_v1.traefik
    values = {
      status = [{ load_balancer = [{ ingress = [{ ip = "203.0.113.10" }] }] }]
    }
  }
}

mock_provider "helm" {}

override_data {
  target = data.terraform_remote_state.cloud
  values = {
    outputs = {
      apps_domain       = "apps.example.com"
      certificate_name  = "apps-cert"
      cluster_name      = "minirail"
      docker_config     = "{}"
      pg_admin_uri      = "postgresql://doadmin:secret@db.example.com:25060/defaultdb"
      registry_endpoint = "registry.digitalocean.com/acme"
      valkey_enabled    = false
      valkey_uri        = null
      vpc_cidr          = "10.10.0.0/16"
    }
  }
}

variables {
  github_token = "token"
  image_digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}

run "apps_domain_points_at_the_load_balancer" {
  command = plan

  assert {
    condition     = digitalocean_record.apps["*"].value == "203.0.113.10" && digitalocean_record.apps["@"].value == "203.0.113.10"
    error_message = "wildcard and apex records must point at the Traefik load balancer"
  }

  assert {
    condition     = output.console_url == "https://console.apps.example.com"
    error_message = "console must be served over https on DigitalOcean"
  }
}
