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
  "value": "Editing Database Providers In The Web UI",
  "id": "editing-database-providers-in-the-web-ui",
  "level": 3
}, {
  "value": "Editing Database Aliases In The Web UI",
  "id": "editing-database-aliases-in-the-web-ui",
  "level": 3
}, {
  "value": "Database Key Spend And Quota Resets",
  "id": "database-key-spend-and-quota-resets",
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
  "value": "Interactive Dashboard Lifecycle",
  "id": "interactive-dashboard-lifecycle",
  "level": 2
}, {
  "value": "Dashboard Layout And Keyboard Controls",
  "id": "dashboard-layout-and-keyboard-controls",
  "level": 2
}, {
  "value": "Dashboard Payload And Block Inspection",
  "id": "dashboard-payload-and-block-inspection",
  "level": 2
}, {
  "value": "Dashboard Requests And Search",
  "id": "dashboard-requests-and-search",
  "level": 2
}, {
  "value": "Dashboard Provider Diagnostics",
  "id": "dashboard-provider-diagnostics",
  "level": 2
}, {
  "value": "Dashboard Stable Inspection And Pause",
  "id": "dashboard-stable-inspection-and-pause",
  "level": 2
}, {
  "value": "Dashboard Metrics And Cost Estimates",
  "id": "dashboard-metrics-and-cost-estimates",
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
  "value": "Payload logging",
  "id": "payload-logging",
  "level": 3
}, {
  "value": "Secret Handling",
  "id": "secret-handling",
  "level": 2
}, {
  "value": "Mock-only Copilot Verification",
  "id": "mock-only-copilot-verification",
  "level": 2
}, {
  "value": "Production Checklist",
  "id": "production-checklist",
  "level": 2
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
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["GitHub Copilot is ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "hermetically verified; live GitHub compatibility unverified"
      }), ".\nSee ", (0,jsx_runtime.jsx)(_components.a, {
        href: "#mock-only-copilot-verification",
        children: "mock-only verification"
      }), " for local checks\nrequiring no real client ID/account. Production ", (0,jsx_runtime.jsx)(_components.code, {
        children: "login"
      }), " below contacts GitHub;\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "configure provider"
      }), " only references its saved credential (no API-key flags,\nOAuth networking or token display in configure). The model name is illustrative:"]
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
      children: "The web provider form offers the same authorization as Connect GitHub: enter\nthe public OAuth client ID, approve at the shown URL with the shown code, then\nsave the provider to apply. The browser never receives tokens; the server\nstores the credential encrypted in the database. CLI sidecar and Connect GitHub\nare alternatives; Copilot Rotate in the web UI opens reauthorization."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Connect GitHub details: the server performs the device challenge over the\nfixed GitHub issuer, fixed ", (0,jsx_runtime.jsx)(_components.code, {
        children: "read:user"
      }), " scope, and the fixed verification page,\nthen stores the result as an AES-GCM-encrypted database credential (never a\nsidecar, never a raw-token API input). The same database encryption key must\nbe configured on every instance or the saved credential fails to decrypt.\nUnfinished device sessions expire (pending is bounded by the issuer expiry and\n15 minutes, ready lasts 10 minutes) and are reaped by a bounded per-minute\ncleanup (100 rows per batch); provider saves set\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Catalog-Saved: true"
      }), " once the database commit is durable, including\nsaved-but-activation-failed outcomes where the old runtime stays active.\nActivation applies to the receiving instance only; there is no cluster\nbroadcast, so reload or mutate each instance (or let its next mutation pick\nthe catalog up). Hermetically verified; live GitHub compatibility unverified\n(see below)."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Inference preserves upstream JSON ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), " errors without implicit re-login;\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "models --upstream"
      }), " adds a re-login hint. A new listing invocation reads the new\nsidecar immediately after login, while the running server still requires reload."]
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
        children: "payload-log configuration"
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
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When multi-tenancy is enabled, database-backed providers, aliases, and\ninbound keys merge into the serving catalog on the same reload path (and\nautomatically after admin API mutations). An invalid row — for example a\nprovider whose encrypted credential no longer decrypts under the current\nencryption key, or an alias pointing at a removed target — fails the reload\nwith the old runtime intact, and blocks server startup the same way an\ninvalid config file does. Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy validate --check-db"
      }), " to identify the\noffending row without running the server, then fix it (rotate the credential,\nretarget or disable the entry) or delete it via the admin UI/CLI; the next\nmutation or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " picks the catalog back up."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Database inbound keys with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "expires_at"
      }), " stop authenticating at that timestamp,\nusing the server clock, even if no reload occurs. Model listing, billing usage,\nand new inference requests (including SSE) return the standard JSON ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), "\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "auth_failed"
      }), " error (", (0,jsx_runtime.jsx)(_components.code, {
        children: "invalid client token"
      }), ") at or after expiry, before upstream\nI/O. Requests and streams already authenticated may finish. Static HCL clients\nand database keys without expiry do not expire. Changing a key's expiry,\nrotating it, or disabling/deleting it still uses the catalog reload path above;\nthe stored deadline in an active runtime requires no database access per request."]
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
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider and alias administrative POST/PUT requests commit their complete aggregate\nbefore requesting runtime activation, then read the response view. Provider model\nwrite failures roll back metadata too. A concurrent edit returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "409"
      }), "; read the\ncurrent provider/alias and retry. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500 could not save catalog edit"
      }), " means no\nactivation was requested. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500 saved but activation failed"
      }), " means the complete\nedit is in the database while the old runtime remains active: inspect server logs,\nresolve the failure and reload. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500 saved but response view unavailable"
      }), " means the\nedit was saved and activation succeeded, but the response view failed; read current\nstate before retrying, especially before repeating a create. These outcomes do not\npromise a distributed transaction between the database and running proxies."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Use reload for routing and auth changes, not for socket-level listener changes."
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "editing-database-providers-in-the-web-ui",
      children: "Editing Database Providers In The Web UI"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The provider editor sends changed fields as a partial update. Both enabled\ntransitions, unchecked forwarding/healthcheck authorization, cleared strings and\ncleared local header lists are saved explicitly. Unchanged models and credential\nreferences are omitted. Leave the write-only API-key field blank to keep its stored\nsecret; entering a new value replaces it. Changing a reference path retains the\ndisplayed key/name, and clearing the path selects the default secrets file."
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Blank URL, user-agent and header-timeout overrides restore the applicable provider\nor root defaults; provider types without a default URL still require one. Turning\noff local user-agent forwarding or clearing local forwarded headers cannot opt out\nof independently enabled root forwarding. The form displays local database values,\nnot the merged effective root settings.\nThese defaults apply to database providers on startup and every successful reload;\nchanging a root setting does not rewrite the saved local fields."
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "For inherited database providers, the editor uses the base provider's models,\ntransport and healthcheck rather than asking for local model rows. Type must match\nthe static base; display name and credentials remain local, with a blank display\nname falling back to the base. The database provider's Enabled flag remains local.\nClearing Extends requires valid local settings and models before saving."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["An existing custom healthcheck remains selected because the current API cannot\nremove its block. Its path must stay nonempty. Clearing other scalar fields resets\nthem to the defaults shown in the form (GET, status 200, body ", (0,jsx_runtime.jsx)(_components.code, {
        children: "*"
      }), ", interval 30s,\ntimeout 5s, failure threshold 2, success threshold 1); unchecking authorization\nexplicitly saves false. Saving and reopening shows the API-normalized values."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "editing-database-aliases-in-the-web-ui",
      children: "Editing Database Aliases In The Web UI"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Enter one ", (0,jsx_runtime.jsx)(_components.code, {
        children: "provider/model"
      }), " target per line. Only the first slash separates the\nprovider from the model: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gateway/z-ai/glm-5.2"
      }), " keeps the complete model name\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "z-ai/glm-5.2"
      }), " when creating, saving or reopening an alias. Deeper names such as\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "gateway/group/family/model"
      }), " are supported too. Blank lines and surrounding provider/\nmodel whitespace are ignored; whitespace inside a name is invalid."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Each provider name and slash-separated model segment must start with a lowercase\nletter or digit, followed by lowercase letters, digits, dots, underscores or\nhyphens. Provider name ", (0,jsx_runtime.jsx)(_components.code, {
        children: "alias"
      }), " is reserved. Missing names, empty segments, repeated\nor trailing slashes and other separators produce a field error before submission.\nTarget errors identify the input line to correct."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Alternatively, supply comma-separated provider names and one full model name in\nthe shorthand fields. The same naming rules apply; list each provider once with\nno empty comma entries. Clear explicit targets before using shorthand, and supply\nboth shorthand fields. Saved shorthand reopens as the equivalent full target list."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "database-key-spend-and-quota-resets",
      children: "Database Key Spend And Quota Resets"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "With multi-tenancy enabled, deleting an inbound key preserves its recorded spend\nand user/team owner attribution. Requests already admitted with that key still\nrecord their usage when they finish, including streamed responses. Creating a\nreplacement key, even with the same name, does not restore the owner's budget;\nscope totals survive spend-cache refresh and server restart. Sharing a key does\nnot transfer its spend to the other users or teams with access."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["A workspace administrator can explicitly reset a user/team budget's spend\nwith ", (0,jsx_runtime.jsx)(_components.code, {
        children: "reset_spend: true"
      }), " on its quota endpoint. Resetting records an offset against\nthe current total; it does not delete historical usage. A request whose usage is\nrecorded after the reset counts against the new budget, even if it started before\nthe reset or its key has been deleted. Budget checks retain their existing\n30-second cache and completion-based accounting; already-admitted requests may\nexceed the budget."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Quota policy reads distinguish missing rows (unlimited) from storage errors.\nWhen required budget/TPM rows cannot be read, or spend cannot be read for a\npositive budget with a cold or expired cache, new inference requests return JSON\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "503"
      }), " ", (0,jsx_runtime.jsx)(_components.code, {
        children: "quota_unavailable"
      }), " before any upstream call. This also applies to SSE\nrequests, which receive JSON rather than a started stream. Storage causes are\nlogged server-side; the response contains no database details. Retry after\nstorage recovers. Actual budget exhaustion remains ", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), " ", (0,jsx_runtime.jsx)(_components.code, {
        children: "budget_exceeded"
      }), ", and\nactual TPM exhaustion remains ", (0,jsx_runtime.jsx)(_components.code, {
        children: "429"
      }), " ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tpm_exceeded"
      }), " with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Retry-After"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["A fresh spend-cache entry remains usable within its existing 30-second TTL,\nincluding for budget denial. Failed refreshes neither cache zero nor extend an\nexpired entry. Policy rows are still read on each request, even with fresh\ncached spend. A genuinely missing or zero budget needs no spend read for\nadmission; static HCL credentials do not acquire database quota dependencies.\nAdmin quota views read current spend even without a budget and fail with a\ncontrolled ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " if the data is unavailable. Admin edits stop on failed policy\nreads; if an edit saves successfully but its response view cannot be read, the\nerror explicitly says the quota was saved. Inspect the view after recovery\nbefore repeating that edit."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The forward database migration retains key and workspace UUIDs for accounting,\nwithout retaining deleted credentials. These identities and the ledger remain\nuntil the workspace is deleted. Previously erased spend cannot be recovered\nby the migration."
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
      id: "interactive-dashboard-lifecycle",
      children: "Interactive Dashboard Lifecycle"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy dashboard"
      }), " attaches to a running local server. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "q"
      }), " (outside search) or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Ctrl+C"
      }), " exits the\ncommand and stops its polling and in-flight requests. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Esc"
      }), " closes help, then a\ndetail, then a zoomed pane; at the main view it exits. Terminal initialization/I/O errors\nare returned by the command, and the TUI uses the command's configured input/output."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Initial attachment still fails clearly for an unreachable server, missing dashboard\nconfiguration, or invalid token. After successful attachment, transient transport,\ntimeout, malformed-snapshot, HTTP 408/429 and server errors preserve the last view.\nThe connection line shows ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "RECONNECTING"
      }), ", the local time of the last successful\nsnapshot, and the next retry time. Retries wait 2, 4, 8, 16, then at most 30 seconds\nafter failed attempts; successful snapshots resume the normal two-second interval.\nEach snapshot request has a two-second timeout. Connection state remains live while\ndisplay updates are paused."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "Ctrl+R"
      }), " requests a retry now in any pane; repeated presses during an active snapshot\nrequest coalesce into that request. Existing pane-local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "r"
      }), " refresh actions remain\navailable. HTTP 401/403/404, redirects and other permanent endpoint denials show\n", (0,jsx_runtime.jsx)(_components.strong, {
        children: "DENIED"
      }), " and stop automatic snapshot retries and new pane RPCs. Fix the server and\npress ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Ctrl+R"
      }), " to probe again. The token/config are read only at attachment: quit and\nre-run ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy dashboard"
      }), " after changing local credentials or listener configuration.\nA successful probe reopens pane access; use pane-local refresh to retry a failed\npane list. Block decisions and take-once detail reads are never automatically replayed."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "dashboard-layout-and-keyboard-controls",
      children: "Dashboard Layout And Keyboard Controls"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The minimum supported terminal is ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "80 columns × 12 rows"
      }), ". Below 30 rows (including\n80×24), a compact layout shows only the focused pane. At 30 rows and above, providers,\nusage and the active bottom tab are stacked. ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Tab / Shift-Tab"
      }), " cycles forward/back\nthrough Providers → Usage → bottom pane, even in compact or zoomed layouts. Headers,\nrows and errors are fitted in terminal cells, including wide Unicode characters.\nThe footer keeps help/back/quit controls visible (apply/cancel/quit while editing), with a separate live\nconnection line. Narrow views abbreviate measurement legends; help explains them."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Key"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Action"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "1"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "2"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "3"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "4"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "5"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Select Aliases / Logs / Payloads / Blocks / Requests and focus it"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "["
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "]"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Previous / next bottom tab in that order, wrapping at either end"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "Enter"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Open selected provider/bottom-row or Usage top-group detail; close an open detail"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "z"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Toggle focused-pane zoom from a list; details already occupy the body"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "Esc"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Close help, else detail, else zoom; otherwise quit"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "q"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "Ctrl+C"
            })]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Quit, including from help/detail (", (0,jsx_runtime.jsx)(_components.code, {
              children: "q"
            }), " is text in search)"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "?"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "h"
            })]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Open help; ", (0,jsx_runtime.jsx)(_components.code, {
              children: "?"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "h"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "Esc"
            }), " or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "Enter"
            }), " closes it"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "j/k"
            }), ", arrows, ", (0,jsx_runtime.jsx)(_components.code, {
              children: "PgUp/PgDn"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "Home/End"
            }), " (", (0,jsx_runtime.jsx)(_components.code, {
              children: "g/G"
            }), ")"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Move/scroll the focused view; scroll help while help is open"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "+/-"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "J/K"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Resize the bottom pane in stacked layout"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "t/e/u"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Tenant / errors-only / public-upstream filters in focused Usage"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "l"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "o"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Level filter / order in focused Logs"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "s"
            }), " (or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "e"
            }), "), ", (0,jsx_runtime.jsx)(_components.code, {
              children: "o"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "r"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Status filter / order / refresh in focused Payloads list"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "r"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Refresh focused Blocks list"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "p"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Pause/resume data in a pane or detail"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "Ctrl+R"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Retry the connection, including from help"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "/"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "Ctrl+U"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Search Requests/Logs/Payloads metadata / clear applied search"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "l"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "v"
            }), " in Request detail"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Exact-ID Logs / Payloads; Esc returns to Requests"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "n"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "N"
            }), " in Usage detail"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Next / previous exact identity group"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Changing focus or tabs closes an open detail first and preserves list selection.\nFor remote details this cancels the read and discards late replies, including paused\nresults. Resizing and opening/closing help preserve the detail and selected record.\nEnter on Usage shows the top group's full tenant/client/model/operation/status;\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "n/N"
      }), " cycles every group, including the final rows. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "z"
      }), " zooms its list.\nHelp owns input: pane filters, pause, tab navigation and decisions cannot run behind\nit. An undersized-terminal warning also suppresses pane actions until resized."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Block-detail ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "n/N"
      }), " selects the next/previous finding. ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "a/s/d"
      }), " records allow\n(non-secret), redact (placeholder), or deny (keep blocking) for ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "only that finding's\nexact hash"
      }), ". The selected index, scope and status stay visible while scrolling.\nNumbered keys 1–5 always navigate tabs outside search and never record a decision."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "dashboard-payload-and-block-inspection",
      children: "Dashboard Payload And Block Inspection"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Payload and block text wraps to terminal cells, including long JSON strings, CJK,\ncombining characters and emoji. Use ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "j/k"
      }), ", ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "PgUp/PgDn"
      }), ", ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Home/End"
      }), " to inspect\nall retained text. The fixed header identifies the record and selected finding;\nwrapped row position and cap notice stay visible. Full IDs, hashes and metadata also\nappear in the scrollable body. Control bytes and invalid UTF-8 appear as escapes\ninstead of being interpreted as terminal commands. Resizing rewraps/clamps the\nposition without changing the selected record or finding."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Payload pretty output retains the existing ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "64 KiB"
      }), " cap. The view explicitly\nlabels pretty-output truncation and server request/upstream-request/response body\ntruncation flags. An unavailable flag is not proof of completeness. Block snippets\nare already server-capped (scanner default 512 bytes, further limited by configured\nquarantine snippet size); the existing response does not carry original lengths or\ntruncation flags, so its fixed notice says ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "truncation unknown"
      }), ". Wrapping cannot\nrecover uncaptured text."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The Blocks list warns before Enter: a successful detail read ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "consumes the\ntake-once capture"
      }), "; closing it does not make it available again. Another operator,\nexpiry, or a canceled/failed read may also leave it unavailable. A decision persists\n", (0,jsx_runtime.jsx)(_components.strong, {
        children: "global future-match behavior for that hash"
      }), ", including identical hashes in other\nfindings/requests. It ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "never replays or resumes the blocked request"
      }), ". There is no\nbulk decision shortcut."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "While recording, finding selection is locked and further decisions are ignored;\nscroll/help/back/quit remain available. A successful acknowledgment suppresses the\nsame action for that hash during the open detail; another action may deliberately\nreplace it. Failures show that the outcome may be unknown (the server could have\npersisted before a connection failure); pressing a/s/d again deliberately retries.\nMissing/invalid hashes disable decisions; the TUI never hashes a possibly truncated\nsnippet as a substitute. Pause defers acknowledgments and prevents new decisions.\nClosing/navigation cancels outstanding work and discards late results; it cannot\nundo a decision already persisted by the server. No read or decision is automatically\nreplayed on reconnect/resume. Full text remains confined to authenticated operator\ndetail fetches, outside Requests metadata and search indexes."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "dashboard-requests-and-search",
      children: "Dashboard Requests And Search"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsxs)(_components.strong, {
        children: ["5", ":Requests"]
      }), " shows the last ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "up to 200 completed inference operations"
      }), ", newest\nfirst, even with disk payload logging disabled. This is a process-local count cap,\nnot a time window or complete request history. It contains no prompts or credentials,\nand does not claim attempt or in-flight statistics. Enter inspects request ID,\ntenant/client, submitted public model, resolved provider/configured model, operation,\nsent HTTP status, duration and reported input/output/cache tokens. A zero token count\nmay mean unreported usage; unresolved targets and old-snapshot IDs are unavailable.\nDetail remains on the selected completion even if its list entry expires."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Recent diagnostic text is bounded before storage: request ID, tenant, client and\nprovider each retain up to ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "256 bytes"
      }), "; submitted/accounting/resolved model fields\neach up to ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "512 bytes"
      }), "; operation up to ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "64 bytes"
      }), ". UTF-8 truncation preserves\ncomplete characters. The detached strings total at most ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "2,624 bytes per entry"
      }), "\n(524,800 for 200 entries, excluding fixed overhead). This does not bound total\nprocess memory or the rest of the dashboard snapshot. Full routing, billing,\nrate and provider/P95 grouping retain their original semantics."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Affected Requests rows show ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "[truncated]"
      }), "; detail lists the shortened fields.\nSearch covers their retained prefixes only. If the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "request ID"
      }), " was truncated,\nexact log/payload correlation is unavailable: ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "l/v"
      }), " stay in detail with an\nexplanation. Other shortened fields do not prevent correlation by an intact ID.\nOld snapshots without truncation metadata keep their existing behavior."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Request metadata is displayed literally: actual newlines, carriage returns, tabs\nand terminal controls appear as escapes such as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\\n"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\\r"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\\t"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\\x1b"
      }), ".\nLiteral backslashes are doubled, so a submitted backslash-n displays as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\\\\n"
      }), ".\nPrintable Unicode is retained. Each completion stays on one summary row; Enter\nopens that selected completion, with wrapped, scrollable escaped detail.\nRequest/Usage identity detail, correlation labels and payload summary/ID labels\nuse a ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "512-source-byte per-value display cap"
      }), ". This preserves every newly retained\nrecent field; larger legacy/sibling values explicitly show\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "[display clipped at 512 bytes]"
      }), ". This is separate from server ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "[truncated]"
      }), " flags.\nSearch and exact-ID correlation still use original retained values, not the display\nescapes: typing ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\\n"
      }), " searches for a literal backslash-n, not an actual newline."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Press ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "/"
      }), " in Requests, Logs or Payloads. Search ANDs space-separated, case-insensitive\nsubstrings; use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "field:value"
      }), " for one metadata field. Examples:\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "client:ci tenant:team-a status:500"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "id:request-123"
      }), ". Supported fields:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Requests: ", (0,jsx_runtime.jsx)(_components.code, {
          children: "id"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "client"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "tenant"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "model"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "resolved"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "status"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "op"
        }), "."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Logs: ", (0,jsx_runtime.jsx)(_components.code, {
          children: "id"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "level"
        }), " (structured metadata, not flattened attributes or body text)."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Payloads: ", (0,jsx_runtime.jsx)(_components.code, {
          children: "id"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "model"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "resolved"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "status"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "method"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "path"
        }), "."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Bare terms search all supported fields. Unknown fields match nothing. Search combines\nwith Requests ", (0,jsx_runtime.jsx)(_components.code, {
        children: "e"
      }), " errors-only, Logs level, and Payloads status filters. The applied\nfilter and no-match state are visible; ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Ctrl+U"
      }), " clears search in the focused list.\nPayload status changes preserve a matching selected identity or clamp to the nearest\nremaining row immediately. While the replacement list is loading, Enter waits for\nthe visible list before opening detail.\nThe editor holds at most ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "256 printable characters"
      }), ". Arrows, Home/End,\nBackspace/Delete edit; ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Enter"
      }), " applies, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Esc"
      }), " cancels to the prior filter,\nand ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Ctrl+U"
      }), " clears the edit. q/p/h/?/numbers/brackets are text while editing;\nCtrl+C still quits and Ctrl+R retries. Leave the editor to open help or pause.\nSearch only filters already bounded metadata; it never searches payload bodies or\nfetches more history. Cached search and request/usage detail work while paused."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["From Request detail, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "l"
      }), " opens exact-ID logs and ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "v"
      }), " opens exact-ID payloads.\nTarget filters are temporarily bypassed; their settings and selected row return\nafterward. Enter inspects a matching row; Esc closes it, then another Esc restores\nRequest detail/scroll/zoom. Ordinary tab/focus navigation exits this return workflow.\nLogs may be disabled, expired or missing structured IDs on older servers. Payloads\nmay be disabled, never captured, expired or outside the latest ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "100"
      }), " entries;\nthe pane explains what is known, without expanding retention. Duplicate caller IDs\ncan match several records. Resume before a remote payload read; late replies after\nclosing correlation are discarded."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "dashboard-provider-diagnostics",
      children: "Dashboard Provider Diagnostics"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Focus ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Providers"
      }), ", move with arrows or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "j/k"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "PgUp/PgDn"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Home/End"
      }), " also work),\nand press ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Enter"
      }), ". The ", (0,jsx_runtime.jsx)(_components.code, {
        children: ">"
      }), " cursor follows the provider name across refreshes and\nenabled/disabled group changes. If a provider disappears, its open detail reports\nremoval; returning to the list selects the nearest surviving row. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "z"
      }), " still zooms\nthe list. Details wrap and scroll even at 80×12; pause freezes their data and clock."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Provider detail includes name/display/type, sanitized effective endpoint, enabled\nand routing-health state, upstream header timeout, configured probe method/path,\nexpected HTTP status, interval/timeout and failure/success thresholds. Probe state\nis the stored threshold state; the last HTTP status and reason describe the latest\ncheck and can differ while a threshold is being reached. Last-check age comes from\nthe stored check timestamp, never snapshot receipt time. Pending, absent probe,\ndisabled provider and unavailable old-server metadata are distinguished. No aliases\nare needed to inspect a provider. Models show public-to-upstream mappings, protocol\nand capabilities from the active catalog; an empty native-provider protocol means\nit is not separately configured."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The list's ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "HOST"
      }), " is the configured hostname, with no DNS/network work in rendering\nor navigation. Endpoint/probe URLs omit userinfo, query strings and fragments. Only\nstandard API/health path segments are retained; other segments show URL-encoded\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "[redacted]"
      }), " to avoid exposing path credentials. Invalid URLs are redacted wholesale.\nRaw probe error text becomes a bounded category (timeout, DNS, connection refused,\nTLS, transport error) or a safe status/body-mismatch reason. Credentials, credential\nreferences, probe expected bodies and authorization settings are not diagnostic\ntransport fields."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Alias detail carries the actual session-affinity configuration and effective header\nnames, including defaults; absent old-server metadata is ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "unknown"
      }), ", not disabled.\nIts counters remain ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "provider-wide lifetime"
      }), ", repeated when targets share a\nprovider; they are not target-specific or alias-specific counts."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "dashboard-stable-inspection-and-pause",
      children: "Dashboard Stable Inspection And Pause"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Refresh keeps the top provider/usage row and selected alias/payload/block by identity.\nIt also keeps the top cursor-list row when that row and the selection can still fit\nin the viewport. If a row disappears, its old position is clamped to the remaining\nlist; selection visibility takes priority. Usage identity includes tenant, client,\nmodel, operation and status. A selected tenant remains selected when other tenants\narrive or reorder; if it disappears, the filter returns to ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "all tenants"
      }), ".\nPayload order changes preserve the selected request. Logs follow new arrivals until\nyou move away; pinned logs stay selected by sequence ID. Changing log order resumes\nfollow in that order."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Press ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "p"
      }), " in a pane or detail to freeze incoming data and the display clock,\nincluding uptime, health, logs, measurement windows, payload/block rows, details and\ndecision acknowledgments. Connection status remains live and separately labeled.\nOnly the latest snapshot and latest accepted result per pane request type are held;\nresume applies them together in one UI update and advances the clock to the latest\ntick. Independent pane RPCs do not promise the same server measurement instant."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "While paused, browse cached rows and use local usage/log filters or ordering. Resume\nbefore requesting a remote refresh, payload status filter, new payload/block detail,\nor block decision; these keys do not queue work while paused. A previously unvisited\npane begins loading on resume. In-flight results wait for resume, and closing a\ndetail discards its buffered result. Closing/reopening an ID or changing a remote\nfilter rejects obsolete replies. Take-once block reads and decisions are never\nautomatically repeated; an already consumed capture can return unavailable on a\ndeliberate reopen."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "dashboard-metrics-and-cost-estimates",
      children: "Dashboard Metrics And Cost Estimates"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.strong, {
          children: "GLOBAL rates:"
        }), " requests/second over the last 60 and 300 complete seconds,\nplus one-minute error, 429 and token totals. The 15-minute graph contains 15\nconsecutive one-minute request totals, oldest on the left. Counters use one-second\nbuckets, include completed requests (including failures/unresolved models), and\ndo not depend on the 200 recent-request cap. The ongoing second appears when it\ncompletes; normal snapshot polling adds up to approximately two seconds. Idle\nminute tokens become zero. Errors exclude 429; throttling is shown separately."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.strong, {
          children: "Provider counters:"
        }), " global process-lifetime requests/errors/429/tokens. The\nsame provider-wide lifetime scope applies to counters shown under alias targets.\nThese do not reconcile to rolling usage/cost after old billing buckets expire."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.strong, {
          children: "P95/n:"
        }), " nearest-rank P95 latency and the number of positive-duration samples\nfor that provider among up to 200 received recent completions across all providers.\nThis is a capped sample with ", (0,jsx_runtime.jsx)(_components.strong, {
          children: "no time window"
        }), ", not a minute percentile or a\nlifetime percentile. ", (0,jsx_runtime.jsx)(_components.code, {
          children: "n/a/0"
        }), " means no positive-duration sample; idle samples remain."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.strong, {
          children: "USAGE and EST$:"
        }), " retained rolling 24-hour usage and estimated USD at current\nconfigured prices. Retention uses minute buckets: a bucket remains while its\nstart is at or after ", (0,jsx_runtime.jsx)(_components.code, {
          children: "snapshot time - 24h"
        }), ". Individual events can therefore expire\nalmost a minute early. ", (0,jsx_runtime.jsx)(_components.code, {
          children: "t"
        }), "/", (0,jsx_runtime.jsx)(_components.code, {
          children: "e"
        }), " select tenant/error usage; global rates and provider\ncounters ignore those filters. ", (0,jsx_runtime.jsx)(_components.code, {
          children: "u"
        }), " switches between public and resolved upstream\nusage over the same retention window, preserving tenant/client dimensions."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Alias estimates use the actual retained public-model/tenant/client/operation/status\nattribution. Direct traffic and other aliases sharing a target are not charged\nagain. Unused targets do not affect the estimate. ", (0,jsx_runtime.jsx)(_components.code, {
          children: "-"
        }), " means unavailable (missing\nattribution, a needed price, or no retained usage), not free. A provider estimate\nis its complete retained subtotal, not a sum of only priced models; another\nprovider's price gap does not hide its own fully attributed/priced subtotal."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Older servers without the additive rate/billing snapshot data display unavailable\nmeasurements. The TUI does not estimate rates from recent requests or costs from\nlifetime totals. Paused/stale views retain the received measurement window; the\nconnection line separately reports local refresh/reconnect activity."
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
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The embedded web dashboard is served at ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/"
      }), " when a ", (0,jsx_runtime.jsx)(_components.code, {
        children: "web_ui"
      }), " block\nis present. Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy webui"
      }), " to print that URL after probing that a\nrunning server answers with the UI (", (0,jsx_runtime.jsx)(_components.code, {
        children: "--open"
      }), " also launches the default\nbrowser). The UI's live views authenticate against the same\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "dashboard"
      }), "-gated ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/_internal/dashboard/*"
      }), " APIs with the dashboard token or an\nactive stored system-administrator account. Ordinary accounts and workspace\nadministrators cannot access these global operator surfaces."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Within a loaded browser tab, changing accounts, changing/removing the dashboard\ntoken, or selecting another workspace clears sensitive cached data and open\ndetail/dialog state. Delayed responses from the previous context are discarded.\nAccount replacement also resets the selected workspace. A successful normal\ntoken refresh preserves the current account's cache and workspace; a failed\nrefresh clears the account session. Sign out clears the local account, workspace\nselection, and dashboard token immediately, even if server-side session revocation\nis delayed or fails. Dashboard tokens are installed only after successful validation.\nThese transitions are local to the current tab; other already-open tabs do not\nautomatically synchronize their in-memory sessions."
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
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "payload-logging",
      children: "Payload logging"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The optional nested ", (0,jsx_runtime.jsx)(_components.code, {
        children: "payload_log"
      }), " block records request/response headers\nand bodies as JSONL (one JSON object per line per inference request). It is\ndisabled by default. Each entry carries three sides: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "request"
      }), " (inbound\nheaders and body as received), ", (0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_request"
      }), " (headers and body actually\nsent upstream, after model rewrites and provider translation; present only\nwhen an upstream call was made, so alias retries log the final attempt), and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "response"
      }), " (the upstream reply, or the proxy's own error on early rejections).\nCredential headers (", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "x-api-key"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "x-goog-api-key"
      }), ", cookies)\nare redacted on every side, and bodies covered by an ingress-guardrail policy\nare omitted from both request sides."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "logging {\n  level      = \"info\"\n  access_log = true\n\n  payload_log {\n    enabled        = true\n    dir            = \"/var/log/aiproxy/payloads\"\n    rotation       = \"daily\"\n    retention      = \"168h\"\n    max_body_bytes = 1048576\n\n    mongodb {\n      uri        = env(\"AIPROXY_PAYLOAD_MONGO_URI\")\n      database   = \"aiproxy\"\n      collection = \"payloads\"\n      timeout    = \"5s\"\n    }\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The disk backend (", (0,jsx_runtime.jsx)(_components.code, {
        children: "dir"
      }), ") and the MongoDB backend (", (0,jsx_runtime.jsx)(_components.code, {
        children: "mongodb"
      }), ") are enabled\nindependently: set ", (0,jsx_runtime.jsx)(_components.code, {
        children: "dir"
      }), " for JSONL files, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "uri"
      }), " for one MongoDB document per\nrequest with the same JSON field names, or both to fan out to both. When\nenabled, at least one backend is required. Recording is best-effort and never\nfails a request; a bad MongoDB URI fails startup fast instead."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The dashboard and TUI payload viewer read from the disk backend only: with\nMongoDB-only logging (no ", (0,jsx_runtime.jsx)(_components.code, {
        children: "dir"
      }), "), the viewer reports payload logging as\ndisabled."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Files are split by datetime so no single file grows without bound: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "daily"
      }), "\nrotation writes ", (0,jsx_runtime.jsx)(_components.code, {
        children: "payload-YYYYMMDD.jsonl"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "hourly"
      }), " writes\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "payload-YYYYMMDD-HH.jsonl"
      }), " (UTC) under ", (0,jsx_runtime.jsx)(_components.code, {
        children: "dir"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "0600"
      }), " files, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "0700"
      }), "\ndirectory). ", (0,jsx_runtime.jsx)(_components.code, {
        children: "retention"
      }), " (default 7 days, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"168h\""
      }), ") controls how long files are\nkept: files older than the retention window are removed on rotation and by an\nhourly sweep; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"0s\""
      }), " disables expiry. Bodies larger than ", (0,jsx_runtime.jsx)(_components.code, {
        children: "max_body_bytes"
      }), " per\nside are truncated with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"truncated\": true"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "0"
      }), " stores full bodies), and\nsensitive headers (", (0,jsx_runtime.jsx)(_components.code, {
        children: "authorization"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "proxy-authorization"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "cookie"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "set-cookie"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "x-api-key"
      }), ") are stored as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[REDACTED]"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Only enable payload logging when you can protect the output directory:\nbodies contain prompts and completions. Changes apply on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " reload."]
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
      id: "mock-only-copilot-verification",
      children: "Mock-only Copilot Verification"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.strong, {
        children: "Hermetically verified; live GitHub compatibility unverified."
      }), " From the repo\nroot, with the repository Go/pnpm toolchain and dependencies installed, run these\ncommands serially. Finish the UI build before Go checks read embedded assets:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "make web-build\ngo test -race ./internal/copilotlogin ./cmd/aiproxy ./internal/config ./internal/app ./internal/provider -count=1\nmake integration\nAIPROXY_BINARY=\"$PWD/dist/aiproxy\" go test -tags=integration ./internal/integration -run GitHubCopilot -count=1 -v\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "No real client ID, account, browser authorization, or PostgreSQL fixture is needed\nfor Copilot scenarios. Binary integration runs on Linux. The final command forces\nthe Copilot binary cases to execute uncached and shows their names. Optional\ndatabase-backed tests in broader suites may skip without their separate fixture."
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Layer"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "What the checks establish"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: ["Protocol/persistence unit tests (", (0,jsx_runtime.jsx)(_components.code, {
              children: "internal/copilotlogin"
            }), "; command ", (0,jsx_runtime.jsx)(_components.code, {
              children: "login_test.go"
            }), ")"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Success, pending/slowdown, denial/expiry, cancellation, malformed/oversized responses, redirect/network errors, secret redaction, file permissions, unsafe destinations and independent writers."
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: ["Composed in-process (", (0,jsx_runtime.jsx)(_components.code, {
              children: "cmd/aiproxy"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "TestCopilotMockProvisioningAndRecovery"
            }), ")"]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Real ", (0,jsx_runtime.jsx)(_components.code, {
              children: "runLoginCopilot"
            }), " command orchestration with injected local issuer/time, persisted sidecar, scripted configure/validate and models commands, real App handler over a local HTTP server, configured/upstream model distinction, JSON/SSE, simulated 401/403 revocation, failed/cancelled re-login preservation, successful re-login, failed-reload rollback and explicit reload recovery."]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: ["Real binary (", (0,jsx_runtime.jsx)(_components.code, {
              children: "internal/integration"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "TestBinaryGitHubCopilot*"
            }), ")"]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["CLI flag constraints, real ", (0,jsx_runtime.jsx)(_components.code, {
              children: "serve"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "models --upstream"
            }), ", static inventory, JSON/SSE, unsupported-operation rejection, 401/403 and sidecar rotation, rejected/successful SIGHUP reloads. Binary tests provision synthetic sidecars directly through ", (0,jsx_runtime.jsx)(_components.code, {
              children: "Save"
            }), "; they do ", (0,jsx_runtime.jsx)(_components.strong, {
              children: "not"
            }), " run device authorization in the child process."]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The composed test uses the existing in-process issuer seam, not a public mock\nflag. Its transport refuses non-loopback dials. The new binary recovery fixture\nroutes unexpected non-loopback HTTP(S) through a rejecting local proxy; all\nintended service URLs are loopback. Tests close local servers, application\nresources, response bodies and child processes, and use automatically removed\ntemporary config/credential directories. They do not modify your saved login."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["These checks simulate the upstream contract. They cannot establish direct-Bearer\nacceptance for your application, entitlement, actual model availability, required\nGitHub headers, token lifetime, or exchange/refresh needs. Live compatibility is\ndeferred separately in repository task ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "COPILOT-LIVE-01"
      }), "\n(", (0,jsx_runtime.jsx)(_components.code, {
        children: "docs/tasks/20260927-102347-copilot-live-compatibility.md"
      }), "). Production login still\nrequires an explicitly supplied client ID and uses GitHub's fixed issuer URLs."]
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