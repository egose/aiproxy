"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[873],{

/***/ 2420
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_configuration_md_9ed_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-configuration-md-9ed.json
const site_docs_configuration_md_9ed_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"configuration","title":"Configuration","description":"aiproxy uses labeled HCL blocks. The core building blocks are:","source":"@site/docs/configuration.md","sourceDirName":".","slug":"/configuration","permalink":"/docs/configuration","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":3,"frontMatter":{"sidebar_position":3},"sidebar":"docsSidebar","previous":{"title":"Quickstart","permalink":"/docs/quickstart"},"next":{"title":"Config Examples","permalink":"/docs/config-examples"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/configuration.md


const frontMatter = {
	sidebar_position: 3
};
const contentTitle = 'Configuration';

const assets = {

};



const toc = [{
  "value": "Mental Model",
  "id": "mental-model",
  "level": 2
}, {
  "value": "Example",
  "id": "example",
  "level": 2
}, {
  "value": "Listener",
  "id": "listener",
  "level": 2
}, {
  "value": "Logging",
  "id": "logging",
  "level": 2
}, {
  "value": "Auth",
  "id": "auth",
  "level": 2
}, {
  "value": "Providers",
  "id": "providers",
  "level": 2
}, {
  "value": "OpenCode Zen And Go",
  "id": "opencode-zen-and-go",
  "level": 3
}, {
  "value": "Provider Inheritance",
  "id": "provider-inheritance",
  "level": 3
}, {
  "value": "Upstream Header Timeout",
  "id": "upstream-header-timeout",
  "level": 2
}, {
  "value": "Models",
  "id": "models",
  "level": 2
}, {
  "value": "Secrets And Environment Variables",
  "id": "secrets-and-environment-variables",
  "level": 2
}, {
  "value": "<code>api_key_ref</code>",
  "id": "api_key_ref",
  "level": 2
}, {
  "value": "Naming Rules",
  "id": "naming-rules",
  "level": 2
}, {
  "value": "Validation Rules",
  "id": "validation-rules",
  "level": 2
}, {
  "value": "Optional Blocks",
  "id": "optional-blocks",
  "level": 2
}, {
  "value": "<code>metrics</code>",
  "id": "metrics",
  "level": 3
}, {
  "value": "<code>dashboard</code>",
  "id": "dashboard",
  "level": 3
}];
function _createMdxContent(props) {
  const _components = {
    a: "a",
    code: "code",
    h1: "h1",
    h2: "h2",
    h3: "h3",
    header: "header",
    li: "li",
    ol: "ol",
    p: "p",
    pre: "pre",
    ul: "ul",
    ...(0,lib/* useMDXComponents */.R)(),
    ...props.components
  };
  return (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
    children: [(0,jsx_runtime.jsx)(_components.header, {
      children: (0,jsx_runtime.jsx)(_components.h1, {
        id: "configuration",
        children: "Configuration"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " uses labeled HCL blocks. The core building blocks are:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "listener \"http\" \"public\""
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "auth \"main\""
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "logging"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "provider \"<type>\" \"<name>\""
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "alias \"<name>\""
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "mental-model",
      children: "Mental Model"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Think about the config in five layers:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ol, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "listener"
        }), " defines how the proxy accepts traffic."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "auth"
        }), " defines who may call it."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "logging"
        }), " defines structured log verbosity and request lifecycle access logging."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "provider"
        }), " blocks define upstream systems and their models."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "alias"
        }), " blocks define the client-facing virtual models used for routing and failover."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "example",
      children: "Example"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "listener \"http\" \"public\" {\n  address = \":8080\"\n\n  timeouts {\n    read_header = \"10s\"\n    idle        = \"60s\"\n    write       = \"0s\"\n  }\n}\n\nupstream_header_timeout = \"120s\"\n\nauth \"main\" {\n  mode = \"bearer_static\"\n\n  rate_limit {\n    requests_per_minute = 120\n    burst               = 120\n  }\n\n  client \"internal-app\" {\n    token          = env(\"AIPROXY_CLIENT_TOKEN\")\n    tenant         = \"internal\"\n    allowed_models = [\"alias/chat_default\", \"openai/gpt-4.1\"]\n  }\n}\n\nlogging {\n  level      = \"info\"\n  access_log = true\n}\n\nprovider \"openai\" \"openai\" {\n  display_name = \"OpenAI\"\n  api_key      = env(\"OPENAI_API_KEY\")\n\n  model \"gpt-4.1\" {\n    display_name = \"GPT-4.1\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n\n  model \"text-embedding-3-large\" {\n    display_name = \"text-embedding-3-large\"\n    capabilities = [\"embeddings\"]\n  }\n}\n\nprovider \"openai-compatible\" \"localai\" {\n  display_name = \"LocalAI\"\n  base_url     = \"https://llm.internal/v1\"\n  upstream_header_timeout = \"180s\"\n\n  api_key_ref {\n    key = \"localai\"\n  }\n\n  model \"qwen3-32b\" {\n    display_name = \"Qwen 3 32B\"\n  }\n}\n\nalias \"chat_default\" {\n  algorithm = \"round_robin\"\n\n  target {\n    provider = \"openai\"\n    model    = \"gpt-4.1\"\n  }\n\n  target {\n    provider = \"localai\"\n    model    = \"qwen3-32b\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "listener",
      children: "Listener"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The listener block configures the inbound HTTP server."
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "address"
        }), " sets the listen address such as ", (0,jsx_runtime.jsx)(_components.code, {
          children: ":8080"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "timeouts"
        }), " configures read, idle, and write timeouts"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Listener address and timeout changes still require a restart, even though runtime state such as auth, providers, models, aliases, upstream header timeouts, access-log enablement, metrics, and provider-health config can reload on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), ". Logging level changes and enabling the dashboard after startup also require a restart."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "For most deployments, one HTTP listener is enough."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "logging",
      children: "Logging"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "logging"
      }), " block controls structured application logs."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "level"
        }), " accepts ", (0,jsx_runtime.jsx)(_components.code, {
          children: "debug"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "info"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "warn"
        }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "error"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "access_log"
        }), " enables or disables request lifecycle logs such as request received, upstream request start and finish, and response sent or stream start and finish"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Defaults:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "level = \"info\""
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "access_log = true"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "auth",
      children: "Auth"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Supported inbound auth modes:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "none"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "bearer_static"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "none"
      }), " is only appropriate for trusted environments."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "bearer_static"
      }), " validates client bearer tokens against statically configured ", (0,jsx_runtime.jsx)(_components.code, {
        children: "client"
      }), " blocks. Each client may also define:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "tenant"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "allowed_models"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "rate_limit"
      }), " block is local and in-memory:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["In ", (0,jsx_runtime.jsx)(_components.code, {
          children: "bearer_static"
        }), " mode, it is keyed per authenticated client"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["In ", (0,jsx_runtime.jsx)(_components.code, {
          children: "none"
        }), " mode, it applies to a shared anonymous bucket"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), " when you want a static allow-list at the proxy boundary rather than relying only on application-level policy."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "providers",
      children: "Providers"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Providers always use two labels:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"<type>\" \"<name>\" {}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Common attributes:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "display_name"
        })
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "base_url"
        }), " for ", (0,jsx_runtime.jsx)(_components.code, {
          children: "openai-compatible"
        }), " (required), and as an optional transport\noverride for ", (0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-zen"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-go"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "extends"
        }), " for restricted provider inheritance"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key_ref"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "upstream_header_timeout"
        })
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "enabled"
        }), " (optional, default ", (0,jsx_runtime.jsx)(_components.code, {
          children: "true"
        }), ")"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["nested ", (0,jsx_runtime.jsx)(_components.code, {
          children: "model"
        }), " blocks (OpenCode models additionally require ", (0,jsx_runtime.jsx)(_components.code, {
          children: "protocol"
        }), ")"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Providers normally declare exactly one of ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ". Enabled\nproviders with unresolved, empty, or missing credentials fail validation, with\none exception: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " providers may omit the credential entirely for\nkeyless upstream access, in which case the proxy sends no ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization"
      }), "\nheader. To\nintentionally disable a provider, declare ", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled = false"
      }), "; disabled\nproviders are still validated for structure, URL, models, and capabilities,\nbut they do not require a usable credential."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"openai\" \"backup\" {\n  enabled = false\n  model \"gpt-4o-mini\" {}\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), " values must be absolute ", (0,jsx_runtime.jsx)(_components.code, {
        children: "https"
      }), " URLs for remote upstreams.\nPlain ", (0,jsx_runtime.jsx)(_components.code, {
        children: "http"
      }), " is accepted only for loopback development endpoints such as\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "localhost"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "127.0.0.1"
      }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "::1"
      }), ". ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai-compatible"
      }), " requires ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), ";\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-go"
      }), " default to their service prefixes\n(", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://opencode.ai/zen/v1"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://opencode.ai/zen/go/v1"
      }), ") and accept\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), " only as a transport override for tests and custom gateways. An\noverride never changes service selection, auth, or header behavior."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Provider names are part of the public model string, so keep them stable and machine-friendly."
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "opencode-zen-and-go",
      children: "OpenCode Zen And Go"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-go"
      }), " share one adapter behind two explicit types;\nthe type selects the service, never the URL or credential. Every model\ndeclares a required ", (0,jsx_runtime.jsx)(_components.code, {
        children: "protocol"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "messages"
      }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), ";\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " is Zen-only):"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"opencode-zen\" \"zen\" {\n  api_key = env(\"OPENCODE_ZEN_API_KEY\")\n\n  model \"glm-5.3\" {\n    protocol     = \"chat\"\n    capabilities = [\"chat\"]\n  }\n\n  model \"claude-sonnet-5\" {\n    protocol = \"messages\"\n  }\n}\n\nprovider \"opencode-go\" \"go\" {\n  api_key = env(\"OPENCODE_GO_API_KEY\")\n\n  model \"minimax-m3\" {\n    protocol = \"messages\"\n  }\n\n  model \"glm-5.3\" {\n    protocol = \"chat\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Public model names are ", (0,jsx_runtime.jsx)(_components.code, {
        children: "zen/glm-5.3"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "go/minimax-m3"
      }), ". ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), " and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), " protocols are native pass-through serving one public operation\neach; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "messages"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " serve ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), " through the\nexisting conservative translation subsets. Anything else, including\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "embeddings"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "images"
      }), ", and audio on both OpenCode types, is rejected before\nupstream I/O. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-go"
      }), " send ", (0,jsx_runtime.jsx)(_components.code, {
        children: "x-opencode-session"
      }), " on every upstream\nrequest (caller values are forwarded only when valid, otherwise a fresh\nper-request ID is generated); direct requests never cross services, and only\nexplicitly configured aliases retry another target. See\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/providers-and-routing",
        children: "Providers and Routing"
      }), " for the full contract and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "examples/opencode-zen.hcl"
      }), " / ", (0,jsx_runtime.jsx)(_components.code, {
        children: "examples/opencode-go.hcl"
      }), " for complete\nvalidated configs."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "provider-inheritance",
      children: "Provider Inheritance"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "extends"
      }), " when several accounts share the same provider type, endpoint, timeout, and model inventory but need separate credentials:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"openai-compatible\" \"nvidia-1\" {\n  display_name = \"Nvidia - j.dev\"\n  base_url     = \"https://integrate.api.nvidia.com/v1\"\n\n  api_key_ref {\n    key = \"nvidia-1\"\n  }\n\n  model \"z-ai/glm-5.2\" {\n    display_name = \"GLM 5.2\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n}\n\nprovider \"openai-compatible\" \"nvidia-2\" {\n  extends      = \"nvidia-1\"\n  display_name = \"Nvidia - corean\"\n\n  api_key_ref {\n    key = \"nvidia-2\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["A derived provider may be declared before or after its base. It may declare only ", (0,jsx_runtime.jsx)(_components.code, {
        children: "extends"
      }), ", optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "display_name"
      }), ", and exactly one local credential, either ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ". It inherits the base provider type, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), ", effective upstream header timeout, enabled state, and all model blocks."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The type label remains required and must match the base. The base must exist, be enabled, and must not itself use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "extends"
      }), "; inheritance chains are rejected. Local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_header_timeout"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " declarations are rejected instead of ignored."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Derived providers are flattened during config loading and reload. After a successful load, direct model strings, health, metrics, billing, and dashboard inventory use the derived provider's own name. Aliases still list each provider target explicitly."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "upstream-header-timeout",
      children: "Upstream Header Timeout"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_header_timeout"
      }), " controls how long the proxy waits for upstream response headers. It accepts Go duration strings such as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "30s"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "2m"
      }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "1h"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "You can set it globally at the root or override it per provider:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "upstream_header_timeout = \"120s\"\n\nprovider \"openai\" \"openai\" {\n  upstream_header_timeout = \"180s\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Precedence is provider value, then root value, then the 90-second default. The timeout applies only until response headers arrive; JSON and streaming response bodies can continue for any duration after headers are received. Root and provider timeout changes apply on a successful ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " reload."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "models",
      children: "Models"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Each provider contains one or more ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " blocks:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "model \"gpt-4.1\" {\n  display_name = \"GPT-4.1\"\n  upstream_name = \"gpt-4.1\"\n  capabilities  = [\"chat\", \"responses\"]\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "The block label is the proxy-visible model name"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "display_name"
        }), " is optional metadata"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "upstream_name"
        }), " lets the upstream identifier differ from the public name"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "protocol"
        }), " is required on ", (0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-zen"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-go"
        }), " models (", (0,jsx_runtime.jsx)(_components.code, {
          children: "chat"
        }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "responses"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "messages"
        }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "gemini"
        }), "; ", (0,jsx_runtime.jsx)(_components.code, {
          children: "gemini"
        }), " is Zen-only) and rejected on\nother provider types"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "capabilities"
        }), " narrows the operations exposed through the proxy"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_name"
      }), " when you want a cleaner or more stable public model name than the exact upstream identifier."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Supported capability values:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "chat"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "responses"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "embeddings"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "images"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "audio_transcriptions"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "audio_speech"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Omitted ", (0,jsx_runtime.jsx)(_components.code, {
        children: "capabilities"
      }), " default to the provider-type defaults, except on\nOpenCode providers where the default is protocol-aware (", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), " serves ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), " serves ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "messages"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " serve both)."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "secrets-and-environment-variables",
      children: "Secrets And Environment Variables"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env(\"VAR\")"
      }), " anywhere a string is allowed. Values are inlined before HCL parsing."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "That makes it suitable for API keys, bearer tokens, URLs, and other deployment-specific values."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For local runs, if your config depends on variables in ", (0,jsx_runtime.jsx)(_components.code, {
        children: ".env"
      }), ", load them first:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "set -a; . ./.env; set +a\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "api_key_ref",
      children: (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When a provider uses ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " reads the secret from a JSON file:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-json",
        children: "{\n  \"openai\": \"sk-...\",\n  \"localai\": \"secret\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The default path is:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "$XDG_CONFIG_HOME/aiproxy/keys.json"
        })
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "~/.config/aiproxy/keys.json"
        }), " when ", (0,jsx_runtime.jsx)(_components.code, {
          children: "XDG_CONFIG_HOME"
        }), " is unset"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "You can override the file path per provider:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "api_key_ref {\n  path = \"/etc/aiproxy/keys.json\"\n  key  = \"localai\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), " when you want provider secrets stored outside the main HCL file."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "naming-rules",
      children: "Naming Rules"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " keeps public names intentionally strict:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "provider names are lowercase"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "alias names are lowercase"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "names must not contain spaces"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["provider and alias names must not contain ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["provider name ", (0,jsx_runtime.jsx)(_components.code, {
          children: "alias"
        }), " is reserved for ", (0,jsx_runtime.jsx)(_components.code, {
          children: "alias/<alias-name>"
        }), " routing"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["model names may contain ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/"
        }), " when every slash-separated segment follows the\nsame lowercase name rule"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["These rules keep model parsing simple and unambiguous; direct model resolution\nsplits on the first ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/"
      }), ", so ", (0,jsx_runtime.jsx)(_components.code, {
        children: "<provider-name>/<model-name>"
      }), " still works when the\nmodel name contains additional slashes."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "validation-rules",
      children: "Validation Rules"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Startup fails on invalid configuration. Important checks include:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "duplicate provider or alias names"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "invalid provider types or alias algorithms"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["provider or alias names that are not lowercase or contain spaces or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["provider name ", (0,jsx_runtime.jsx)(_components.code, {
          children: "alias"
        })]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "model names with invalid slash-separated segments"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "openai-compatible"
        }), " providers missing ", (0,jsx_runtime.jsx)(_components.code, {
          children: "base_url"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["malformed provider ", (0,jsx_runtime.jsx)(_components.code, {
          children: "base_url"
        }), " values, and non-loopback ", (0,jsx_runtime.jsx)(_components.code, {
          children: "http"
        }), " base URLs"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-zen"
        }), " or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-go"
        }), " models missing ", (0,jsx_runtime.jsx)(_components.code, {
          children: "protocol"
        }), ", using an unknown\nprotocol, using ", (0,jsx_runtime.jsx)(_components.code, {
          children: "gemini"
        }), " on ", (0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-go"
        }), ", or declaring a capability the\nprotocol does not serve; ", (0,jsx_runtime.jsx)(_components.code, {
          children: "protocol"
        }), " on any other provider type"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["providers with both ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key_ref"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["enabled providers with no resolved credential, including an empty\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key = env(\"...\")"
        }), "; missing or empty credentials fail validation unless\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "enabled = false"
        }), " is declared explicitly or the provider type is\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-zen"
        })]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "providers without any models"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "aliases without any targets"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "alias targets that reference unknown providers or models"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["a ", (0,jsx_runtime.jsx)(_components.code, {
          children: "metrics"
        }), " block with an empty or missing token"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["listener addresses that are URLs instead of TCP ", (0,jsx_runtime.jsx)(_components.code, {
          children: "host:port"
        }), " bind addresses"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["a ", (0,jsx_runtime.jsx)(_components.code, {
          children: "dashboard"
        }), " block with ", (0,jsx_runtime.jsx)(_components.code, {
          children: "allow_insecure_remote = true"
        }), "; the dashboard command\nis local-only"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "optional-blocks",
      children: "Optional Blocks"
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "metrics",
      children: (0,jsx_runtime.jsx)(_components.code, {
        children: "metrics"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "metrics {\n  token = env(\"AIPROXY_METRICS_TOKEN\")\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When present, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "GET /metrics"
      }), " requires ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization: Bearer <token>"
      }), " with the\nconfigured value. The metrics token is checked independently of API auth\nclient tokens; API clients cannot scrape ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/metrics"
      }), " with their own credentials."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "dashboard",
      children: (0,jsx_runtime.jsx)(_components.code, {
        children: "dashboard"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "dashboard {\n  token = env(\"AIPROXY_DASHBOARD_TOKEN\")\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When ", (0,jsx_runtime.jsx)(_components.code, {
        children: "token"
      }), " is omitted, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy serve"
      }), " mints a random secret at startup and\npersists it to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "$XDG_CONFIG_HOME/aiproxy/dashboard.token"
      }), "; the ", (0,jsx_runtime.jsx)(_components.code, {
        children: "dashboard"
      }), "\ncommand reads that file to authenticate. The dashboard command is local-only: it\nconnects over loopback plain HTTP and refuses concrete non-loopback listener\nhosts. HTTPS and remote dashboard URLs are not supported by the current\nconfiguration model."]
    })]
  });
}
function MDXContent(props = {}) {
  const {wrapper: MDXLayout} = {
    ...(0,lib/* useMDXComponents */.R)(),
    ...props.components
  };
  return MDXLayout ? (0,jsx_runtime.jsx)(MDXLayout, {
    ...props,
    children: (0,jsx_runtime.jsx)(_createMdxContent, {
      ...props
    })
  }) : _createMdxContent(props);
}



