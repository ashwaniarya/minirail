data "terraform_remote_state" "cloud" {
  backend = "local"
  config = {
    path = "../cloud/terraform.tfstate"
  }
}

locals {
  cloud = data.terraform_remote_state.cloud.outputs
}

data "digitalocean_kubernetes_cluster" "main" {
  name = local.cloud.cluster_name
}

locals {
  kube_host  = data.digitalocean_kubernetes_cluster.main.endpoint
  kube_token = data.digitalocean_kubernetes_cluster.main.kube_config[0].token
  kube_ca    = base64decode(data.digitalocean_kubernetes_cluster.main.kube_config[0].cluster_ca_certificate)
}

provider "kubernetes" {
  host                   = local.kube_host
  token                  = local.kube_token
  cluster_ca_certificate = local.kube_ca
}

provider "helm" {
  kubernetes = {
    host                   = local.kube_host
    token                  = local.kube_token
    cluster_ca_certificate = local.kube_ca
  }
}
