terraform {
  required_providers {
    sealedsecret = {
      version = ">=0.1.0"
      source  = "akselleirv/sealedsecret"
    }
    helm         = {
      source  = "hashicorp/helm"
      version = "2.3.0"
    }
  }
}


provider "sealedsecret" {
  controller_name      = "sealed-secret-controller-sealed-secrets"
  controller_namespace = "kube-system"

  kubernetes {
    host                   = var.k8s_host
    client_certificate     = base64decode(var.k8s_client_certificate)
    client_key             = base64decode(var.k8s_client_key)
    cluster_ca_certificate = base64decode(var.k8s_cluster_ca_certificate)
  }

  git {
    url      = var.git_url
    username = var.git_username
    token    = var.git_token

    source_branch = "sealed-secret-update"
    gitlab        = true
  }
}

resource "sealedsecret_in_git" "example" {
  name      = "example-secret"
  namespace = "default"
  data      = {
    "key" : "value"
  }
  filepath  = "sealed-file.yaml"

}


provider "helm" {
  kubernetes {
    host                   = var.k8s_host
    client_certificate     = base64decode(var.k8s_client_certificate)
    client_key             = base64decode(var.k8s_client_key)
    cluster_ca_certificate = base64decode(var.k8s_cluster_ca_certificate)
  }
}

resource "helm_release" "sealed_secret_controller" {
  name         = "sealed-secret-controller"
  repository   = "https://bitnami-labs.github.io/sealed-secrets"
  chart        = "sealed-secrets"

  set {
    name  = "namespace"
    value = "kube-system"
  }
}

