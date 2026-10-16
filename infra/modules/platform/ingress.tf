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
      web = { forwardedHeaders = { trustedIPs = var.trusted_ips } }
      # TLS ends at the load balancer, so 443 carries plain HTTP to the web entrypoint.
      websecure = { targetPort = "web" }
    }
    service = { annotations = var.traefik_service_annotations }
  })]
}
