listener "http" "public" {
  address = "127.0.0.1:8080"
}

auth "main" {
  mode = "bearer_static"
  client "application" {
    token = env("AIPROXY_CLIENT_TOKEN")
  }
}

encrypted_reasoning {
  on_caller_mismatch = "strip_and_retry"
  match_messages    = ["  Caller binding mismatch  "]
}

provider "openai-compatible" "primary" {
  base_url = "http://127.0.0.1:18081/v1"
  api_key  = env("UPSTREAM_API_KEY")

  encrypted_reasoning {
    passthrough = false
  }

  model "chat" {}
  model "recover" {
    encrypted_reasoning {
      on_caller_mismatch = "strip_and_retry"
    }
  }
  model "safe" {
    encrypted_reasoning {}
  }
}

provider "openai-compatible" "secondary" {
  extends = "primary"
  api_key = env("SECONDARY_UPSTREAM_API_KEY")

  encrypted_reasoning {}

  model "chat" {
    encrypted_reasoning {
      on_caller_mismatch = "strip_and_retry"
      match_messages    = []
    }
  }
  model "safe" {
    encrypted_reasoning {
      passthrough = false
    }
  }
}

provider "openai-compatible" "inherited" {
  extends = "primary"
  api_key = env("SECONDARY_UPSTREAM_API_KEY")
}

provider "openai-compatible" "root-only" {
  base_url = "http://127.0.0.1:18082/v1"
  api_key  = env("UPSTREAM_API_KEY")
  model "chat" {}
}

alias "target_policy" {
  algorithm = "round_robin"
  providers = ["primary", "secondary"]
  model     = "chat"
}

alias "safe_pool" {
  algorithm = "round_robin"
  providers = ["primary", "secondary"]
  model     = "recover"
  encrypted_reasoning {}
}

alias "recover_pool" {
  algorithm = "round_robin"
  providers = ["primary", "secondary"]
  model     = "recover"
  encrypted_reasoning {
    on_caller_mismatch = "strip_and_retry"
  }
}
