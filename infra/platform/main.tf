locals {
  console_host  = "console.${local.cloud.apps_domain}"
  control_image = "${local.cloud.registry_endpoint}/minirail@${var.image_digest}"
}

resource "kubernetes_namespace_v1" "main" {
  for_each = toset(["minirail", "apps"])

  metadata {
    name = each.key
  }
}

resource "kubernetes_secret_v1" "registry" {
  for_each = kubernetes_namespace_v1.main
  type     = "kubernetes.io/dockerconfigjson"

  metadata {
    name      = "registry"
    namespace = each.value.metadata[0].name
  }

  data = {
    ".dockerconfigjson" = local.cloud.docker_config
  }
}

resource "kubernetes_secret_v1" "github_token" {
  metadata {
    name      = "github-token"
    namespace = kubernetes_namespace_v1.main["apps"].metadata[0].name
  }

  data = {
    token = var.github_token
  }
}

resource "kubernetes_secret_v1" "valkey" {
  count = local.cloud.valkey_enabled ? 1 : 0

  metadata {
    name      = "valkey"
    namespace = kubernetes_namespace_v1.main["apps"].metadata[0].name
  }

  data = {
    REDIS_URL = local.cloud.valkey_uri
  }
}

resource "random_password" "webhook_secret" {
  length  = 40
  special = false
}

resource "random_password" "admin_password" {
  length  = 32
  special = false
}

resource "kubernetes_secret_v1" "minirail" {
  metadata {
    name      = "minirail"
    namespace = kubernetes_namespace_v1.main["minirail"].metadata[0].name
  }

  data = {
    GITHUB_TOKEN   = var.github_token
    WEBHOOK_SECRET = random_password.webhook_secret.result
    ADMIN_PASSWORD = random_password.admin_password.result
    PG_ADMIN_URL   = local.cloud.pg_admin_uri
  }
}

resource "kubernetes_service_account_v1" "minirail" {
  metadata {
    name      = "minirail"
    namespace = kubernetes_namespace_v1.main["minirail"].metadata[0].name
  }
}

resource "kubernetes_role_v1" "minirail" {
  metadata {
    name      = "minirail"
    namespace = kubernetes_namespace_v1.main["apps"].metadata[0].name
  }

  rule {
    api_groups = [""]
    resources  = ["configmaps", "secrets", "services", "pods", "pods/log"]
    verbs      = ["*"]
  }

  rule {
    api_groups = ["apps"]
    resources  = ["deployments"]
    verbs      = ["*"]
  }

  rule {
    api_groups = ["networking.k8s.io"]
    resources  = ["ingresses"]
    verbs      = ["*"]
  }

  rule {
    api_groups = ["batch"]
    resources  = ["jobs", "cronjobs"]
    verbs      = ["*"]
  }
}

resource "kubernetes_role_binding_v1" "minirail" {
  metadata {
    name      = "minirail"
    namespace = kubernetes_role_v1.minirail.metadata[0].namespace
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = kubernetes_role_v1.minirail.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account_v1.minirail.metadata[0].name
    namespace = kubernetes_service_account_v1.minirail.metadata[0].namespace
  }
}

resource "kubernetes_deployment_v1" "minirail" {
  metadata {
    name      = "minirail"
    namespace = kubernetes_namespace_v1.main["minirail"].metadata[0].name
  }

  spec {
    replicas = 1

    selector {
      match_labels = { app = "minirail" }
    }

    template {
      metadata {
        labels = { app = "minirail" }
      }

      spec {
        service_account_name = kubernetes_service_account_v1.minirail.metadata[0].name

        image_pull_secrets {
          name = kubernetes_secret_v1.registry["minirail"].metadata[0].name
        }

        container {
          name  = "minirail"
          image = local.control_image

          port {
            container_port = 8080
          }

          env {
            name  = "MINIRAIL_DOMAIN"
            value = local.cloud.apps_domain
          }

          env {
            name  = "MINIRAIL_REGISTRY"
            value = local.cloud.registry_endpoint
          }

          env {
            name  = "MINIRAIL_CONTROL_IMAGE"
            value = local.control_image
          }

          env {
            name  = "MINIRAIL_VALKEY"
            value = tostring(local.cloud.valkey_enabled)
          }

          env_from {
            secret_ref {
              name = kubernetes_secret_v1.minirail.metadata[0].name
            }
          }

          readiness_probe {
            http_get {
              path = "/healthz"
              port = 8080
            }
          }

          resources {
            requests = { cpu = "50m", memory = "64Mi" }
            limits   = { memory = "256Mi" }
          }
        }
      }
    }
  }
}

resource "kubernetes_service_v1" "minirail" {
  metadata {
    name      = "minirail"
    namespace = kubernetes_deployment_v1.minirail.metadata[0].namespace
  }

  spec {
    selector = { app = "minirail" }

    port {
      port        = 80
      target_port = 8080
    }
  }
}

resource "kubernetes_ingress_v1" "console" {
  metadata {
    name      = "console"
    namespace = kubernetes_service_v1.minirail.metadata[0].namespace
  }

  spec {
    ingress_class_name = "traefik"

    rule {
      host = local.console_host

      http {
        path {
          path      = "/"
          path_type = "Prefix"

          backend {
            service {
              name = kubernetes_service_v1.minirail.metadata[0].name
              port {
                number = 80
              }
            }
          }
        }
      }
    }
  }

  depends_on = [helm_release.traefik]
}
