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

run "control_plane_runs_the_pushed_digest" {
  command = plan

  assert {
    condition     = kubernetes_deployment_v1.minirail.spec[0].template[0].spec[0].container[0].image == "registry.digitalocean.com/acme/minirail@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    error_message = "control plane must run its image by digest, not by a mutable tag"
  }
}

run "rejects_a_tag_in_place_of_a_digest" {
  command = plan

  variables {
    image_digest = "latest"
  }

  expect_failures = [var.image_digest]
}
