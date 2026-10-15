mock_provider "kubernetes" {}

mock_provider "helm" {}

variables {
  apps_domain       = "apps.example.com"
  registry_endpoint = "registry.digitalocean.com/acme"
  docker_config     = "{}"
  image_digest      = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  github_token      = "token"
  pg_admin_uri      = "postgresql://doadmin:secret@db.example.com:25060/defaultdb"
  trusted_ips       = ["10.10.0.0/16"]
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

run "local_cluster_serves_http_and_pushes_to_a_plain_http_registry" {
  command = plan

  variables {
    scheme            = "http"
    registry_insecure = true
  }

  assert {
    condition     = output.console_url == "http://console.apps.example.com"
    error_message = "console link must follow the scheme"
  }

  assert {
    condition = length(setintersection(
      [for env in kubernetes_deployment_v1.minirail.spec[0].template[0].spec[0].container[0].env : "${env.name}=${env.value}"],
      ["MINIRAIL_SCHEME=http", "MINIRAIL_REGISTRY_INSECURE=true"],
    )) == 2
    error_message = "control plane must get the scheme and the insecure registry flag"
  }
}