/***/ },

/***/ 1982
(__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   R: () => (/* binding */ useMDXComponents),
/* harmony export */   x: () => (/* binding */ MDXProvider)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(489);
/**
 * @import {MDXComponents} from 'mdx/types.js'
 * @import {Component, ReactElement, ReactNode} from 'react'
 */

/**
 * @callback MergeComponents
 *   Custom merge function.
 * @param {Readonly<MDXComponents>} currentComponents
 *   Current components from the context.
 * @returns {MDXComponents}
 *   Additional components.
 *
 * @typedef Props
 *   Configuration for `MDXProvider`.
 * @property {ReactNode | null | undefined} [children]
 *   Children (optional).
 * @property {Readonly<MDXComponents> | MergeComponents | null | undefined} [components]
 *   Additional components to use or a function that creates them (optional).
 * @property {boolean | null | undefined} [disableParentContext=false]
 *   Turn off outer component context (default: `false`).
 */



/** @type {Readonly<MDXComponents>} */
const emptyComponents = {}

const MDXContext = react__WEBPACK_IMPORTED_MODULE_0__.createContext(emptyComponents)

/**
 * Get current components from the MDX Context.
 *
 * @param {Readonly<MDXComponents> | MergeComponents | null | undefined} [components]
 *   Additional components to use or a function that creates them (optional).
 * @returns {MDXComponents}
 *   Current components.
 */
function useMDXComponents(components) {
  const contextComponents = react__WEBPACK_IMPORTED_MODULE_0__.useContext(MDXContext)

  // Memoize to avoid unnecessary top-level context changes
  return react__WEBPACK_IMPORTED_MODULE_0__.useMemo(
    function () {
      // Custom merge via a function prop
      if (typeof components === 'function') {
        return components(contextComponents)
      }

      return {...contextComponents, ...components}
    },
    [contextComponents, components]
  )
}

/**
 * Provider for MDX context.
 *
 * @param {Readonly<Props>} properties
 *   Properties.
 * @returns {ReactElement}
 *   Element.
 * @satisfies {Component}
 */
function MDXProvider(properties) {
  /** @type {Readonly<MDXComponents>} */
  let allComponents

  if (properties.disableParentContext) {
    allComponents =
      typeof properties.components === 'function'
        ? properties.components(emptyComponents)
        : properties.components || emptyComponents
  } else {
    allComponents = useMDXComponents(properties.components)
  }

  return react__WEBPACK_IMPORTED_MODULE_0__.createElement(
    MDXContext.Provider,
    {value: allComponents},
    properties.children
  )
}


/***/ }

}]);