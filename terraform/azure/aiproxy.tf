resource "random_password" "api_client_token" {
  length  = 32
  special = false
}

locals {
  aiproxy_config_hcl = file("${path.module}/files/aiproxy.secret.hcl")
}

resource "azurerm_container_app" "aiproxy" {
  name                         = "${local.prefix}-api"
  container_app_environment_id = azurerm_container_app_environment.this.id
  resource_group_name          = azurerm_resource_group.this.name
  revision_mode                = "Single"

  registry {
    server = "ghcr.io"
  }

  secret {
    name  = "aiproxy-config"
    value = local.aiproxy_config_hcl
  }

  secret {
    name  = "client-token"
    value = random_password.api_client_token.result
  }

  template {
    min_replicas = var.min_replicas
    max_replicas = var.max_replicas

    volume {
      name         = "config"
      storage_type = "Secret"
    }

    container {
      name   = "aiproxy"
      image  = "ghcr.io/egose/aiproxy:${var.image_tag}"
      cpu    = 0.25
      memory = "0.5Gi"

      env {
        name        = "AIPROXY_CLIENT_TOKEN"
        secret_name = "client-token" # pragma: allowlist secret
      }

      volume_mounts {
        name     = "config"
        path     = "/etc/aiproxy/config.hcl"
        sub_path = "aiproxy-config"
      }

      readiness_probe {
        transport = "HTTP"
        port      = 8080
        path      = "/readyz"
      }

      liveness_probe {
        transport = "HTTP"
        port      = 8080
        path      = "/healthz"
      }
    }

    http_scale_rule {
      name                = "http"
      concurrent_requests = var.concurrent_requests
    }
  }

  ingress {
    external_enabled = true
    target_port      = 8080
    transport        = "auto"

    traffic_weight {
      percentage      = 100
      latest_revision = true
    }
  }
}
