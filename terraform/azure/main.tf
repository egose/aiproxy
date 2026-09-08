locals {
  prefix            = var.prefix
  resource_location = var.location
}

resource "azurerm_resource_group" "this" {
  name     = "${local.prefix}-terraform"
  location = local.resource_location
}

resource "azurerm_log_analytics_workspace" "this" {
  name                = "${local.prefix}-default"
  location            = azurerm_resource_group.this.location
  resource_group_name = azurerm_resource_group.this.name
  sku                 = "PerGB2018"
  retention_in_days   = 30
}

resource "azurerm_container_app_environment" "this" {
  name                       = "${local.prefix}-default"
  location                   = azurerm_resource_group.this.location
  resource_group_name        = azurerm_resource_group.this.name
  log_analytics_workspace_id = azurerm_log_analytics_workspace.this.id
}
