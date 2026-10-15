resource "helm_release" "traefik" {
  name             = "traefik"
  repository       = "https://traefik.github.io/charts"
  chart            = "traefik"
  version          = "41.6.1"
  namespace        = "traefik"
  create_namespace = true
  wait             = true
  timeout          = 900

  values = [yamlencode({
    deployment   = { replicas = 2 }
    ingressClass = { enabled = true, isDefaultClass = true, name = "traefik" }
    ports = {
      web = { forwardedHeaders = { trustedIPs = [local.cloud.vpc_cidr] } }
      # TLS ends at the DO load balancer, so 443 carries plain HTTP to the web entrypoint.
      websecure = { targetPort = "web" }
    }
    service = {
      annotations = {
        "service.beta.kubernetes.io/do-loadbalancer-certificate-name"       = local.cloud.certificate_name
        "service.beta.kubernetes.io/do-loadbalancer-tls-ports"              = "443"
        "service.beta.kubernetes.io/do-loadbalancer-protocol"               = "http"
        "service.beta.kubernetes.io/do-loadbalancer-redirect-http-to-https" = "true"
      }
    }
  })]
}

data "kubernetes_service_v1" "traefik" {
  metadata {
    name      = "traefik"
    namespace = helm_release.traefik.namespace
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
