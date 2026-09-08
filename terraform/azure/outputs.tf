output "fqdn" {
  description = "Public FQDN of the aiproxy Container App."
  value       = azurerm_container_app.aiproxy.ingress[0].fqdn
}

output "client_token" {
  description = "Bearer token for the internal-app client (AIPROXY_CLIENT_TOKEN)."
  value       = random_password.api_client_token.result
  sensitive   = true
}
