# OpenAI provider serving every supported modality. Chat-capable models
# sit behind alias/openai_chat; embeddings, images, and audio models are
# called directly (e.g. openai/text-embedding-3-large) because alias
# targets must share at least one capability.
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

logging {
  level      = "info"
  access_log = true
}

provider "openai" "openai" {
  display_name = "OpenAI"
  api_key      = "test-placeholder-key" # pragma: allowlist secret

  model "gpt-4.1" {
    display_name = "GPT-4.1"
    capabilities = ["chat", "responses"]
  }

  model "gpt-4o-mini" {
    display_name = "GPT-4o mini"
    capabilities = ["chat", "responses"]
  }

  model "text-embedding-3-large" {
    display_name = "text-embedding-3-large"
    capabilities = ["embeddings"]
  }

  model "gpt-image-1" {
    display_name = "GPT Image 1"
    capabilities = ["images"]
  }

  model "whisper-1" {
    display_name = "Whisper"
    capabilities = ["audio_transcriptions"]
  }

  model "tts-1" {
    display_name = "TTS 1"
    capabilities = ["audio_speech"]
  }
}

alias "openai_chat" {
  algorithm = "round_robin"

  target {
    provider = "openai"
    model    = "gpt-4.1"
  }

  target {
    provider = "openai"
    model    = "gpt-4o-mini"
  }
}
