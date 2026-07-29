## [0.1.0](https://github.com/egose/aiproxy/compare/v0.0.4...v0.1.0) (2026-07-29)

### Features

* **website:** add default config paths and example subcommands ([499af0b](https://github.com/egose/aiproxy/commit/499af0b8749d6afcd1d9c2b57a29daaa1ddb0ae4))

### Documentation

* **website:** document default config locations and CLI examples ([2e3ddec](https://github.com/egose/aiproxy/commit/2e3ddec6d69a411713c6a34fe059e4eee613f4a5))

## [0.0.4](https://github.com/egose/aiproxy/compare/c3b0146ff893f3a43bf76d10fcec56a37fc0389f...v0.0.4) (2026-07-29)

### Features

* add audio capability constants for transcription and speech ([bf3d3bc](https://github.com/egose/aiproxy/commit/bf3d3bc1fcd5e9dd9789e51e8b411cbd7347a8b3))
* add audio speech support for openai providers ([6723d58](https://github.com/egose/aiproxy/commit/6723d58bf6576097bb8d894ebddcfaedb7442da4))
* add billing usage endpoint for aggregated summaries ([e85b0ef](https://github.com/egose/aiproxy/commit/e85b0ef3cac393b160182ebb673d052dcb1a1622))
* add cli entrypoint with serve, validate, and version commands ([88089ba](https://github.com/egose/aiproxy/commit/88089baf500270c43fee17314407310190d83b8f))
* add download, install, and release listing scripts ([f548049](https://github.com/egose/aiproxy/commit/f54804943fb3ac306cbe693216546dbdbc1b3435))
* add explicit audio capability support for OpenAI routes ([c2f8a89](https://github.com/egose/aiproxy/commit/c2f8a89f52cfe690d4574b4a091f6b5d09ea605d))
* add image generation and audio transcription routing ([2840689](https://github.com/egose/aiproxy/commit/28406897a575a21d37488465d84dfe7812af88a7))
* add in-process usage accounting and metrics ([cd40ba7](https://github.com/egose/aiproxy/commit/cd40ba7dfa39a6699c6b43a359922f475474a16f))
* add live config reload and extend metrics coverage ([c3b0146](https://github.com/egose/aiproxy/commit/c3b0146ff893f3a43bf76d10fcec56a37fc0389f))
* add local auth rate limiting and provider health tracking ([480960b](https://github.com/egose/aiproxy/commit/480960b96b1716917cfa29ef6cde10eb927c2fe3))
* add redis-backed provider health sharing ([58371fa](https://github.com/egose/aiproxy/commit/58371fa9e4c2d473dbab6d67f9c527801e6b42e1))
* add request safety, auth filtering, and provider health improvements ([86ba20f](https://github.com/egose/aiproxy/commit/86ba20fd58e4868c2f7c22bdd498857c3e3beaf8))
* add static client tenants and model allow lists ([29fbbcf](https://github.com/egose/aiproxy/commit/29fbbcf87073b4edf4ab7921c214509fecd56146))
* support streaming responses for anthropic and gemini providers ([77e35b3](https://github.com/egose/aiproxy/commit/77e35b33859791b40e770931c001cedf9df18d34))
* **website:** add retention pruning to aggregated usage summaries ([49b1001](https://github.com/egose/aiproxy/commit/49b100176bd846293e673e793e44c84382aec2cd))
* **website:** add reusable upstream execution and improve provider streaming handling ([93a9fc4](https://github.com/egose/aiproxy/commit/93a9fc45a98ad6b3662fc6a177bf8be06a181e02))
* **website:** launch Docusaurus documentation site and publishing workflow ([b75d0f1](https://github.com/egose/aiproxy/commit/b75d0f1e3c7cabedf4eb57d0797bcac7185eb458))
* **website:** refresh website docs homepage and styling ([bae25e8](https://github.com/egose/aiproxy/commit/bae25e8eb881a5111be80654bac53fce8d104c98))

### Bug Fixes

* archive build outputs from target directory ([473d28c](https://github.com/egose/aiproxy/commit/473d28cd87b8f4defb6b02fcf2aa86e7d9b69216))
* disable trivy in publish workflow ([7178a80](https://github.com/egose/aiproxy/commit/7178a80b8b0ae61e5ffe876c2c72a6d8e4d2c171))

### Documentation

* document billing usage API in guides and overview ([db60d78](https://github.com/egose/aiproxy/commit/db60d782772e66899d311a9b0b73cc144485219a))
* document static client tenancy and model restrictions ([681e5e2](https://github.com/egose/aiproxy/commit/681e5e244337b6c95e891f117f8d2418a5d1cf7f))
* update capability lists for audio transcription and speech ([38ac2f4](https://github.com/egose/aiproxy/commit/38ac2f4b5887d4837b49a50e0335961f7b01aaab))
* update docs for audio speech availability ([ff892ca](https://github.com/egose/aiproxy/commit/ff892ca5fc02004cd32a9dec37d29b021a7776ef))
* update health and accounting docs ([18b9414](https://github.com/egose/aiproxy/commit/18b9414ade356c1341ec0a443d68ff23c925f44d))
