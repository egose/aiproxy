listener "http" "public" {
  address = ":8080"
}

upstream_header_timeout = "360s"

auth "main" {
  mode = "bearer_static"

  client "internal-app" {
    token = env("AIPROXY_CLIENT_TOKEN")
  }
}

logging {
  level      = "info"
  access_log = true
}
