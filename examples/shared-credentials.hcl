# Repeated credentials via extends: nvidia-2 inherits type, endpoint,
# timeout, enabled state, and models from nvidia-1 and adds its own
# credential. Aliases still list each provider target explicitly.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai-compatible" "nvidia-1" {
  display_name = "Nvidia - primary"
  base_url     = "https://integrate.api.nvidia.com/v1"
  api_key      = "test-placeholder-key-1" # pragma: allowlist secret

  model "z-ai/glm-5.2" {
    display_name = "GLM 5.2"
    capabilities = ["chat", "responses"]
  }
}

provider "openai-compatible" "nvidia-2" {
  extends      = "nvidia-1"
  display_name = "Nvidia - secondary"
  api_key      = "test-placeholder-key-2" # pragma: allowlist secret
}

alias "nvidia_chat" {
  algorithm = "round_robin"

  target {
    provider = "nvidia-1"
    model    = "z-ai/glm-5.2"
  }

  target {
    provider = "nvidia-2"
    model    = "z-ai/glm-5.2"
  }
}
