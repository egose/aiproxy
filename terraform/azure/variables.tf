variable "subscription_id" {
  description = "The Subscription ID which should be used. Auth comes from `az login` (Azure CLI)."
  type        = string
}

variable "location" {
  description = "Azure region for the resource group, Log Analytics, and Container App Environment."
  type        = string
  default     = "West US 2"
}

variable "prefix" {
  description = "Prefix for resource names."
  type        = string
  default     = "aiproxy"
}

variable "image_tag" {
  description = "Tag of ghcr.io/egose/aiproxy to deploy."
  type        = string
  default     = "latest"
}

variable "min_replicas" {
  description = "Minimum replicas. Use 1 to avoid cold starts, 0 for lowest cost."
  type        = number
  default     = 0
}

variable "max_replicas" {
  description = "Maximum replicas."
  type        = number
  default     = 2
}

variable "concurrent_requests" {
  description = "Concurrent HTTP requests per replica before scaling."
  type        = number
  default     = 50
}
