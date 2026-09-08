terraform {
  required_version = ">=1.11.4"

  backend "azurerm" {
    resource_group_name  = "jdev-azure"
    storage_account_name = "jdevazureterraform"
    container_name       = "terraform-state"
    key                  = "tools-terraform.tfstate"
  }

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "=4.57.0"
    }

    random = {
      source  = "hashicorp/random"
      version = "=3.7.2"
    }
  }
}

# Authenticated via `az login` (Azure CLI). No client_id/secret needed.
provider "azurerm" {
  subscription_id = var.subscription_id

  features {
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
  }
}
