locals {
  kubeconfig_path = "${path.root}/kubeconfig.yaml"
}

provider "kubernetes" {
  config_path = local.kubeconfig_path
}

provider "helm" {
  kubernetes = {
    config_path = local.kubeconfig_path
  }
}
