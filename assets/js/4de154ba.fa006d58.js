"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[81],{

/***/ 4852
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_operations_md_4de_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-operations-md-4de.json
const site_docs_operations_md_4de_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"operations","title":"Operations","description":"This page covers the commands and operational behavior that matter most for local development and production deployment.","source":"@site/docs/operations.md","sourceDirName":".","slug":"/operations","permalink":"/docs/operations","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":6,"frontMatter":{"sidebar_position":6},"sidebar":"docsSidebar","previous":{"title":"API Reference","permalink":"/docs/api-reference"},"next":{"title":"Deployment","permalink":"/docs/deployment"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/operations.md


const frontMatter = {
	sidebar_position: 6
};
const contentTitle = 'Operations';

const assets = {

};



const toc = [{
  "value": "Build And Run",
  "id": "build-and-run",
  "level": 2
}, {
  "value": "Configure Wizard",
  "id": "configure-wizard",
  "level": 2
}, {
  "value": "Docker",
  "id": "docker",
  "level": 2
}, {
  "value": "Tests",
  "id": "tests",
  "level": 2
}, {
  "value": "Reload Behavior",
  "id": "reload-behavior",
  "level": 2
}, {
  "value": "Metrics And Health",
  "id": "metrics-and-health",
  "level": 2
}, {
  "value": "Shared Provider Health",
  "id": "shared-provider-health",
  "level": 2
}, {
  "value": "Dashboard Transport Security",
  "id": "dashboard-transport-security",
  "level": 2
}, {
  "value": "Security Defaults",
  "id": "security-defaults",
  "level": 2
}, {
  "value": "Logging",
  "id": "logging",
  "level": 2
}, {
  "value": "Secret Handling",
  "id": "secret-handling",
  "level": 2
}, {
  "value": "Production Checklist",
  "id": "production-checklist",
  "level": 2
}];
function _createMdxContent(props) {
  const _components = {
    code: "code",
    h1: "h1",
    h2: "h2",
    header: "header",
    li: "li",
    p: "p",
    pre: "pre",
    ul: "ul",
    ...(0,lib/* useMDXComponents */.R)(),
    ...props.components
  };
  return (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
    children: [(0,jsx_runtime.jsx)(_components.header, {
      children: (0,jsx_runtime.jsx)(_components.h1, {
        id: "operations",
        children: "Operations"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This page covers the commands and operational behavior that matter most for local development and production deployment."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "build-and-run",
      children: "Build And Run"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Common commands:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "make build\nmake run CONFIG=path/to/config.hcl\nmake validate CONFIG=path/to/config.hcl\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Direct CLI usage:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy serve\naiproxy validate\naiproxy login github-copilot --client-id YOUR_GITHUB_OAUTH_CLIENT_ID --credential copilot-main\naiproxy paths\naiproxy examples\naiproxy configure\naiproxy configure provider\naiproxy serve --config /etc/aiproxy/config.hcl\naiproxy validate --config /etc/aiproxy/config.hcl\naiproxy version\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Without ", (0,jsx_runtime.jsx)(_components.code, {
        children: "--config"
      }), ", the CLI reads ", (0,jsx_runtime.jsx)(_components.code, {
        children: "$XDG_CONFIG_HOME/aiproxy/config.hcl"
      }), ", falling back to\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "~/.config/aiproxy/config.hcl"
      }), " when ", (0,jsx_runtime.jsx)(_components.code, {
        children: "XDG_CONFIG_HOME"
      }), " is unset. Set ", (0,jsx_runtime.jsx)(_components.code, {
        children: "$AIPROXY_CONFIG"
      }), "\nto inline HCL to skip the config file (explicit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "--config"
      }), " overrides it; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "serve -d"
      }), "\nand ", (0,jsx_runtime.jsx)(_components.code, {
        children: "configure"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "login"
      }), " file workflows require a file)."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Foreground ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy serve"
      }), " is supported across the advertised release targets.\nLinux additionally supports ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy serve -d"
      }), " and the ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy status"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy stop"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy restart"
      }), " daemon lifecycle commands. On non-Linux\nplatforms those daemon lifecycle commands return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "daemon lifecycle is unsupported on this platform"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "When running locally with env-based secrets, load your environment before invoking the binary:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "set -a; . ./.env; set +a\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "configure-wizard",
      children: "Configure Wizard"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " includes an interactive config editor for the top-level HCL blocks and\nthe provider secrets JSON file."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Interactive entrypoints:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy configure\naiproxy configure provider\naiproxy configure auth\naiproxy configure alias\naiproxy configure listener\naiproxy configure upstream\naiproxy configure logging\naiproxy configure provider-health\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The root ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy configure"
      }), " command shows a block selector. The block-specific\nsubcommands can also be used directly."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Supported workflows:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["create or update ", (0,jsx_runtime.jsx)(_components.code, {
          children: "listener"
        }), ", root ", (0,jsx_runtime.jsx)(_components.code, {
          children: "upstream_header_timeout"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "auth"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "alias"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "logging"
        }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider_health"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["update provider secrets when using ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key_ref"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["delete existing blocks with ", (0,jsx_runtime.jsx)(_components.code, {
          children: "--delete"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For scripted environments, use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "--non-interactive"
      }), " on block subcommands."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Provider example:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy configure provider \\\n  --config /etc/aiproxy/config.hcl \\\n  --non-interactive \\\n  --name backup \\\n  --type openai-compatible \\\n  --display-name \"Backup provider\" \\\n  --base-url https://llm.internal/v1 \\\n  --upstream-header-timeout 180s \\\n  --secrets-path /etc/aiproxy/keys.json \\\n  --secrets-key localai \\\n  --api-key \"$LOCALAI_API_KEY\" \\\n  --model qwen3-32b=qwen/qwen3-32b \\\n  --model-capabilities qwen3-32b=chat,responses\n\naiproxy configure provider \\\n  --config /etc/aiproxy/config.hcl \\\n  --non-interactive \\\n  --name backup-2 \\\n  --type openai-compatible \\\n  --extends backup \\\n  --display-name \"Backup provider 2\" \\\n  --secrets-key backup-2 \\\n  --api-key \"$BACKUP_2_API_KEY\"\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["OpenCode providers use explicit types with per-model protocols (", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "messages"
      }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), "; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " is Zen-only). Base URLs are\nomitted to use the service defaults; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "--base-url"
      }), " remains available as a\ntransport-only override."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy configure provider \\\n  --config /etc/aiproxy/config.hcl \\\n  --non-interactive \\\n  --name zen \\\n  --type opencode-zen \\\n  --api-key-env OPENCODE_ZEN_API_KEY \\\n  --model glm-5.3 \\\n  --model-protocol glm-5.3=chat\n\naiproxy configure provider \\\n  --config /etc/aiproxy/config.hcl \\\n  --non-interactive \\\n  --name go \\\n  --type opencode-go \\\n  --api-key-env OPENCODE_GO_API_KEY \\\n  --model minimax-m3 \\\n  --model-protocol minimax-m3=messages\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "GitHub Copilot providers reference a saved device-flow login (no API-key\nflags, no OAuth networking here, no token display):"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy login github-copilot --client-id YOUR_GITHUB_OAUTH_CLIENT_ID --credential copilot-main\n\naiproxy configure provider \\\n  --config /etc/aiproxy/config.hcl \\\n  --non-interactive \\\n  --name copilot \\\n  --type github-copilot \\\n  --credential copilot-main \\\n  --model gpt-5.4-nano\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "login"
      }), " prints the verification URI and user code, then writes\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "<secrets-dir>/copilot-<name>.json"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "0600"
      }), "). It never edits HCL or signals a\nserver: restart or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " to activate, and re-run the same ", (0,jsx_runtime.jsx)(_components.code, {
        children: "login"
      }), " + reload\non upstream ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), ", revocation, or expiry. Use your own public OAuth\nclient ID; never reuse another application's client ID."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Root upstream timeout example:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy configure upstream \\\n  --config /etc/aiproxy/config.hcl \\\n  --non-interactive \\\n  --upstream-header-timeout 120s\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Alias example:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy configure alias \\\n  --config /etc/aiproxy/config.hcl \\\n  --non-interactive \\\n  --name chat_default \\\n  --algorithm round_robin \\\n  --target primary/gpt-4o-mini \\\n  --target backup/qwen3-32b\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Auth example:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy configure auth \\\n  --config /etc/aiproxy/config.hcl \\\n  --non-interactive \\\n  --name main \\\n  --mode bearer_static \\\n  --rate-limit-rpm 120 \\\n  --rate-limit-burst 120 \\\n  --client internal-app \\\n  --client-token-env internal-app=AIPROXY_CLIENT_TOKEN \\\n  --client-tenant internal-app=internal \\\n  --client-allowed-models internal-app=alias/chat_default,openai/gpt-4o-mini\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Delete examples:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy configure provider --config /etc/aiproxy/config.hcl --delete --name backup\naiproxy configure alias --config /etc/aiproxy/config.hcl --delete --name chat_default\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "docker",
      children: "Docker"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "make docker-build\nmake docker-run CONFIG=path/to/config.hcl\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The image mounts the config file and runs the same CLI entrypoint."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["In containerized deployments, mount the config file read-only and inject secrets through environment variables or the key file used by ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "tests",
      children: "Tests"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "make vet\nmake test\nmake test-race\nmake docs-contract\nmake cover\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The standard local sanity check is:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "make vet test\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "There is no separate typecheck target. A successful Go build is the typecheck."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Documentation-only pull requests run ", (0,jsx_runtime.jsx)(_components.code, {
        children: "make docs-contract"
      }), " through the ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Docs Contract"
      }), " workflow. Website pull requests also run ", (0,jsx_runtime.jsx)(_components.code, {
        children: "pnpm typecheck"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "pnpm build"
      }), " from the ", (0,jsx_runtime.jsx)(_components.code, {
        children: "website"
      }), " directory."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "reload-behavior",
      children: "Reload Behavior"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " supports live config reload on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " for runtime state such as:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "auth configuration"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "provider and model inventory"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "root and provider upstream header timeouts"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "alias routing state"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "access-log enablement"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "metrics configuration"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "provider-health configuration"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "metrics-backed inventory state"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "If rate-limit settings are unchanged, reload preserves existing limiter buckets.\nChanging rate-limit settings creates a fresh limiter and resets bucket state."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Alias cooldown deadlines survive reload only for fingerprint-unchanged targets\n(resolved ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), ", credential, upstream model, protocol); removed or changed\ntargets are dropped, and failed reloads leave state untouched."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "These changes still require a restart:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "listener address changes"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "listener timeout changes"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "logging level changes"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "enabling the dashboard after startup"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Use reload for routing and auth changes, not for socket-level listener changes."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "metrics-and-health",
      children: "Metrics And Health"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The proxy exposes Prometheus metrics at ", (0,jsx_runtime.jsx)(_components.code, {
        children: "GET /metrics"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Coverage includes:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "inbound request counts and latency"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "streaming counts and duration"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "provider selection counts"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "alias retry counts"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "alias in-flight gauges"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "provider health state"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "readiness state and reason"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "upstream request counts, latency, and response sizes"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "provider health backend error counts"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "provider health fallback counts by operation and reason"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "/metrics"
      }), " requires a dedicated bearer token declared in a ", (0,jsx_runtime.jsx)(_components.code, {
        children: "metrics"
      }), " block:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "metrics {\n  token = env(\"AIPROXY_METRICS_TOKEN\")\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "GET /metrics"
      }), " without a valid ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization: Bearer <metrics token>"
      }), " header\nreturns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), ". The metrics token is checked independently of API auth client\ntokens; API clients cannot scrape ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/metrics"
      }), " with their own credentials. An\nempty or missing token is rejected at config validation."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Transient transport failures, upstream request errors, and upstream ", (0,jsx_runtime.jsx)(_components.code, {
        children: "5xx"
      }), " responses can mark a provider unhealthy for routing and readiness decisions. Configured retryable ", (0,jsx_runtime.jsx)(_components.code, {
        children: "4xx"
      }), " statuses can trigger alias failover but do not mark providers unhealthy."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Alias upstream retry advice (", (0,jsx_runtime.jsx)(_components.code, {
        children: "retry-after-ms"
      }), ", else ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Retry-After"
      }), ") is tracked\nseparately from provider health as process-local per-target cooldown deadlines.\nIt is never shared across processes or via Redis, never marks providers\nunhealthy, and leaves skipped targets out of upstream attribution while the\nclient-facing ", (0,jsx_runtime.jsx)(_components.code, {
        children: "429"
      }), " stays visible in HTTP accounting and metrics."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This health state is shared across requests within the same process."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "shared-provider-health",
      children: "Shared Provider Health"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Without extra config, provider health is in-process only."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["You can optionally configure Redis-backed shared health state with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "provider_health"
      }), " so multiple instances can observe the same transient provider status."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider_health {\n  redis_url  = env(\"AIPROXY_REDIS_URL\")\n  key_prefix = \"aiproxy:provider-health\"\n  cooldown   = \"30s\"\n  cache_ttl  = \"30s\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "cache_ttl"
      }), " (default 30s) bounds how long a stale in-process cache entry is\nreused for routing and readiness when the Redis backend becomes unreadable.\nWhen a Redis health read fails, routing, readiness, and dashboard snapshots fall\nback to the bounded in-process cache and fail open only when no fresh cache\nentry exists; both the backend error and the fallback reason are recorded as\nPrometheus metrics so degraded mode is observable."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Without Redis-backed sharing, each instance tracks transient health independently."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "dashboard-transport-security",
      children: "Dashboard Transport Security"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy dashboard"
      }), " command and the ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/_internal/dashboard/*"
      }), " HTTP endpoints\nshare the proxy listener."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Listener addresses are TCP bind addresses in ", (0,jsx_runtime.jsx)(_components.code, {
          children: "host:port"
        }), " form, not URLs."]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "The dashboard command is local-only. It connects over loopback plain HTTP with\nbearer authentication and refuses concrete non-loopback listener hosts."
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "HTTPS and remote dashboard URLs are not supported by the current configuration\nmodel. Remote dashboard access requires a future explicit transport design."
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Repeated invalid dashboard tokens are rate limited with ", (0,jsx_runtime.jsx)(_components.code, {
          children: "429"
        }), " and a\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "Retry-After"
        }), " header."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "security-defaults",
      children: "Security Defaults"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "API keys and client bearer tokens are never logged"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "prompt and response bodies should be redacted or omitted from standard logs"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "request IDs are emitted for correlation"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "logging",
      children: "Logging"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use the optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "logging"
      }), " block to control structured log verbosity and request lifecycle access logging."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "logging {\n  level      = \"info\"\n  access_log = true\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When ", (0,jsx_runtime.jsx)(_components.code, {
        children: "access_log = true"
      }), ", request logs include events for request receipt, upstream provider/model selection and completion, and the final response or streaming start and end."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "secret-handling",
      children: "Secret Handling"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), " is used, the default key file path is:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "$XDG_CONFIG_HOME/aiproxy/keys.json"
        })
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "~/.config/aiproxy/keys.json"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Mount this file read-only in production deployments."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["GitHub Copilot logins live beside that file as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "copilot-<name>.json"
      }), "\nsidecars (", (0,jsx_runtime.jsx)(_components.code, {
        children: "0600"
      }), ", restrictive parent directory). ", (0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref.path"
      }), "\ndefaults to the same secrets path; mount the secrets directory (not just\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "keys.json"
      }), ") when Copilot providers are configured, and reload with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), "\nor a restart after every ", (0,jsx_runtime.jsx)(_components.code, {
        children: "login"
      }), " or sidecar rotation."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "production-checklist",
      children: "Production Checklist"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["enable ", (0,jsx_runtime.jsx)(_components.code, {
          children: "bearer_static"
        }), " auth unless the deployment is fully trusted"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "keep provider secrets out of the HCL file when possible"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "mount config and key files read-only"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["declare ", (0,jsx_runtime.jsx)(_components.code, {
          children: "metrics { token = env(\"AIPROXY_METRICS_TOKEN\") }"
        }), " and scrape\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /metrics"
        }), " with the configured bearer token"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["use ", (0,jsx_runtime.jsx)(_components.code, {
          children: "aiproxy dashboard"
        }), " only from the local host; remote dashboard access is\nunsupported until an explicit transport design is added"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["explicitly ", (0,jsx_runtime.jsx)(_components.code, {
          children: "enabled = false"
        }), " any provider you want to keep defined but\ninactive; missing credentials on enabled providers fail validation"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "use aliases for controlled failover instead of relying on direct model requests"
      }), "\n"]
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