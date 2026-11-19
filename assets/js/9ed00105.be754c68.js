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
  "value": "Tenant-Managed Upstreams",
  "id": "tenant-managed-upstreams",
  "level": 3
}, {
  "value": "Forwarding Client Authorization",
  "id": "forwarding-client-authorization",
  "level": 3
}, {
  "value": "Allowing Plain-HTTP Upstreams",
  "id": "allowing-plain-http-upstreams",
  "level": 3
}, {
  "value": "OpenCode Zen And Go",
  "id": "opencode-zen-and-go",
  "level": 3
}, {
  "value": "Provider Inheritance",
  "id": "provider-inheritance",
  "level": 3
}, {
  "value": "Scoped Encrypted Reasoning",
  "id": "scoped-encrypted-reasoning",
  "level": 2
}, {
  "value": "Upstream Header Timeout",
  "id": "upstream-header-timeout",
  "level": 2
}, {
  "value": "Peer Deferred Transport v1",
  "id": "peer-deferred-transport-v1",
  "level": 2
}, {
  "value": "Executor Policy",
  "id": "executor-policy",
  "level": 3
}, {
  "value": "Caller Policy",
  "id": "caller-policy",
  "level": 3
}, {
  "value": "Compatibility Defaults And Migration",
  "id": "compatibility-defaults-and-migration",
  "level": 3
}, {
  "value": "Receiver Limits",
  "id": "receiver-limits",
  "level": 3
}, {
  "value": "Mixed Models: Full Caller And Executor",
  "id": "mixed-models-full-caller-and-executor",
  "level": 3
}, {
  "value": "Local Static Derivative Overlays",
  "id": "local-static-derivative-overlays",
  "level": 3
}, {
  "value": "Full Caller Configuration",
  "id": "full-caller-configuration",
  "level": 3
}, {
  "value": "Full Executor Configuration",
  "id": "full-executor-configuration",
  "level": 3
}, {
  "value": "Upstream TLS ClientHello",
  "id": "upstream-tls-clienthello",
  "level": 2
}, {
  "value": "Custom Specification Files",
  "id": "custom-specification-files",
  "level": 3
}, {
  "value": "Upstream User-Agent",
  "id": "upstream-user-agent",
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
  "value": "<code>credential_ref</code> (GitHub Copilot)",
  "id": "credential_ref-github-copilot",
  "level": 3
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
}, {
  "value": "<code>web_ui</code>",
  "id": "web_ui",
  "level": 3
}, {
  "value": "<code>ingress_guardrails</code>",
  "id": "ingress_guardrails",
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
    strong: "strong",
    table: "table",
    tbody: "tbody",
    td: "td",
    th: "th",
    thead: "thead",
    tr: "tr",
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
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["optional root/provider/model/alias ", (0,jsx_runtime.jsx)(_components.code, {
          children: "encrypted_reasoning"
        }), " policy"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["optional ", (0,jsx_runtime.jsx)(_components.code, {
          children: "deferred"
        }), " executor policy"]
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
        children: "listener \"http\" \"public\" {\n  address = \":8080\"\n\n  timeouts {\n    read_header = \"10s\"\n    idle        = \"60s\"\n    write       = \"0s\"\n  }\n}\n\nupstream_header_timeout = \"120s\"\n\nuser_agent = \"my-proxy/1.0\"\n\nauth \"main\" {\n  mode = \"bearer_static\"\n\n  rate_limit {\n    requests_per_minute = 120\n    burst               = 120\n  }\n\n  client \"internal-app\" {\n    token          = env(\"AIPROXY_CLIENT_TOKEN\")\n    tenant         = \"internal\"\n    allowed_models = [\"alias/chat_default\", \"openai/gpt-4.1\"]\n  }\n}\n\nlogging {\n  level      = \"info\"\n  access_log = true\n}\n\nprovider \"openai\" \"openai\" {\n  display_name = \"OpenAI\"\n  api_key      = env(\"OPENAI_API_KEY\")\n\n  model \"gpt-4.1\" {\n    display_name = \"GPT-4.1\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n\n  model \"text-embedding-3-large\" {\n    display_name = \"text-embedding-3-large\"\n    capabilities = [\"embeddings\"]\n  }\n}\n\nprovider \"openai-compatible\" \"localai\" {\n  display_name = \"LocalAI\"\n  base_url     = \"https://llm.internal/v1\"\n  upstream_header_timeout = \"180s\"\n\n  api_key_ref {\n    key = \"localai\"\n  }\n\n  model \"qwen3-32b\" {\n    display_name = \"Qwen 3 32B\"\n  }\n}\n\nalias \"chat_default\" {\n  algorithm = \"round_robin\"\n\n  target {\n    provider = \"openai\"\n    model    = \"gpt-4.1\"\n  }\n\n  target {\n    provider = \"localai\"\n    model    = \"qwen3-32b\"\n  }\n}\n"
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
      children: ["Listener address and timeout changes still require a restart, even though runtime state such as auth, providers, models, aliases, encrypted-reasoning policies, upstream header timeouts, access-log enablement, metrics, and provider-health config can reload on ", (0,jsx_runtime.jsx)(_components.code, {
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
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The nested ", (0,jsx_runtime.jsx)(_components.code, {
        children: "payload_log"
      }), " block is optional and disabled by default. When\nenabled, the proxy appends one JSON object per line (JSONL) per inference\nrequest with full request/response headers and bodies:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "logging {\n  level      = \"info\"\n  access_log = true\n\n  payload_log {\n    enabled        = true\n    dir            = \"/var/log/aiproxy/payloads\"\n    rotation       = \"daily\"\n    retention      = \"168h\"\n    max_body_bytes = 1048576\n\n    mongodb {\n      uri        = env(\"AIPROXY_PAYLOAD_MONGO_URI\")\n      database   = \"aiproxy\"\n      collection = \"payloads\"\n      timeout    = \"5s\"\n    }\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "enabled"
        }), " defaults to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "false"
        }), "; nothing is written unless it is ", (0,jsx_runtime.jsx)(_components.code, {
          children: "true"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "dir"
        }), " selects the disk backend and holds the ", (0,jsx_runtime.jsx)(_components.code, {
          children: "payload-<date>.jsonl"
        }), " files.\nWhen enabled, at least one of ", (0,jsx_runtime.jsx)(_components.code, {
          children: "dir"
        }), " or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "mongodb.uri"
        }), " is required: set only\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "dir"
        }), " for disk logging, only ", (0,jsx_runtime.jsx)(_components.code, {
          children: "mongodb"
        }), " for MongoDB-only logging, or both to\nfan out to both backends. Omit ", (0,jsx_runtime.jsx)(_components.code, {
          children: "dir"
        }), " (or leave it empty) to disable the disk\nbackend."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "redact_guardrail_bodies"
        }), " defaults to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "false"
        }), ". When ", (0,jsx_runtime.jsx)(_components.code, {
          children: "true"
        }), " and an ingress\nguardrail policy is enabled, chat-completions/responses/messages bodies are\nlogged with gitleaks-flagged text replaced by the guardrail\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "redact_placeholder"
        }), " instead of being omitted entirely (see\n", (0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/operations#payload-logging",
          children: "operations → payload logging"
        }), " for the exact\nsemantics and limits). Bodies that were not scanned (early rejections) or\ncould not be scanned completely or safely redacted stay omitted. Logging-only\nCLI edits preserve a declared ", (0,jsx_runtime.jsx)(_components.code, {
          children: "true"
        }), " or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "false"
        }), " value."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["The optional nested ", (0,jsx_runtime.jsx)(_components.code, {
          children: "mongodb"
        }), " block selects the MongoDB backend, which\ninserts one document per inference request with the same JSON field names as\nthe disk entries. It is disabled when ", (0,jsx_runtime.jsx)(_components.code, {
          children: "uri"
        }), " is empty, so keep the secret in\nthe environment and reference it with ", (0,jsx_runtime.jsx)(_components.code, {
          children: "env(\"...\")"
        }), ". ", (0,jsx_runtime.jsx)(_components.code, {
          children: "database"
        }), " defaults to\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "\"aiproxy\""
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "collection"
        }), " to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "\"payloads\""
        }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "timeout"
        }), " (per-operation\nconnect/ping/insert timeout, Go duration string) to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "\"5s\""
        }), ". Recording is\nbest-effort: a MongoDB outage after startup is dropped without failing the\nrequest, but a bad URI fails startup fast. Consider an index on\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "{request_id: 1}"
        }), " for single-request lookups."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "rotation"
        }), " splits files by datetime to bound single-file growth: ", (0,jsx_runtime.jsx)(_components.code, {
          children: "daily"
        }), "\nwrites ", (0,jsx_runtime.jsx)(_components.code, {
          children: "payload-YYYYMMDD.jsonl"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "hourly"
        }), " writes ", (0,jsx_runtime.jsx)(_components.code, {
          children: "payload-YYYYMMDD-HH.jsonl"
        }), "\n(UTC). Defaults to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "daily"
        }), "."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "retention"
        }), " is the configurable file retention period (Go duration string,\ndefault ", (0,jsx_runtime.jsx)(_components.code, {
          children: "\"168h\""
        }), " / 7 days). Files whose modification time is older than the\nretention window are deleted on rotation and by an hourly background sweep.\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "0"
        }), " (for example ", (0,jsx_runtime.jsx)(_components.code, {
          children: "\"0s\""
        }), ") keeps files forever."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "max_body_bytes"
        }), " caps the stored bytes per request/response body\n(default ", (0,jsx_runtime.jsx)(_components.code, {
          children: "1048576"
        }), "); larger bodies are truncated and marked\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "\"truncated\": true"
        }), ". ", (0,jsx_runtime.jsx)(_components.code, {
          children: "0"
        }), " stores full bodies. Binary bodies are base64\nencoded."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "authorization"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "proxy-authorization"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "cookie"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "set-cookie"
        }), ", and\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "x-api-key"
        }), " header values are always recorded as ", (0,jsx_runtime.jsx)(_components.code, {
          children: "[REDACTED]"
        }), "."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Payload-log changes apply on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " reload without a restart."]
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
      }), " allows anonymous inference only when multi-tenancy is disabled. With\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "multi_tenancy"
      }), " enabled it is a management-only choice: model discovery,\ninference, billing usage, and deferred controls require workspace-scoped bearer\nauthentication and return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), " in ", (0,jsx_runtime.jsx)(_components.code, {
        children: "none"
      }), " mode. Browser sign-in uses separate\nsession credentials."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "bearer_static"
      }), " to serve inference with multi-tenancy enabled. It validates\nbearer tokens from statically configured ", (0,jsx_runtime.jsx)(_components.code, {
        children: "client"
      }), " blocks and, with multi-tenancy,\ndatabase-issued keys. File-managed providers, models, and aliases belong to\n", (0,jsx_runtime.jsx)(_components.strong, {
        children: "System"
      }), "; an HCL client can use only System's catalog. A database key can use\nonly its owning workspace, even if a system administrator issued it."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Each static client may also define:"
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
        }), " mode, each configured client and each database key gets its\nown bucket, even if they have the same name. A database key's bucket follows\nits UUID across token rotation when rate settings remain unchanged. A deleted\nkey's replacement gets a new bucket."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["In ", (0,jsx_runtime.jsx)(_components.code, {
          children: "none"
        }), " mode, all requests share the anonymous bucket."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), " narrows access to public names (", (0,jsx_runtime.jsx)(_components.code, {
        children: "<provider>/<model>"
      }), " or\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "alias/<name>"
      }), "). Omit it or use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[]"
      }), " for all models and aliases available in\nthe credential's own workspace. An alias can be allowed without granting its\ntarget names separately, but every target must still be available in the same\nworkspace, as must a derived provider's base. An invalid cross-workspace pool\nis unavailable as a whole, not a partially usable fallback route. Without\nmulti-tenancy, the allowlist applies to the file-only catalog as before."]
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
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-go"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "github-copilot"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "zenmux"
        }), ", and\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "openrouter"
        }), " (each defaults to its service prefix, e.g.\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "https://zenmux.ai/api/v1"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "https://openrouter.ai/api/v1"
        }), ")"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "allow_insecure_base_url"
        }), " to accept a non-loopback plain-HTTP ", (0,jsx_runtime.jsx)(_components.code, {
          children: "base_url"
        }), "\n(provider-only, default ", (0,jsx_runtime.jsx)(_components.code, {
          children: "false"
        }), "; see below)"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "user_agent"
        }), " as an optional upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "User-Agent"
        }), " override for any provider\ntype (defaults to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "aiproxy/<version>"
        }), ")"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "forward_user_agent"
        }), " to forward the inbound caller ", (0,jsx_runtime.jsx)(_components.code, {
          children: "User-Agent"
        }), " upstream on\nlive inference requests instead of the default (explicit ", (0,jsx_runtime.jsx)(_components.code, {
          children: "user_agent"
        }), " wins)"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "forward_authorization"
        }), " to use the caller's ", (0,jsx_runtime.jsx)(_components.code, {
          children: "Authorization"
        }), " header instead of\na configured upstream key (provider-only, default ", (0,jsx_runtime.jsx)(_components.code, {
          children: "false"
        }), "; see below)"]
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
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "credential_ref"
        }), " for ", (0,jsx_runtime.jsx)(_components.code, {
          children: "github-copilot"
        }), " only (saved device-flow login)"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "upstream_header_timeout"
        })
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "tls_client_hello"
        }), " (provider-only upstream TLS profile; see below)"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "tls_client_hello_spec_file"
        }), " (required only for static ", (0,jsx_runtime.jsx)(_components.code, {
          children: "custom"
        }), " profiles)"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "encrypted_reasoning"
        }), " (whole-block policy; also supported at root, model and\nalias scopes; see ", (0,jsx_runtime.jsx)(_components.a, {
          href: "#scoped-encrypted-reasoning",
          children: "Scoped Encrypted Reasoning"
        }), ")"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "deferred_transport"
        }), " (static-only caller compatibility default; prefer a\nmodel-level block) / ", (0,jsx_runtime.jsx)(_components.code, {
          children: "deferred_receiver"
        }), " (provider-only executor opt-in;\nsee ", (0,jsx_runtime.jsx)(_components.a, {
          href: "#peer-deferred-transport-v1",
          children: "Peer Deferred Transport v1"
        }), ")"]
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
      }), ". Enabled\nproviders with unresolved, empty, or missing credentials fail validation, with\nthese exceptions: providers with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_authorization = true"
      }), " require no\nconfigured credential, and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " providers may omit the credential entirely for\nkeyless upstream access, in which case the proxy sends no ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization"
      }), "\nheader. To\nintentionally disable a provider, declare ", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled = false"
      }), "; disabled\nproviders are still validated for structure, URL, models, capabilities and TLS profiles,\nbut they normally do not require a usable credential. A declared\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_transport"
      }), " on a provider or any of its models still requires a\nconfigured ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ", including on a disabled provider."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "tenant-managed-upstreams",
      children: "Tenant-Managed Upstreams"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For invite-only onboarding, explicitly set ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allow_public_registration = false"
      }), "\ninside ", (0,jsx_runtime.jsx)(_components.code, {
        children: "multi_tenancy"
      }), ". A bare HCL ", (0,jsx_runtime.jsx)(_components.code, {
        children: "multi_tenancy {}"
      }), " enables tenancy and defaults public\npassword registration to ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "true"
      }), "; the configure wizard defaults it to ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "false"
      }), "\nfor new setups and preserves existing choices. Neither setting enables OIDC\njust-in-time provisioning: SSO signs in existing accounts only, including users\nwho redeem an invitation first. With registration closed, recipients can still\npaste a separately delivered one-time code on the clean ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/invite"
      }), " page.\nSee ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/api-reference#invitation-onboarding",
        children: "invitation and SSO admission"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Enabling tenancy does not configure a funded workspace budget or assign a payer.\nDatabase serving changes reconcile across replicas with a fixed 30-second\nfreshness admission deadline; there is no configurable stale-serving bypass.\nSee ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#bounded-replica-convergence",
        children: "bounded replica convergence"
      }), "\nfor independent key revocation, readiness and file-rollout limitations.\nDatabase user/team quotas are owner-scoped completion-based admission thresholds,\nnot hard spending limits; ownerless keys and static/HCL traffic have no such\nquota scope. See the ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#spending-contract-owner-quotas-not-funded-workspace-caps",
        children: "spending contract"
      }), "\nbefore offering usage limits."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["With multi-tenancy enabled, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "non-System database providers are denied outbound\naccess by default"
      }), ". The operator approves exact public HTTPS origins in the\nHCL or JSON config, never through a provider API or tenant portal field:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        metastring: "title=\"tenant-upstream-policy.hcl\"",
        children: "listener \"http\" \"public\" { address = \"127.0.0.1:8080\" }\nauth \"main\" {\n  mode = \"bearer_static\"\n  client \"operator\" { token = env(\"AIPROXY_OPERATOR_TOKEN\") }\n}\ndatabase { url = env(\"AIPROXY_DATABASE_URL\") }\nmulti_tenancy {\n  allow_public_registration = false\n  allowed_upstream_origins = [\n    \"https://api.openai.com\",\n    \"https://api.githubcopilot.com\",\n    \"https://probe.example.com:8443\",\n  ]\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The equivalent JSON config uses the same attribute and the usual bare ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env()"
      }), "\nvalues:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-json",
        metastring: "title=\"tenant-upstream-policy.json\"",
        children: "{\n  \"listener\": { \"http\": { \"public\": { \"address\": \"127.0.0.1:8080\" } } },\n  \"auth\": {\n    \"main\": {\n      \"mode\": \"bearer_static\",\n      \"client\": { \"operator\": { \"token\": env(\"AIPROXY_OPERATOR_TOKEN\") } }\n    }\n  },\n  \"database\": { \"url\": env(\"AIPROXY_DATABASE_URL\") },\n  \"multi_tenancy\": {\n    \"allow_public_registration\": false,\n    \"allowed_upstream_origins\": [\n      \"https://api.openai.com\",\n      \"https://api.githubcopilot.com\",\n      \"https://probe.example.com:8443\"\n    ]\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Omission or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[]"
      }), " approves nothing. At most ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "128"
      }), " unique origins are accepted.\nAn entry contains only ", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://hostname[:port]"
      }), ": no wildcard, path (even ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/"
      }), "),\nuserinfo, query or fragment. Hostnames are ASCII fully qualified names without a\ntrailing dot; local/internal names and non-public IP literals are rejected.\nMatching uses the exact scheme, case-insensitive hostname and numeric port;\nomitted HTTPS port and ", (0,jsx_runtime.jsx)(_components.code, {
        children: ":443"
      }), " are equivalent. A parent domain does not approve\nsubdomains, and another port requires its own entry."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The effective upstream URL, including provider-type defaults and inherited\nsettings, must match an approval. Relative health paths use that upstream\norigin; an absolute health URL needs its own exact approval. This policy applies\nto the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "destination workspace"
      }), ", including edits authored by global\nadministrators, disabled rows, credential-only updates and startup/reload of\nexisting rows. It does not grant System or foreign provider/model/alias access,\ninheritance, or credential rights."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Tenant providers may use their own write-only, encrypted database API keys,\ntheir own encrypted Copilot device-flow database credentials, or supported\nkeyless Zen access. Nonempty host ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), " and sidecar ", (0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref"
      }), "\nreferences, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_authorization = true"
      }), " and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "allow_insecure_base_url = true"
      }), " are reserved for System. Approval does not\noverride those restrictions. The portal hides these controls in tenant\nworkspaces; the server enforces them independently."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Every new restricted connection resolves DNS, rejects the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "entire"
      }), " answer set\nif any address is non-public, then dials only the validated numeric IPs while\nretaining the original TLS hostname/SNI and certificate verification. Loopback,\nprivate, link-local, metadata, multicast, documentation/reserved and transition\naddresses are rejected, including mapped IPv4 and IPv6 answers. DNS lookup and\nTCP attempts share a 30-second budget and at most 64 answers are accepted.\nEnvironment HTTP/HTTPS/SOCKS proxies are not used for tenant upstreams. Native\nand built-in TLS profiles use this same restriction. Existing same-origin\nredirects remain supported; even a separately approved origin cannot be reached\nby a cross-origin redirect."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "LoadFile"
      }), ", operator inline config and HCL/JSON conversion use the same approval\nvalidation. Conversion preserves the attribute and resolves ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env()"
      }), " as usual;\nnone of these operations proves DNS reachability or sends a probe. Runtime IP\nenforcement is not a validation-time DNS check. System-managed providers, HCL\nproviders and file-only operation retain their trusted transport/credential\nbehavior. See ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#tenant-provider-policy-upgrade",
        children: "upgrade and preflight"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "forwarding-client-authorization",
      children: "Forwarding Client Authorization"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"openai-compatible\" \"custom\" {\n  base_url = \"https://upstream.example.com/v1\"\n  forward_authorization = true\n\n  model \"chat\" {}\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "forward_authorization"
      }), " is supported by ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai-compatible"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-go"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "zenmux"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openrouter"
      }), ". It is not supported\nby ", (0,jsx_runtime.jsx)(_components.code, {
        children: "anthropic"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "github-copilot"
      }), ". There is no root-level default;\nderived providers must opt in locally rather than inherit it."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When enabled, live inference forwards exactly one nonempty, valid client\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization"
      }), " header unchanged, including its authentication scheme. Missing,\nblank, repeated, oversized (over 4096 bytes), or invalid header values return\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "400"
      }), " before upstream I/O, without credential fallback or alias failover.\nConfiguring ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ", or another credential alongside this mode\nis rejected. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization"
      }), " remains forbidden in ", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_headers"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.strong, {
        children: "Proxy authentication still runs first."
      }), " With bearer or JWT authentication,\nthe same token used to authenticate to aiproxy is sent upstream. Enable this\nonly for trusted upstreams; for clients supplying upstream tokens directly,\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "auth"
      }), " mode ", (0,jsx_runtime.jsx)(_components.code, {
        children: "none"
      }), " requires separate access protection. Background health probes\nand CLI upstream model discovery have no caller header and do not forward it.\nAlias health and cooldown state remain shared across callers, not per forwarded\ncredential. Authorization-mode changes apply on successful ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " reload."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The CLI supports ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy configure provider --non-interactive ... --forward-authorization"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "--forward-authorization=false"
      }), ". Administrative\nprovider POST/PUT in the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "System workspace"
      }), " also accepts ", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_authorization"
      }), "; existing credentials\nmust be explicitly cleared when switching modes. Existing databases require\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy migrate up"
      }), " to apply the additive forwarding-column migration."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "allowing-plain-http-upstreams",
      children: "Allowing Plain-HTTP Upstreams"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"openai-compatible\" \"incluster\" {\n  base_url = \"http://llm.internal.my-ns.svc.cluster.local/v1\"\n  allow_insecure_base_url = true\n  api_key_ref { key = \"localai\" }\n\n  model \"chat\" {}\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), " must normally be ", (0,jsx_runtime.jsx)(_components.code, {
        children: "https"
      }), ", with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "http"
      }), " allowed only for loopback\nhosts (", (0,jsx_runtime.jsx)(_components.code, {
        children: "localhost"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "127.0.0.1"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "::1"
      }), "). Setting ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allow_insecure_base_url = true"
      }), "\non a provider opts that provider into a non-loopback plain-HTTP upstream. This\nexists for trusted transport-local networks, most commonly another Service in\nthe same Kubernetes namespace. There is no root-level default, environment\nvariable, or CLI bypass; the opt-in is per provider and explicit."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.strong, {
        children: "Credentials and payloads travel unencrypted."
      }), " Only enable this for networks\nwhere cleartext HTTP is acceptable. For trusted System/HCL providers, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "https"
      }), "\nbase URLs are always allowed and\nthe flag has no effect on them; non-http(s) schemes, userinfo, and\nnon-absolute URLs remain rejected. Derived providers cannot declare the\nattribute — it is inherited from the base provider together with the base URL.\nThe admin API rejects explicit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "true"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "false"
      }), " on derived providers. Changing\na concrete database provider to a derived one clears its omitted local flag;\nstored derived rows with a true local override are rejected during catalog load."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The CLI supports ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy configure provider --non-interactive ... --allow-insecure-base-url"
      }), " (and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "--allow-insecure-base-url=false"
      }), "), with a\nplain-HTTP confirmation prompt in interactive flows. Administrative provider\nPOST/PUT in the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "System workspace"
      }), " also accepts ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allow_insecure_base_url"
      }), ", and existing databases require\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy migrate up"
      }), " to apply the additive column migration."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["GitHub Copilot is ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "hermetically verified; live GitHub compatibility unverified"
      }), ".\nConfig validation resolves local credentials; it does not prove GitHub entitlement\nor model availability. See ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#mock-only-copilot-verification",
        children: "mock-only verification"
      }), "\nfor checks requiring no real client ID/account."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "github-copilot"
      }), " never uses ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ". Provision with\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy login github-copilot --client-id <id> --credential <name>"
      }), " (your own\npublic OAuth client ID, no secret), then reference the saved login:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"github-copilot\" \"copilot\" {\n  credential_ref {\n    name = \"copilot-main\"\n  }\n\n  model \"gpt-5.4-nano\" {}\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref.path"
      }), " is optional and defaults to the shared secrets path so\nthe ", (0,jsx_runtime.jsx)(_components.code, {
        children: "copilot-<name>.json"
      }), " sidecar is found next to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "keys.json"
      }), ". Derived\nCopilot providers require their own local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref"
      }), ". The token\nactivates on restart/", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), "; re-run ", (0,jsx_runtime.jsx)(_components.code, {
        children: "login"
      }), " with the same client ID/name\nand reload on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), ", revocation, or expiry."]
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
      }), " only as a transport override for tests and custom gateways. An\noverride never changes service selection, auth, or header behavior.\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "github-copilot"
      }), " defaults to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://api.githubcopilot.com"
      }), " with the same\ntransport-override-only ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), " rule. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "zenmux"
      }), " defaults to\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://zenmux.ai/api/v1"
      }), " with the same transport-override-only rule.\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "openrouter"
      }), " defaults to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://openrouter.ai/api/v1"
      }), " with the same\ntransport-override-only rule."]
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
      }), " is native Anthropic passthrough for ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/messages"
      }), " (plus\nconservative translation for ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), ") and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " serves ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), "\nand ", (0,jsx_runtime.jsx)(_components.code, {
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
      }), " on every upstream\nrequest (caller values are forwarded only when valid, falling back to a valid\ncaller ", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Session-Id"
      }), ", otherwise a fresh\nper-request ID is generated); direct requests never cross services, and only\nexplicitly configured aliases retry another target. See\n", (0,jsx_runtime.jsx)(_components.a, {
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
      children: ["A derived provider may be declared before or after its base. It normally declares ", (0,jsx_runtime.jsx)(_components.code, {
        children: "extends"
      }), ", optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "display_name"
      }), ", and exactly one local credential, either ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ". It inherits the base provider type, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), ", effective upstream header timeout, TLS ClientHello settings/profile, enabled state, and all model blocks."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "github-copilot"
      }), " derivatives instead require a local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref"
      }), "; they cannot redefine ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), " or model definitions, but may use the reasoning-only overlays below. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref"
      }), " is rejected on all other types, and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), " are rejected on Copilot blocks."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The type label remains required and must match the base. The base must exist, be enabled, and must not itself use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "extends"
      }), "; inheritance chains are rejected. Local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_header_timeout"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello_spec_file"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled"
      }), " declarations are rejected instead of ignored, including explicit TLS defaults. Local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " declarations are rejected except for existing-name policy overlays described below."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["With multi-tenancy enabled, these HCL providers and their models belong only to\nSystem. Database-managed inheritance requires a base in the derived provider's\nown workspace, even when a global administrator authors the row or the\nderivative is disabled. Tenant providers cannot inherit System provider/model\ninventory or another tenant's inventory. Alias targets, including shorthand, have the same\nworkspace restriction. Provider and alias names remain globally unique:\nstartup/reload rejects a static/managed name collision, including disabled\nproviders, instead of replacing or reassigning a resource. File-only operation\nis unchanged. Existing deployments should run\n", (0,jsx_runtime.jsxs)(_components.a, {
        href: "/docs/operations#workspace-isolation-upgrade-and-recovery",
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "validate --check-db"
        }), " before upgrading"]
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Derived providers are flattened during config loading and reload. After a successful load, direct model strings, health, metrics, billing, and dashboard inventory use the derived provider's own name. Aliases still list each provider target explicitly."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Declared base provider/model reasoning policies are copied by value. A local\nprovider ", (0,jsx_runtime.jsx)(_components.code, {
        children: "encrypted_reasoning"
      }), " block replaces the inherited provider block, but\nan inherited model block still outranks it. Override/reset that model with a\nlocal existing-name ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " overlay; removing the overlay restores the base\nmodel declaration, not necessarily the child's provider policy."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Static derivatives inherit model definitions, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "not"
      }), " base provider or model\ndeferred caller opt-ins, nor receiver opt-ins/limits. An existing-name model\noverlay may contain ", (0,jsx_runtime.jsx)(_components.code, {
        children: "encrypted_reasoning"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_transport"
      }), ", or both\n(at most one of each, at least one total). New model\nnames, definition fields (", (0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_name"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "display_name"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "protocol"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "capabilities"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "pricing"
      }), "), empty model overlays and duplicate overlays are\nrejected, even if a forbidden field is empty, null or a default value."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["A local provider caller compatibility default is still supported on a static\nderivative. With no local model block or provider default, the inherited model\nuses ordinary transport. Local caller blocks use standard caller defaults;\nlocal provider-only ", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_receiver"
      }), " blocks use root defaults. See the\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "#local-static-derivative-overlays",
        children: "exact-tested overlay example"
      }), ".\nManaged derivatives cannot extend a base carrying any provider/model caller\nsetting or receiver opt-in."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "scoped-encrypted-reasoning",
      children: "Scoped Encrypted Reasoning"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["An optional unlabeled ", (0,jsx_runtime.jsx)(_components.code, {
        children: "encrypted_reasoning"
      }), " block controls replayed reasoning\nhistory for ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/chat/completions"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/responses"
      }), ". It can be\ndeclared at ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "root, provider, model, or alias"
      }), " scope, including on disabled\nproviders. It does not enable a model capability or decrypt upstream outputs.\nNative ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/messages"
      }), " and all other operations are unaffected."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Direct precedence: ", (0,jsx_runtime.jsx)(_components.strong, {
          children: "model → provider → root → built-in defaults"
        }), "."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Alias precedence: ", (0,jsx_runtime.jsx)(_components.strong, {
          children: "explicit alias → selected model → selected provider →\nroot → built-in defaults"
        }), ". Without an alias block, resolution happens per\nselected target, not from the first member of the pool."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["A missing block inherits. A present empty ", (0,jsx_runtime.jsx)(_components.code, {
          children: "encrypted_reasoning {}"
        }), " resets to\nsafe built-ins and stops inheritance. Blocks replace ", (0,jsx_runtime.jsx)(_components.strong, {
          children: "wholesale"
        }), ": omitted\nmembers use built-ins, never members of the lower-priority block."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Member"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Built-in default"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Meaning"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "passthrough"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "true"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Send history intact; ", (0,jsx_runtime.jsx)(_components.code, {
              children: "false"
            }), " pre-strips supported reasoning shapes without adding a recovery attempt."]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "on_caller_mismatch"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "\"fail\""
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "\"strip_and_retry\""
            }), " permits one conditional same-target recovery. Explicit ", (0,jsx_runtime.jsx)(_components.code, {
              children: "\"\""
            }), " also selects ", (0,jsx_runtime.jsx)(_components.code, {
              children: "\"fail\""
            }), "."]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "match_messages"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Built-in substrings"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["A nonempty list replaces built-ins; omitted or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "[]"
            }), " uses built-ins."]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Patterns are literal case-insensitive substrings, trimmed and lowercased before\nvalidation/matching. Supply at most 16, each ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "4–256 Go string bytes after\nnormalization"
      }), ", unique case-insensitively. Keep them specific; this is not\ngeneral error recovery. Built-ins are ", (0,jsx_runtime.jsx)(_components.code, {
        children: "not issued to this caller"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "invalid_encrypted_content"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "could not be verified"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "could not be decrypted"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "item_id did not match"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "invalid signature"
      }), ". Unknown members, wrong types,\nduplicate/case-variant keys and invalid blocks are rejected."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["This complete example uses environment-supplied credentials and loopback stub\nURLs; config validation makes no upstream calls. It is also shipped as\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "examples/scoped-encrypted-reasoning.hcl"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        metastring: "title=\"scoped-encrypted-reasoning.hcl\"",
        children: "listener \"http\" \"public\" {\n  address = \"127.0.0.1:8080\"\n}\n\nauth \"main\" {\n  mode = \"bearer_static\"\n  client \"application\" {\n    token = env(\"AIPROXY_CLIENT_TOKEN\")\n  }\n}\n\nencrypted_reasoning {\n  on_caller_mismatch = \"strip_and_retry\"\n  match_messages    = [\"  Caller binding mismatch  \"]\n}\n\nprovider \"openai-compatible\" \"primary\" {\n  base_url = \"http://127.0.0.1:18081/v1\"\n  api_key  = env(\"UPSTREAM_API_KEY\")\n\n  encrypted_reasoning {\n    passthrough = false\n  }\n\n  model \"chat\" {}\n  model \"recover\" {\n    encrypted_reasoning {\n      on_caller_mismatch = \"strip_and_retry\"\n    }\n  }\n  model \"safe\" {\n    encrypted_reasoning {}\n  }\n}\n\nprovider \"openai-compatible\" \"secondary\" {\n  extends = \"primary\"\n  api_key = env(\"SECONDARY_UPSTREAM_API_KEY\")\n\n  encrypted_reasoning {}\n\n  model \"chat\" {\n    encrypted_reasoning {\n      on_caller_mismatch = \"strip_and_retry\"\n      match_messages    = []\n    }\n  }\n  model \"safe\" {\n    encrypted_reasoning {\n      passthrough = false\n    }\n  }\n}\n\nprovider \"openai-compatible\" \"inherited\" {\n  extends = \"primary\"\n  api_key = env(\"SECONDARY_UPSTREAM_API_KEY\")\n}\n\nprovider \"openai-compatible\" \"root-only\" {\n  base_url = \"http://127.0.0.1:18082/v1\"\n  api_key  = env(\"UPSTREAM_API_KEY\")\n  model \"chat\" {}\n}\n\nalias \"target_policy\" {\n  algorithm = \"round_robin\"\n  providers = [\"primary\", \"secondary\"]\n  model     = \"chat\"\n}\n\nalias \"safe_pool\" {\n  algorithm = \"round_robin\"\n  providers = [\"primary\", \"secondary\"]\n  model     = \"recover\"\n  encrypted_reasoning {}\n}\n\nalias \"recover_pool\" {\n  algorithm = \"round_robin\"\n  providers = [\"primary\", \"secondary\"]\n  model     = \"recover\"\n  encrypted_reasoning {\n    on_caller_mismatch = \"strip_and_retry\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Here ", (0,jsx_runtime.jsx)(_components.code, {
        children: "primary/chat"
      }), " strips with built-in ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"fail\""
      }), " and built-in patterns, not\nthe root's retry/custom pattern. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "primary/safe"
      }), " resets to passthrough/fail.\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "secondary/recover"
      }), " retains its inherited model retry policy despite the child\nprovider reset. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "secondary/chat"
      }), " is a local model retry override with default\npatterns; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "secondary/safe"
      }), " strips via a local model override. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "root-only/chat"
      }), "\nuses the root's normalized ", (0,jsx_runtime.jsx)(_components.code, {
        children: "caller binding mismatch"
      }), " pattern.\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "inherited/chat"
      }), " copies the base provider strip policy; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "inherited/safe"
      }), " copies\nthe base model reset, with no local declarations.\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "alias/safe_pool"
      }), " resets the entire pool; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "alias/target_policy"
      }), " instead uses\neach selected target's policy."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Removing a static block/key resumes inheritance; static HCL does not use\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "encrypted_reasoning = null"
      }), ", and static JSON null policies are rejected.\nManaged provider/model/alias policies have\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/api-reference#encrypted-reasoning-policy-intent",
        children: "explicit nullable administrative semantics"
      }), ",\nincluding sparse derivative model overlays. Root remains static-only. The\nCLI supports ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#editing-encrypted-reasoning-policies",
        children: "inherit/reset/override editing"
      }), ".\nManaged provider/model/alias web forms also offer inherit, safe reset and sparse\ncustom controls, with inherited definitions and static rows read-only. Effective\ncontext describes saved state, not unsaved resolution or proof of activation;\nan undeclared alias remains target-dependent."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.strong, {
        children: "Stripping is lossy."
      }), " Ordinary message text and tool history are retained,\nbut supported typed thinking/reasoning items and chat ", (0,jsx_runtime.jsx)(_components.code, {
        children: "reasoning"
      }), "/\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "reasoning_content"
      }), " fields can be removed even without their own signature.\nDo not assume universal plaintext-reasoning preservation. See\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/providers-and-routing#scoped-encrypted-reasoning",
        children: "recovery gates, streaming and alias budgets"
      }), "\nbefore enabling retry or pre-stripping."]
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
      children: ["Precedence is provider value, then root value, then the 90-second default. The timeout applies only until response headers arrive; it does not cap JSON or streaming response bodies after headers are received. Deferred execution has a separate whole-job deadline. Root and provider timeout changes apply on a successful ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " reload."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "peer-deferred-transport-v1",
      children: "Peer Deferred Transport v1"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["This opt-in replaces a silent, long-lived ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "caller proxy → executor proxy"
      }), " chat\nrequest with one submission and bounded finite polls. End clients still call\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/chat/completions"
      }), " with their original ", (0,jsx_runtime.jsx)(_components.code, {
        children: "stream"
      }), " choice and receive ordinary\nJSON or SSE. It is a transport extension, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "not a model capability, Batch API, or\nResponses background mode"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Role / operation"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Opt-in and behavior"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Caller"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Static ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai-compatible"
            }), " model with local ", (0,jsx_runtime.jsx)(_components.code, {
              children: "deferred_transport"
            }), ", or its provider's local compatibility default; chat JSON/SSE only"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Executor"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Root ", (0,jsx_runtime.jsx)(_components.code, {
              children: "deferred"
            }), " plus a static ", (0,jsx_runtime.jsx)(_components.code, {
              children: "deferred_receiver"
            }), " provider and a chat-capable model; uses the existing adapter"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Caller aliases"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "May mix ordinary and deferred targets; peer control failures never authorize a new inference attempt"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Executor aliases"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Every configured target must be static, receiver-opted-in and chat-capable, with no caller opt-in; mixed pools are rejected, not filtered or reordered"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Other operations"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Use ordinary transport and existing capability checks, even on a caller-opted-in model"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The caller's ", (0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_name"
      }), " names the executor's ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "public"
      }), " model, such as\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "primary/chat"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "alias/peer_chat"
      }), ". Executor aliases advertise the minimum\nexecution timeout and result limit of the entire target pool."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["All three settings are unlabeled blocks, supported in HCL, JSON and inline\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "AIPROXY_CONFIG"
      }), ". Block presence enables the opt-in; there is no ", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled"
      }), "\nattribute, and null declarations are invalid. A caller-only proxy does not need a\nroot ", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred"
      }), " block. An executor requires ", (0,jsx_runtime.jsx)(_components.code, {
        children: "auth.mode = \"bearer_static\""
      }), ".\nEach directly opted-in caller model must support chat; provider compatibility\ndefaults and receivers require at least one chat-capable model. Settings are\nvalidated even on disabled providers. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_receiver"
      }), " remains\n", (0,jsx_runtime.jsx)(_components.strong, {
        children: "provider-only"
      }), " and additive: eligible models still accept ordinary requests.\nIt does not force ordinary calls into deferral. A receiver cannot coexist with\nany caller model setting or provider caller default on the same provider, and\nneither role can combine with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_authorization"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["These settings are ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "static-config-only"
      }), ". Managed provider/credential writes\nreject either declaration, including nested model declarations and empty/null\nor case-variant fields, before commit; managed providers cannot inherit an\nopted-in base. There are no new\ninteractive or non-interactive CLI creation flags or web form fields. Existing\nCLI edits preserve the blocks and supported ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env()"
      }), " spelling."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "executor-policy",
      children: "Executor Policy"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Defaults below apply when ", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred {}"
      }), " is declared. Durations are Go duration\nstrings; byte sizes and counts are integers. Ranges are inclusive."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.th, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "deferred"
            }), " attribute"]
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Default"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Range / rule"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_jobs"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "64"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "1–4096; retained jobs and outstanding finalization ownership"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_running"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "8"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["1–256, at most ", (0,jsx_runtime.jsx)(_components.code, {
              children: "max_jobs"
            }), "; preparation reserves a slot"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_admissions"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "4"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "1–64 concurrent submit readers/preparations"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_entries"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "256"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "max_jobs"
            }), "–65536; dedup records, including rejections and tombstones"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_memory_bytes"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "268435456"
            }), " (256 MiB)"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "16 MiB–4 GiB; feature-owned reservations, not total process RSS"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_request_bytes"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "1048576"
            }), " (1 MiB)"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "1 KiB–8 MiB decoded inference input, before base64 expansion"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_result_bytes"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "8388608"
            }), " (8 MiB)"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "1 KiB–32 MiB retained output / buffered upstream body ceiling"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_work_bytes"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "67108864"
            }), " (64 MiB)"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "4–256 MiB temporary work reservation per job"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "per_owner_jobs"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "8"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["1–", (0,jsx_runtime.jsx)(_components.code, {
              children: "max_jobs"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "per_owner_running"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "2"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["1–", (0,jsx_runtime.jsx)(_components.code, {
              children: "max_running"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_control_inflight"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "16"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "4–128; two slots reserved for DELETE"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "execution_timeout"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "\"10m\""
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "1s–1h, starting at commit"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "retention"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "\"2m\""
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "10s–1h, starting at terminal sealing; reads do not renew it"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "control_timeout"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "\"5s\""
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "100ms–30s, finite control read/write deadline"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "finalization_timeout"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "\"5s\""
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "100ms–30s, completion deadline including queue wait"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Validation requires enough memory for fixed metadata/control allowances, submit\ndecoding scratch, and at least one maximum input, output and work reservation.\nConcurrency settings are upper bounds: memory pressure may admit fewer jobs.\nThere is no inference execution queue."
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "caller-policy",
      children: "Caller Policy"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The caller selects transport for the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "resolved model"
      }), " on direct requests and\nfor each selected alias target. Models sharing a provider share its endpoint and\ncredential, not necessarily its chat transport."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.th, {
            children: ["Local model ", (0,jsx_runtime.jsx)(_components.code, {
              children: "deferred_transport"
            })]
          }), (0,jsx_runtime.jsxs)(_components.th, {
            children: ["Local provider ", (0,jsx_runtime.jsx)(_components.code, {
              children: "deferred_transport"
            })]
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Effective chat transport"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: ["Present, including ", (0,jsx_runtime.jsx)(_components.code, {
              children: "{}"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Present or absent"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Model block, replacing the whole provider block"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Absent"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Present"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Provider compatibility default"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Absent"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Absent"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Ordinary request"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["An empty model block opts in with all standard caller defaults below. A partial\nmodel block uses those same defaults for every omitted field, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "never values\nfrom the provider block"
      }), ". Omitting a model block is not an opt-out when a\nprovider default exists."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.th, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "deferred_transport"
            }), " attribute"]
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Default"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Inclusive range / rule"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "control_timeout"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "\"5s\""
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "100ms–30s; each complete HTTP control transfer"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_wait"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "\"15m\""
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "1s–2h; total wait starts before capability discovery"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "poll_interval"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "\"250ms\""
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "50ms–5s; minimum empty-poll delay, combined with peer advice"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_control_calls"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "4096"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "4–65536; discovery, submission and retrieval calls combined"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_control_retries"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "8"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "0–32 failed-call retries total, not per poll"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The effective call timeout is bounded by the caller setting, advertised executor\nsetting and remaining ", (0,jsx_runtime.jsx)(_components.code, {
        children: "max_wait"
      }), ". One separate, deadline-bound best-effort DELETE\nis outside the retrieval call/retry budgets. The caller uses its configured\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), " to authenticate to the executor, not the end client's\ntoken."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "compatibility-defaults-and-migration",
      children: "Compatibility Defaults And Migration"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Existing provider-level caller blocks remain supported as ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "local compatibility\ndefaults"
      }), "; they are never silently ignored. This compatibility example can be\nappended to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "peer-model-caller.hcl"
      }), " below:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        metastring: "title=\"peer-compatibility-default.hcl\"",
        children: "provider \"openai-compatible\" \"legacy\" {\n  base_url = \"https://executor.example/v1\"\n  api_key  = env(\"AIPROXY_PEER_TOKEN\")\n  deferred_transport {\n    control_timeout     = \"20s\"\n    max_wait            = \"1h\"\n    poll_interval       = \"1s\"\n    max_control_calls   = 2048\n    max_control_retries = 20\n  }\n\n  model \"model-1\" {\n    upstream_name = \"b/model-1\"\n  }\n  model \"model-2\" {\n    upstream_name = \"b/model-2\"\n    deferred_transport {}\n  }\n  model \"model-3\" {\n    upstream_name = \"b/model-2\"\n    deferred_transport {\n      max_wait = \"20m\"\n    }\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Here all three chat models defer: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model-1"
      }), " uses the provider values, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model-2"
      }), "\nuses all standard defaults, and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model-3"
      }), " uses ", (0,jsx_runtime.jsx)(_components.code, {
        children: "20m"
      }), " plus standard defaults for\nthe other four fields. Add their ", (0,jsx_runtime.jsx)(_components.code, {
        children: "legacy/..."
      }), " names to the application's\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), " if exposing this compatibility example. There is no\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled = false"
      }), " or other caller opt-out.\nTo mix ordinary and deferred models, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "remove the provider-wide default"
      }), " and\nmove the desired full policy into each intended model block. Copy all fields\nwhose old nonstandard values must survive; partial blocks do not merge."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Unsupported provider types, non-chat-only model opt-ins, labeled/duplicate\nblocks, unknown attributes, null declarations, model-level receivers and\nout-of-range settings fail validation rather than reverting to ordinary\ntransport. Correct the declaration; do not rely on a silent fallback."
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "receiver-limits",
      children: "Receiver Limits"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.th, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "deferred_receiver"
            }), " attribute"]
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Default"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Inclusive range / rule"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "execution_timeout"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Root ", (0,jsx_runtime.jsx)(_components.code, {
              children: "execution_timeout"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "1s–root value; may narrow, never increase it"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "max_result_bytes"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Root ", (0,jsx_runtime.jsx)(_components.code, {
              children: "max_result_bytes"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "1024–root value; may narrow, never increase it"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "mixed-models-full-caller-and-executor",
      children: "Mixed Models: Full Caller And Executor"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "These complete configurations demonstrate the single-provider case:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-text",
        children: "client → a/model-1 → b/model-1: ordinary chat\nclient → a/model-2 → b/model-2: deferred chat\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Set ", (0,jsx_runtime.jsx)(_components.code, {
        children: "AIPROXY_CLIENT_TOKEN"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "AIPROXY_PEER_TOKEN"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "UPSTREAM_API_KEY"
      }), " to distinct\nnonempty secrets and replace the ", (0,jsx_runtime.jsx)(_components.code, {
        children: ".example"
      }), " hosts. The HTTPS executor address\nassumes a TLS edge forwarding to its HTTP listener. The caller needs no root\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred"
      }), " block."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        metastring: "title=\"peer-model-caller.hcl\"",
        children: "listener \"http\" \"public\" {\n  address = \":8080\"\n  timeouts {\n    read_header = \"10s\"\n    idle        = \"60s\"\n    write       = \"0s\"\n  }\n}\n\nauth \"main\" {\n  mode = \"bearer_static\"\n  client \"application\" {\n    token          = env(\"AIPROXY_CLIENT_TOKEN\")\n    allowed_models = [\"a/model-1\", \"a/model-2\"]\n  }\n}\n\nprovider \"openai-compatible\" \"a\" {\n  base_url = \"https://executor.example/v1\"\n  api_key  = env(\"AIPROXY_PEER_TOKEN\")\n\n  model \"model-1\" {\n    upstream_name = \"b/model-1\"\n  }\n  model \"model-2\" {\n    upstream_name = \"b/model-2\"\n    deferred_transport {\n      max_wait      = \"20m\"\n      poll_interval = \"500ms\"\n    }\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        metastring: "title=\"peer-model-executor.hcl\"",
        children: "listener \"http\" \"public\" {\n  address = \":8081\"\n  timeouts {\n    read_header = \"10s\"\n    idle        = \"60s\"\n    write       = \"0s\"\n  }\n}\n\nupstream_header_timeout = \"10m\"\n\nauth \"main\" {\n  mode = \"bearer_static\"\n  client \"peer-caller\" {\n    token          = env(\"AIPROXY_PEER_TOKEN\")\n    tenant         = \"peer\"\n    allowed_models = [\"b/model-1\", \"b/model-2\"]\n  }\n}\n\ndeferred {}\n\nprovider \"openai-compatible\" \"b\" {\n  base_url = \"https://upstream.example/v1\"\n  api_key  = env(\"UPSTREAM_API_KEY\")\n  deferred_receiver {}\n\n  model \"model-1\" {\n    upstream_name = \"upstream-model-1\"\n  }\n  model \"model-2\" {\n    upstream_name = \"upstream-model-2\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Both models belong to the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "same receiver provider"
      }), ". Its receiver block permits\nprivate admission for either chat model but does not change ordinary\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/chat/completions"
      }), " calls into jobs. Only the caller's ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model-2"
      }), " chat\nrequests use capability discovery, submit and finite reads. Responses and\nembeddings on either model use ordinary routes and normal capability checks."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "local-static-derivative-overlays",
      children: "Local Static Derivative Overlays"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Append this exact-tested snippet to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "peer-model-caller.hcl"
      }), ". The local credential\nis required even when reusing the same peer token (and thus executor owner):"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        metastring: "title=\"peer-model-derived.hcl\"",
        children: "provider \"openai-compatible\" \"derived\" {\n  extends = \"a\"\n  api_key = env(\"AIPROXY_PEER_TOKEN\")\n  model \"model-2\" {\n    deferred_transport {}\n  }\n}\n\nprovider \"openai-compatible\" \"ordinary-derived\" {\n  extends = \"a\"\n  api_key = env(\"AIPROXY_PEER_TOKEN\")\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "derived/model-2"
      }), " inherits its model definition but uses standard caller defaults,\nnot the base's ", (0,jsx_runtime.jsx)(_components.code, {
        children: "20m"
      }), " / ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500ms"
      }), ". ", (0,jsx_runtime.jsx)(_components.code, {
        children: "derived/model-1"
      }), " is ordinary. Both models on\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "ordinary-derived"
      }), " are ordinary because base caller opt-ins are not inherited.\nTo expose these derivative names to the application, add them to its\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), ". Overlays must name an existing inherited model and contain only\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_transport"
      }), "; change model definitions on the concrete base, not here."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "full-caller-configuration",
      children: "Full Caller Configuration"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Copy the following into ", (0,jsx_runtime.jsx)(_components.code, {
        children: "peer-caller.hcl"
      }), ". Set each referenced environment\nvariable to a nonempty secret, and replace the reserved ", (0,jsx_runtime.jsx)(_components.code, {
        children: ".example"
      }), " hosts for\ndeployment. The executor example below uses the same ", (0,jsx_runtime.jsx)(_components.code, {
        children: "AIPROXY_PEER_TOKEN"
      }), ";\napplication and metrics credentials are separate. The HTTPS executor address\nassumes a TLS edge forwarding to its HTTP listener."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        metastring: "title=\"peer-caller.hcl\"",
        children: "listener \"http\" \"public\" {\n  address = \":8080\"\n  timeouts {\n    read_header = \"10s\"\n    idle        = \"60s\"\n    write       = \"0s\"\n  }\n}\n\nauth \"main\" {\n  mode = \"bearer_static\"\n  client \"application\" {\n    token          = env(\"AIPROXY_CLIENT_TOKEN\")\n    allowed_models = [\"peer/chat\"]\n  }\n}\n\nlogging {\n  level      = \"info\"\n  access_log = true\n}\n\nmetrics {\n  token = env(\"AIPROXY_CALLER_METRICS_TOKEN\")\n}\n\nprovider \"openai-compatible\" \"peer\" {\n  base_url = \"https://executor.example/v1\"\n  api_key  = env(\"AIPROXY_PEER_TOKEN\")\n\n  model \"chat\" {\n    upstream_name = \"alias/peer_chat\"\n    capabilities  = [\"chat\", \"responses\"]\n    deferred_transport {\n      control_timeout     = \"5s\"\n      max_wait            = \"15m\"\n      poll_interval       = \"250ms\"\n      max_control_calls   = 4096\n      max_control_retries = 8\n    }\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "peer/chat"
      }), " chat requests use deferred transport. Responses requests to the same\nmodel still use the executor's ordinary ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/v1/responses"
      }), " route; they are not\nbackground jobs."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "full-executor-configuration",
      children: "Full Executor Configuration"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Copy this into ", (0,jsx_runtime.jsx)(_components.code, {
        children: "peer-executor.hcl"
      }), ". The upstream credentials are distinct from\nthe peer credential. The secondary provider demonstrates the required ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "local"
      }), "\nderivative receiver opt-in; this alias advertises ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "5m / 4 MiB"
      }), ", not the larger\nroot limits."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        metastring: "title=\"peer-executor.hcl\"",
        children: "listener \"http\" \"public\" {\n  address = \":8081\"\n  timeouts {\n    read_header = \"10s\"\n    idle        = \"60s\"\n    write       = \"0s\"\n  }\n}\n\nupstream_header_timeout = \"10m\"\n\nauth \"main\" {\n  mode = \"bearer_static\"\n  client \"peer-caller\" {\n    token          = env(\"AIPROXY_PEER_TOKEN\")\n    tenant         = \"peer\"\n    allowed_models = [\"alias/peer_chat\"]\n  }\n}\n\nlogging {\n  level      = \"info\"\n  access_log = true\n}\n\nmetrics {\n  token = env(\"AIPROXY_EXECUTOR_METRICS_TOKEN\")\n}\n\ndeferred {\n  max_jobs             = 64\n  max_running          = 8\n  max_admissions       = 4\n  max_entries          = 256\n  max_memory_bytes     = 268435456\n  max_request_bytes    = 1048576\n  max_result_bytes     = 8388608\n  max_work_bytes       = 67108864\n  per_owner_jobs       = 8\n  per_owner_running    = 2\n  max_control_inflight = 16\n  execution_timeout    = \"10m\"\n  retention            = \"2m\"\n  control_timeout      = \"5s\"\n  finalization_timeout = \"5s\"\n}\n\nprovider \"openai-compatible\" \"primary\" {\n  base_url = \"https://upstream.example/v1\"\n  api_key  = env(\"UPSTREAM_API_KEY\")\n  deferred_receiver {}\n\n  model \"chat\" {\n    upstream_name = \"chat-model\"\n    capabilities  = [\"chat\", \"responses\"]\n  }\n}\n\nprovider \"openai-compatible\" \"secondary\" {\n  extends = \"primary\"\n  api_key = env(\"SECONDARY_UPSTREAM_API_KEY\")\n  deferred_receiver {\n    execution_timeout = \"5m\"\n    max_result_bytes  = 4194304\n  }\n}\n\nalias \"peer_chat\" {\n  algorithm = \"round_robin\"\n  target {\n    provider = \"primary\"\n    model    = \"chat\"\n  }\n  target {\n    provider = \"secondary\"\n    model    = \"chat\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "With the environment variables set, validate each full file without dialing a peer\nor upstream:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy validate --config peer-caller.hcl\naiproxy validate --config peer-executor.hcl\naiproxy validate --config peer-model-caller.hcl\naiproxy validate --config peer-model-executor.hcl\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The repository's ", (0,jsx_runtime.jsx)(_components.code, {
        children: "go test ./internal/config -run DocumentedDeferred -count=1"
      }), "\nloads both exact full pairs and the appended compatibility/derivative snippets\nwith non-secret fixture values and checks HCL/JSON conversion, file and inline/env\nloading. See the ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/api-reference#peer-deferred-transport-v1",
        children: "wire API"
      }), "\nand ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#peer-deferred-transport-v1",
        children: "deployment, deadlines and diagnostics"
      }), "\nbefore enabling traffic."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "upstream-tls-clienthello",
      children: "Upstream TLS ClientHello"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello"
      }), " is an optional ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "provider-only"
      }), " selector for upstream HTTPS.\nThere is no root default or bypass setting. Values are strict, case-sensitive\nstrings; empty, null, wrong-type, wrong-case and unknown selections fail validation."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Selection"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Effective behavior"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: ["Omitted or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "native"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Standard Go 1.27 TLS (the default)"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chrome"
            }), " or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "chrome-133"
            })]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Bundled Chrome 133; canonical selector ", (0,jsx_runtime.jsx)(_components.code, {
              children: "chrome-133"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "safari"
            }), " or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "safari-16.0"
            })]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Bundled Safari 16.0; canonical selector ", (0,jsx_runtime.jsx)(_components.code, {
              children: "safari-16.0"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "custom"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Supplied static JSON recipe; requires ", (0,jsx_runtime.jsx)(_components.code, {
              children: "tls_client_hello_spec_file"
            })]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Browser presets come from pinned uTLS ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "v1.8.2"
      }), ". Aliases are frozen bundled\nmappings, not live/latest browser downloads. Firefox, Bun, generic BoringSSL/iOS,\nrandomized, draft and PSK selectors are not supported. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "custom"
      }), " never detects Bun."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"openai-compatible\" \"profiled\" {\n  base_url         = \"https://api.example.test/v1\"\n  api_key          = env(\"UPSTREAM_API_KEY\")\n  tls_client_hello = \"chrome-133\"\n\n  model \"example-model\" {}\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The effective profile applies to HTTPS inference, each alias target, provider health\nprobes and CLI upstream model discovery. Plain-HTTP requests perform no upstream TLS;\nan absolute HTTPS health URL still uses the profile even when ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), " is HTTP.\nStatic derived providers inherit both TLS attributes/profile with the base endpoint\nand reject all local declarations. Disabled profiles are validated too."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Profiles change ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "ClientHello only"
      }), ", not HTTP headers, HTTP/2 settings, the TCP stack,\ninbound listener transport or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_agent"
      }), ". Certificate and hostname verification\nremain enabled. Bundled compatibility advertisements do not enable weak negotiation:\nnon-native connections require TLS 1.2/1.3 with strong ECDHE/AEAD or TLS 1.3 AEAD,\nreject SHA-1/weak server selections, and never rewrite, fall back or cycle profiles.\nResumed/PSK handshakes, renegotiation and selected ALPS (even empty settings) are\nunsupported. A ClientHello match does not guarantee provider, WAF or bot acceptance."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "custom-specification-files",
      children: "Custom Specification Files"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "custom"
      }), " requires a nonempty ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello_spec_file"
      }), ". Declaring that attribute\nwith any non-custom selector is invalid ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "even when the file value is empty"
      }), ".\nCustom recipes are file-based only; inline recipe fields/uploads are unsupported."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"openai-compatible\" \"custom-profile\" {\n  base_url                  = \"https://api.example.test/v1\"\n  api_key                   = env(\"UPSTREAM_API_KEY\")\n  tls_client_hello           = \"custom\"\n  tls_client_hello_spec_file = \"tls/custom-clienthello.json\"\n\n  model \"example-model\" {}\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["If the config is ", (0,jsx_runtime.jsx)(_components.code, {
        children: "config/config.hcl"
      }), ", create ", (0,jsx_runtime.jsx)(_components.code, {
        children: "config/tls/custom-clienthello.json"
      }), ".\nRelative paths resolve against the config file's absolute ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "lexical directory"
      }), ",\nnot the shell cwd. Inline ", (0,jsx_runtime.jsx)(_components.code, {
        children: "AIPROXY_CONFIG"
      }), " or source-less loading requires an\nabsolute host-local reference. References are at most 4096 UTF-8 bytes without\ncontrol characters/NUL. Regular-file symlinks are allowed; directories, devices\nand FIFOs are rejected. There is no tilde, shell or URL expansion, and recipe JSON\ndoes not expand ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env()"
      }), "; config-level ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env()"
      }), " inlining remains available."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Recipes are read, strictly validated and cached immutably during candidate\nconfiguration validation. Requests and runtime validators never reopen files;\neach connection gets fresh mutable extension objects, randomness, session bytes\nand real key shares, with SNI from the actual upstream hostname. Replacing a file\nalone does not change a running server. Successful ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " reload picks up new\ncontents; a failed reload preserves old cached bytes, even at the same path.\nIn-place CLI edits preserve reference and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env()"
      }), " spelling. HCL/JSON conversion\nemits resolved absolute references without copying recipes: this preserves\nhost-local usability, not portability across machines or container mounts."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The following ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "illustrative"
      }), ", unmeasured recipe is a minimal TLS 1.2/1.3 example,\nnot a Chrome or Bun fingerprint. Save it at the path above. Capture metadata is\nseparate and must not be added to this object."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-json",
        children: "{\n  \"cipher_suites\": [\"TLS_AES_128_GCM_SHA256\", \"TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256\"],\n  \"compression_methods\": [\"NULL\"],\n  \"extensions\": [\n    { \"name\": \"server_name\" },\n    { \"name\": \"supported_groups\", \"named_group_list\": [\"x25519\", \"secp256r1\"] },\n    { \"name\": \"signature_algorithms\", \"supported_signature_algorithms\": [\"rsa_pss_rsae_sha256\"] },\n    { \"name\": \"application_layer_protocol_negotiation\", \"protocol_name_list\": [\"h2\", \"http/1.1\"] },\n    { \"name\": \"supported_versions\", \"versions\": [\"TLS 1.3\", \"TLS 1.2\"] },\n    { \"name\": \"key_share\", \"client_shares\": [{ \"group\": \"x25519\" }] }\n  ],\n  \"min_vers\": 771,\n  \"max_vers\": 772\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["This stable v1 subset uses native-shaped uTLS JSON, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "not"
      }), " ordinary Go struct\nmarshaling. Files contain one valid UTF-8 object, at most ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "64 KiB"
      }), ", depth ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "8"
      }), "\nand ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "4096 tokens"
      }), ". Duplicate keys, unknown fields, null required collections,\ntrailing data and contradictory settings are rejected. Array order is significant."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Field"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Accepted v1 contents"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "cipher_suites"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["1–64 unique ordered names from the list below; at most one ", (0,jsx_runtime.jsx)(_components.code, {
              children: "GREASE"
            }), "; require a safe usable suite for every enabled version"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "compression_methods"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Exactly ", (0,jsx_runtime.jsx)(_components.code, {
              children: "[\"NULL\"]"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "extensions"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["1–32 ordered extension objects; unique names except up to two ", (0,jsx_runtime.jsx)(_components.code, {
              children: "GREASE"
            }), " entries; require ", (0,jsx_runtime.jsx)(_components.code, {
              children: "server_name"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "supported_groups"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "signature_algorithms"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "min_vers"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "max_vers"
            })]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Optional numeric bounds ", (0,jsx_runtime.jsx)(_components.code, {
              children: "771"
            }), " (TLS 1.2) or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "772"
            }), " (TLS 1.3), matching ", (0,jsx_runtime.jsx)(_components.code, {
              children: "supported_versions"
            }), "; without that extension, only TLS 1.2"]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Safe suite names are:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-text",
        children: "TLS_AES_128_GCM_SHA256\nTLS_AES_256_GCM_SHA384\nTLS_CHACHA20_POLY1305_SHA256\nTLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256\nTLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256\nTLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384\nTLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384\nTLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256\nTLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "GREASE"
      }), " and these eight legacy names are accepted as ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "advertisements only"
      }), ";\nthey cannot satisfy the safe-suite requirement or be negotiated:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-text",
        children: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA\nTLS_ECDHE_RSA_WITH_AES_128_CBC_SHA\nTLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA\nTLS_ECDHE_RSA_WITH_AES_256_CBC_SHA\nTLS_RSA_WITH_AES_128_GCM_SHA256\nTLS_RSA_WITH_AES_256_GCM_SHA384\nTLS_RSA_WITH_AES_128_CBC_SHA\nTLS_RSA_WITH_AES_256_CBC_SHA\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Extension payloads are restricted as follows; no other payload fields are accepted:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.th, {
            children: ["Extension ", (0,jsx_runtime.jsx)(_components.code, {
              children: "name"
            })]
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Payload"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "GREASE"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "server_name"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "extended_master_secret"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "renegotiation_info"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "session_ticket"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "status_request"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "signed_certificate_timestamp"
            })]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "name"
            }), " only; SNI is generated, tickets are empty first-flight advertisements"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "supported_groups"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "named_group_list"
            }), ": 1–16 unique entries from ", (0,jsx_runtime.jsx)(_components.code, {
              children: "X25519MLKEM768"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "x25519"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "secp256r1"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "secp384r1"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "secp521r1"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "GREASE"
            }), "; require a real group and a classical group for TLS 1.2"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "ec_point_formats"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "ec_point_format_list"
            }), ": exactly ", (0,jsx_runtime.jsx)(_components.code, {
              children: "[\"uncompressed\"]"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "signature_algorithms"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "supported_signature_algorithms"
            }), ": 1–32 unique names listed below; require a usable modern signature compatible with the enabled suites/versions"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "application_layer_protocol_negotiation"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "protocol_name_list"
            }), ": ", (0,jsx_runtime.jsx)(_components.code, {
              children: "[\"http/1.1\"]"
            }), " or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "h2"
            }), " plus ", (0,jsx_runtime.jsx)(_components.code, {
              children: "http/1.1"
            }), " in either order; no empty, duplicate, unknown or h2-only list; the extension may be absent"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "supported_versions"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "versions"
            }), ": 1–3 unique entries from ", (0,jsx_runtime.jsx)(_components.code, {
              children: "TLS 1.2"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "TLS 1.3"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "GREASE"
            }), ", with at least one real version"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "key_share"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "client_shares"
            }), ": 1–3 unique shares in ", (0,jsx_runtime.jsx)(_components.code, {
              children: "supported_groups"
            }), " order; at most one real classical share, with ", (0,jsx_runtime.jsx)(_components.code, {
              children: "x25519"
            }), " fallback required for ", (0,jsx_runtime.jsx)(_components.code, {
              children: "X25519MLKEM768"
            }), "; required for TLS 1.3"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "psk_key_exchange_modes"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "ke_modes"
            }), ": exactly ", (0,jsx_runtime.jsx)(_components.code, {
              children: "[\"psk_dhe_ke\"]"
            }), ", TLS 1.3 only; no imported PSK identities or sessions"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "padding"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "len"
            }), ": integer 0–4096; zero means BoringPaddingStyle rather than a fixed padding count"]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Each share is ", (0,jsx_runtime.jsx)(_components.code, {
        children: "{ \"group\": \"<allowed-group>\" }"
      }), ": omit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "key_exchange"
      }), " for every\nreal group. GREASE may use dummy ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"key_exchange\": [0]"
      }), " only. The private\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X25519MLKEM768"
      }), " JSON symbol adapter generates fresh keys without changing uTLS\ndictionaries. MLKEM-only and multiple distinct classical shares are unsupported."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Modern signature names are ", (0,jsx_runtime.jsx)(_components.code, {
        children: "ecdsa_secp256r1_sha256"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "ecdsa_secp384r1_sha384"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "ecdsa_secp521r1_sha512"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "rsa_pss_rsae_sha256"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "rsa_pss_rsae_sha384"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "rsa_pss_rsae_sha512"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "rsa_pkcs1_sha256"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "rsa_pkcs1_sha384"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "rsa_pkcs1_sha512"
      }), "\nand ", (0,jsx_runtime.jsx)(_components.code, {
        children: "ed25519"
      }), ". RSA-PKCS1 signature selection is TLS 1.2 only. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "rsa_pkcs1_sha1"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "ecdsa_sha1"
      }), "\nmay be advertised but are rejected if selected. DSA, SHA-224, Ed448 and unknown\nschemes are unsupported."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["A synthetic-SNI dry run must fit a ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "16 KiB"
      }), " ClientHello; its generated crypto is\ndiscarded. Custom raw/generic extensions, ECH, ALPS, certificate compression, NPN,\nQUIC, cookies, delegated credentials, early data and resumption-specific fields\nare rejected, not approximated. Empty ticket/PSK-mode advertisements do not enable\nresumption. Bundled profiles may advertise compatibility fields outside the custom\nsubset; selected ALPS remains explicitly unsupported."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Database/API/web UI providers allow ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "built-ins only"
      }), ", including effective\ninheritance. They reject custom/file/inline data before commit for all admin roles;\na managed derivative cannot extend a static custom base. Views expose only\nselector/effective/inherited/static-only metadata, never recipe paths/content.\nSee the ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/api-reference#provider-tls-clienthello-settings",
        children: "admin contract"
      }), " and\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#local-bun-clienthello-capture",
        children: "capture, deployment and proxy limits"
      }), "\nfor the measured Bun fixture and supported transport paths."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "upstream-user-agent",
      children: "Upstream User-Agent"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Every upstream request sends ", (0,jsx_runtime.jsx)(_components.code, {
        children: "User-Agent: aiproxy/<version>"
      }), " unless overridden. You can set a default for all providers at the root, override it per provider with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_agent"
      }), ", or forward the inbound caller ", (0,jsx_runtime.jsx)(_components.code, {
        children: "User-Agent"
      }), " with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_user_agent"
      }), ":"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "user_agent = \"my-proxy/1.0\"\n\nprovider \"openai\" \"openai\" {\n  user_agent = \"openai-specific/2.0\"\n}\n\nprovider \"openai\" \"passthrough\" {\n  forward_user_agent = true\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Precedence is provider ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_agent"
      }), ", then root ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_agent"
      }), ", then the ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy/<version>"
      }), " default. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_user_agent"
      }), " applies when a provider has no explicit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_agent"
      }), " of its own and is effective when set at either the provider or root level; there is no per-provider opt-out when the root enables it, so set the flag per provider instead in that case. Forwarded values must be non-empty, at most 256 characters, and printable ASCII, otherwise the default is sent. The inbound caller ", (0,jsx_runtime.jsx)(_components.code, {
        children: "User-Agent"
      }), " is never forwarded unless forwarding is enabled. Root and provider user-agent changes apply on a successful ", (0,jsx_runtime.jsx)(_components.code, {
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
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "deferred_transport"
        }), " opts a static ", (0,jsx_runtime.jsx)(_components.code, {
          children: "openai-compatible"
        }), " chat model into\n", (0,jsx_runtime.jsx)(_components.a, {
          href: "#caller-policy",
          children: "peer caller transport"
        }), "; ", (0,jsx_runtime.jsx)(_components.code, {
          children: "deferred_receiver"
        }), " is provider-only"]
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
          children: "messages"
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
      }), " serves ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), " and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "messages"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " serves ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), ")."]
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
      children: ["Set ", (0,jsx_runtime.jsx)(_components.code, {
        children: "$AIPROXY_CONFIG"
      }), " to inline the whole HCL document and skip the config file:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "export AIPROXY_CONFIG='listener \"http\" \"public\" { address = \":8080\" }\nauth \"main\" { mode = \"none\" }\nprovider \"openai\" \"openai\" {\n  api_key = env(\"OPENAI_API_KEY\")\n  model \"gpt-4o-mini\" {}\n}'\naiproxy validate\naiproxy serve\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["An explicit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "--config"
      }), " overrides ", (0,jsx_runtime.jsx)(_components.code, {
        children: "$AIPROXY_CONFIG"
      }), ". ", (0,jsx_runtime.jsx)(_components.code, {
        children: "serve -d"
      }), " and the\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "configure"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "login"
      }), " file workflows require a file. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " re-reads\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "$AIPROXY_CONFIG"
      }), " when the server was started from it."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["HCL blocks must be separated by newlines, so a multi-block document collapsed\nto a single line (for example by a single-line input field) is rejected. When\nnewlines cannot survive the transport, use the JSON form instead: it is fully\nsingle-line safe. Block labels become nested objects, repeatable label-less\nblocks (such as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "target"
      }), ") accept a single object or an array, and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env(\"VAR\")"
      }), "\nis written bare as the value:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "export AIPROXY_CONFIG='{\"listener\":{\"http\":{\"public\":{\"address\":\":8080\"}}},\"auth\":{\"main\":{\"mode\":\"none\"}},\"provider\":{\"openai\":{\"openai\":{\"api_key\":env(\"OPENAI_API_KEY\"),\"model\":{\"gpt-4o-mini\":{}}}}}}'\naiproxy validate\naiproxy serve\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Convert an existing config between the two forms with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy convert"
      }), ".\nThe direction is detected automatically, the source defaults to the usual\nconfig context (", (0,jsx_runtime.jsx)(_components.code, {
        children: "--config"
      }), ", otherwise ", (0,jsx_runtime.jsx)(_components.code, {
        children: "$AIPROXY_CONFIG"
      }), ", otherwise the default\npath), and the target defaults to a file in the current working directory.\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "env(\"VAR\")"
      }), " calls are resolved at conversion time, so the referenced\nvariables must be set, and the output is validated before writing:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy convert ./config.json --config /etc/aiproxy/config.hcl\naiproxy convert --compact - --config /etc/aiproxy/config.hcl\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Conversion materializes environment values, including secrets, in the output;\nthis also applies to stdout (", (0,jsx_runtime.jsx)(_components.code, {
        children: "-"
      }), "). On Linux and macOS, file output is staged in the destination\ndirectory and published atomically with exact ", (0,jsx_runtime.jsx)(_components.code, {
        children: "0600"
      }), " permissions. Without\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "--force"
      }), ", publication refuses an existing destination even if another process\ncreates it during conversion. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "--force"
      }), " atomically replaces an existing regular\nfile and resets its permissions to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "0600"
      }), ". Symlinks (live or dangling) and\nnon-regular destinations are rejected. Missing parent directories are created\nwith mode ", (0,jsx_runtime.jsx)(_components.code, {
        children: "0700"
      }), " (subject to umask)."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Failed validation, staging, or publication leaves any old destination intact.\nNon-force publication requires filesystem hard-link support and fails if it is\nunavailable. A failure after publication (temporary-file cleanup, directory\nsync, or writing the confirmation) can leave the complete new file in place;\nstdout write failures also return an error."
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Windows file conversion is unsupported and fails before creating files or\ndirectories, because this writer cannot guarantee owner-only permissions and\natomic replacement there. Stdout conversion still works; save its output through\na suitably secured external tool."
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
    }), "\n", (0,jsx_runtime.jsxs)(_components.h3, {
      id: "credential_ref-github-copilot",
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref"
      }), " (GitHub Copilot)"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref"
      }), " references a structured sidecar written by ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy login github-copilot"
      }), ":"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "credential_ref {\n  name = \"copilot-main\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The sidecar lives at ", (0,jsx_runtime.jsx)(_components.code, {
        children: "<secrets-dir>/copilot-<name>.json"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "0600"
      }), "). ", (0,jsx_runtime.jsx)(_components.code, {
        children: "path"
      }), " is\noptional and defaults to the shared secrets path. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "configure provider --credential/--credential-path"
      }), " writes this block without OAuth networking or\ntoken display."]
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
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "empty, wrong-type, wrong-case or unknown TLS selectors, root/local derived TLS\ndeclarations, any spec-file declaration on non-custom, or missing/unreadable/\ninvalid custom recipes; disabled providers are subject to these TLS checks too"
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
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "github-copilot"
        }), " providers with ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key"
        }), "/", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key_ref"
        }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "credential_ref"
        }), "\non any other provider type; enabled Copilot providers without a resolvable\nsidecar credential"]
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
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["aliases without any targets (either ", (0,jsx_runtime.jsx)(_components.code, {
          children: "target"
        }), " blocks or the ", (0,jsx_runtime.jsx)(_components.code, {
          children: "providers"
        }), "/", (0,jsx_runtime.jsx)(_components.code, {
          children: "model"
        }), " shorthand)"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["aliases combining ", (0,jsx_runtime.jsx)(_components.code, {
          children: "target"
        }), " blocks with the ", (0,jsx_runtime.jsx)(_components.code, {
          children: "providers"
        }), "/", (0,jsx_runtime.jsx)(_components.code, {
          children: "model"
        }), " shorthand, or shorthand with only one of ", (0,jsx_runtime.jsx)(_components.code, {
          children: "providers"
        }), "/", (0,jsx_runtime.jsx)(_components.code, {
          children: "model"
        }), " set"]
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
      }), "\ncommand reads that file to authenticate. If a reload drops a previously\ndeclared token, the carried-over secret is published to the file before the\nnew runtime activates, so tokenless discovery keeps working; a persistence\nfailure rejects the reload and keeps the old runtime unchanged. The dashboard command is local-only: it\nconnects over loopback plain HTTP and refuses concrete non-loopback listener\nhosts. HTTPS and remote dashboard URLs are not supported by the current\nconfiguration model."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "web_ui",
      children: (0,jsx_runtime.jsx)(_components.code, {
        children: "web_ui"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "web_ui {\n  enabled = true\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Serves the embedded React dashboard at ", (0,jsx_runtime.jsx)(_components.code, {
        children: "GET /"
      }), " (with client-side\nroutes such as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/providers"
      }), " and hashed assets under\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "/assets/"
      }), "). The block is optional and a bare ", (0,jsx_runtime.jsx)(_components.code, {
        children: "web_ui {}"
      }), " enables\nit; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled = false"
      }), " keeps the UI closed. The UI never claims API\nsurfaces (", (0,jsx_runtime.jsx)(_components.code, {
        children: "/v1/*"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/healthz"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/readyz"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/metrics"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/_internal/*"
      }), ")\nor the reserved ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/dashboard"
      }), " path, and extensionless client-side routes\nfall back to the app shell only for browser (", (0,jsx_runtime.jsx)(_components.code, {
        children: "text/html"
      }), ") navigations,\nso API clients keep receiving JSON 404s. This gate is independent of the\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "dashboard"
      }), " block above: the static UI loads without it, but the live\nsnapshot/logs/payloads/blocks views still call the bearer-gated\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "/_internal/dashboard/*"
      }), " APIs. When ", (0,jsx_runtime.jsx)(_components.code, {
        children: "multi_tenancy"
      }), " is disabled, paste the\ndashboard token on the UI's Token page. When it is enabled, the browser uses\nthe signed-in system administrator's session and ignores any saved dashboard\ntoken. The TUI still uses its dashboard token in either mode. Toggling ", (0,jsx_runtime.jsx)(_components.code, {
        children: "web_ui"
      }), "\napplies on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " without restart."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "ingress_guardrails",
      children: (0,jsx_runtime.jsx)(_components.code, {
        children: "ingress_guardrails"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "ingress_guardrails {\n  enabled        = true\n  mode           = \"block\"\n  max_text_bytes = 65536\n  max_strings    = 512\n\n  exceptions_file    = \"/etc/aiproxy/guardrail-exceptions.json\"\n  redact_placeholder = \"REDACTED\"\n\n  quarantine {\n    enabled           = true\n    max_entries       = 128\n    ttl               = \"15m\"\n    max_snippet_bytes = 512\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Opt-in secret scanning of inbound ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/chat/completions"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/responses"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/messages"
      }), " requests using the embedded Gitleaks rule set. Absent or\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled = false"
      }), " preserves existing behavior. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "mode"
      }), " is ", (0,jsx_runtime.jsx)(_components.code, {
        children: "block"
      }), " (default,\nrejects with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "400 secret_blocked"
      }), " / ", (0,jsx_runtime.jsx)(_components.code, {
        children: "400 scan_incomplete"
      }), " and zero upstream\nI/O) or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "audit"
      }), " (forwards and records\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy_guardrail_scans_total{operation,mode,outcome}"
      }), "). A blocked flagged\nrequest carries ", (0,jsx_runtime.jsx)(_components.code, {
        children: "block_id"
      }), " in the error body. Scanned text is the\nJSON-decoded message content, tool arguments (plus one JSON-decoded level),\ntool results, responses instructions/input, and messages system/content/tool\npayloads; images, audio, embeddings,\nattachments, encoded blobs, and response/SSE output are out of scope and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "gitleaks:allow"
      }), " cannot suppress scans. Bounds are 1024..32MiB text bytes\nand 1..16384 strings and default to 65536 text bytes\nand 512 strings (zeros select defaults); over-limit or canceled scans are\nvisible ", (0,jsx_runtime.jsx)(_components.code, {
        children: "incomplete"
      }), " outcomes. While enabled, covered-operation request\nbodies are omitted from payload-log entries. Policy changes apply on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), "\nwith atomic rollback on failure."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The nested ", (0,jsx_runtime.jsx)(_components.code, {
        children: "quarantine"
      }), " block is optional and disabled by default. When\nenabled, matched snippets (", (0,jsx_runtime.jsx)(_components.code, {
        children: "secret"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "match"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "line"
      }), ", truncated to\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "max_snippet_bytes"
      }), ", default 512) are kept in process memory (bounded by\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "max_entries"
      }), ", default 128, and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "ttl"
      }), ", default ", (0,jsx_runtime.jsx)(_components.code, {
        children: "15m"
      }), ") so an operator can\ntriage false positives by ", (0,jsx_runtime.jsx)(_components.code, {
        children: "block_id"
      }), " through the dashboard-gated\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "GET /_internal/dashboard/blocks"
      }), " (metadata only) and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "GET /_internal/dashboard/blocks/<block_id>"
      }), " (full capture, take-once)\nendpoints, or the ", (0,jsx_runtime.jsx)(_components.code, {
        children: "4:Blocks"
      }), " tab in ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy dashboard"
      }), ". Dashboard token\nholders can read live matched secrets while quarantine is on; state is\nprocess-local, lost on restart, and rebuilt empty whenever its config\nchanges on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Triage decisions outlive quarantine: open a block and press ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[a]llow"
      }), "\n(non-secret), ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[s]anitize"
      }), " (replace with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "redact_placeholder"
      }), " upstream), or\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "[d]eny"
      }), " (keep blocking). The decision fans out to every matched secret in\nthe block via ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /_internal/dashboard/blocks/<block_id>/decision"
      }), " and is\nstored by SHA-256 fingerprint in ", (0,jsx_runtime.jsx)(_components.code, {
        children: "exceptions_file"
      }), " (default\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "$XDG_CONFIG_HOME/aiproxy/guardrail-exceptions.json"
      }), ", shown by\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy paths"
      }), "; fingerprints and actions only, never secret text).\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "redact_placeholder"
      }), " defaults to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "REDACTED"
      }), " and is rejected at startup when\nflagged as a secret itself."]
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