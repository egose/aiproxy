"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[177],{

/***/ 8634
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_providers_and_routing_md_80a_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-providers-and-routing-md-80a.json
const site_docs_providers_and_routing_md_80a_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"providers-and-routing","title":"Providers and Routing","description":"aiproxy separates the client-facing model name from the concrete upstream target.","source":"@site/docs/providers-and-routing.md","sourceDirName":".","slug":"/providers-and-routing","permalink":"/docs/providers-and-routing","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":4,"frontMatter":{"sidebar_position":4},"sidebar":"docsSidebar","previous":{"title":"Config Examples","permalink":"/docs/config-examples"},"next":{"title":"Request Examples","permalink":"/docs/request-examples"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/providers-and-routing.md


const frontMatter = {
	sidebar_position: 4
};
const contentTitle = 'Providers and Routing';

const assets = {

};



const toc = [{
  "value": "Public Model Names",
  "id": "public-model-names",
  "level": 2
}, {
  "value": "Direct Routing",
  "id": "direct-routing",
  "level": 2
}, {
  "value": "Alias Routing",
  "id": "alias-routing",
  "level": 2
}, {
  "value": "Alias Algorithms",
  "id": "alias-algorithms",
  "level": 2
}, {
  "value": "Failover Rules",
  "id": "failover-rules",
  "level": 2
}, {
  "value": "Upstream Retry Cooldown",
  "id": "upstream-retry-cooldown",
  "level": 2
}, {
  "value": "Provider Types",
  "id": "provider-types",
  "level": 2
}, {
  "value": "OpenCode Zen And Go",
  "id": "opencode-zen-and-go",
  "level": 2
}, {
  "value": "Session And Client Requirements",
  "id": "session-and-client-requirements",
  "level": 3
}, {
  "value": "Quota Errors, Failover, And Static Catalogs",
  "id": "quota-errors-failover-and-static-catalogs",
  "level": 3
}, {
  "value": "GitHub Copilot",
  "id": "github-copilot",
  "level": 2
}, {
  "value": "Model Capabilities",
  "id": "model-capabilities",
  "level": 2
}, {
  "value": "<code>GET /v1/models</code> Metadata",
  "id": "get-v1models-metadata",
  "level": 2
}];
function _createMdxContent(props) {
  const _components = {
    code: "code",
    em: "em",
    h1: "h1",
    h2: "h2",
    h3: "h3",
    header: "header",
    li: "li",
    p: "p",
    pre: "pre",
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
        id: "providers-and-routing",
        children: "Providers and Routing"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " separates the client-facing model name from the concrete upstream target."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "That separation is what allows the proxy to expose a stable public catalog while still changing providers, upstream identifiers, or pool composition over time."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "public-model-names",
      children: "Public Model Names"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Clients use one of two forms:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "<provider-name>/<model-name>"
        }), " for direct routing"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "alias/<alias-name>"
        }), " for proxy-managed routing"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider and alias names are lowercase and must not contain spaces or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/"
      }), ".\nModel names are lowercase, must not contain spaces, and may contain ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/"
      }), " when\nevery slash-separated segment follows the same lowercase name rule."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "direct-routing",
      children: "Direct Routing"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Direct requests resolve to one configured provider/model pair and do not fail over."
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Use direct routing when the client intentionally wants a specific upstream model."
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Examples:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "openai/gpt-4.1"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "localai/qwen3-32b"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "alias-routing",
      children: "Alias Routing"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Aliases expose a virtual model name backed by one or more concrete targets."
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "alias \"chat_default\" {\n  algorithm = \"round_robin\"\n\n  target {\n    provider = \"openai\"\n    model    = \"gpt-4o-mini\"\n  }\n\n  target {\n    provider = \"anthropic\"\n    model    = \"claude-sonnet\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Aliases are useful when you want:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "simple failover"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "pool-based routing"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "a stable client-facing model name while changing upstream inventory"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Examples:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "alias/chat_default"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "alias/chat_fallback"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "alias-algorithms",
      children: "Alias Algorithms"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "round_robin"
        }), ": rotates through targets in process-local order"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "least_connections"
        }), ": picks the target with the fewest in-flight requests in the current process"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "least_connections"
      }), " is best-effort and not coordinated across instances."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "If you run multiple proxy instances, each instance makes its own routing decision locally."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "failover-rules",
      children: "Failover Rules"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Alias requests retry the next target when the selected target fails with:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "transport errors"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "timeouts"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["upstream responses whose status code is listed in ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry_status_codes"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["By default ", (0,jsx_runtime.jsx)(_components.code, {
        children: "retry_status_codes"
      }), " is ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[\"500\", \"502\", \"503\", \"504\"]"
      }), ", so only those common ", (0,jsx_runtime.jsx)(_components.code, {
        children: "5xx"
      }), " responses trigger status-based failover. Add codes like ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"429\""
      }), " to also retry on rate-limited responses."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Alias requests do not fail over on other upstream ", (0,jsx_runtime.jsx)(_components.code, {
        children: "4xx"
      }), " responses. Those are returned to the client as-is."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This avoids masking client-side request problems as routing problems."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Retryable ", (0,jsx_runtime.jsx)(_components.code, {
        children: "4xx"
      }), " statuses are an alias failover policy only. They do not mark the provider unhealthy; provider health is mutated by transport/upstream request errors and upstream ", (0,jsx_runtime.jsx)(_components.code, {
        children: "5xx"
      }), " responses."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "upstream-retry-cooldown",
      children: "Upstream Retry Cooldown"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Alias targets additionally honor upstream retry advice as a cross-request cooldown, so a throttled upstream is not called again until its deadline expires."
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Identity: each deadline is keyed by ", (0,jsx_runtime.jsx)(_components.code, {
          children: "(alias, provider, model)"
        }), ", alias-local\nand shared across that alias's operations. Direct ", (0,jsx_runtime.jsx)(_components.code, {
          children: "<provider>/<model>"
        }), "\nrequests never consult or populate cooldown state and never fail over."]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Triggers: any alias-target response carrying valid advice records or extends\na deadline, regardless of status. Successes are still returned normally;\nadvice affects future selection only. Transport errors without a response,\npre-I/O validation errors, and client-canceled contexts record nothing."
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Parsing and precedence: a valid positive-integer ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry-after-ms"
        }), " wins;\notherwise standard ", (0,jsx_runtime.jsx)(_components.code, {
          children: "Retry-After"
        }), " (delay-seconds, then HTTP-date) is used.\nHeader names are case-insensitive; the first valid value wins per header.\nZero, malformed, past, or unrepresentable (overflow) values record no\ncooldown from that header, with ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry-after-ms"
        }), " falling back to\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "Retry-After"
        }), ". There is no configured maximum duration, only overflow\nprotection."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Selection: cooling targets are excluded alongside already-tried targets for\nboth ", (0,jsx_runtime.jsx)(_components.code, {
          children: "round_robin"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "least_connections"
        }), ", rechecked before dispatch.\nExpired advice no longer excludes a target."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["All-cooling response: when every pool target has an active cooldown and no\nresponse has been committed, the proxy returns a generated JSON ", (0,jsx_runtime.jsx)(_components.code, {
          children: "429"
        }), "\n(", (0,jsx_runtime.jsx)(_components.code, {
          children: "type: upstream_rate_limited"
        }), ") with both ", (0,jsx_runtime.jsx)(_components.code, {
          children: "Retry-After"
        }), " (ceiling seconds,\nmin 1) and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry-after-ms"
        }), " (ceiling milliseconds, min 1) computed from the\nsame earliest ", (0,jsx_runtime.jsx)(_components.em, {
          children: "remaining"
        }), " delay (", (0,jsx_runtime.jsx)(_components.code, {
          children: "deadline - now"
        }), "), so clients are never told\nto retry early. The stored deadline uses the original delay; the response\nuses the remaining delay. Zero upstream calls occur and skipped targets gain\nno upstream attribution. A mixed pool of cooling plus otherwise-unhealthy\ntargets keeps the existing exhaustion behavior instead of synthetic ", (0,jsx_runtime.jsx)(_components.code, {
          children: "429"
        }), "."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Terminal policy: a retryable failure (per that alias's ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry_status_codes"
        }), ")\nthat newly cools the last eligible target is discarded and becomes synthetic\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "429"
        }), " immediately. A success or non-retryable error is always returned\nverbatim even when its advice completes all-cooling coverage; only subsequent\nrequests observe synthetic ", (0,jsx_runtime.jsx)(_components.code, {
          children: "429"
        }), "."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Lifetime and reload: cooldown state is process-local with no Redis sharing,\npersistence, or cross-process coordination (like ", (0,jsx_runtime.jsx)(_components.code, {
          children: "least_connections"
        }), ", each\ninstance decides locally). Effective identity additionally includes resolved\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "base_url"
        }), ", credential, upstream model, and protocol; deadlines survive\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "SIGHUP"
        }), " for fingerprint-unchanged targets (algorithm and\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry_status_codes"
        }), " changes do not invalidate them), are dropped for\nremoved/changed targets, and are untouched by failed reloads. In-flight\nrequests admitted before advice arrives cannot be retroactively prevented."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Cooldown never marks providers unhealthy and never adds ", (0,jsx_runtime.jsx)(_components.code, {
          children: "429"
        }), " to\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry_status_codes"
        }), " on its own."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "provider-types",
      children: "Provider Types"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Provider type"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Behavior"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Notes"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "openai"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Pass-through OpenAI adapter"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Sends OpenAI-style requests upstream"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "openai-compatible"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Pass-through compatible adapter"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Requires ", (0,jsx_runtime.jsx)(_components.code, {
              children: "base_url"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "anthropic"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Translated provider-native adapter"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Supports chat and responses"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "gemini"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Translated provider-native adapter"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Supports chat, responses, and embeddings"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "opencode-zen"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Native or translated, per protocol"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Requires per-model ", (0,jsx_runtime.jsx)(_components.code, {
              children: "protocol"
            }), "; Zen service"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "opencode-go"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Native or translated, per protocol"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Requires per-model ", (0,jsx_runtime.jsx)(_components.code, {
              children: "protocol"
            }), "; Go service"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "github-copilot"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Pass-through chat-only adapter"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Device-flow login; ", (0,jsx_runtime.jsx)(_components.code, {
              children: "credential_ref"
            }), "; chat JSON/SSE only"]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai-compatible"
      }), ", the proxy stays close to pass-through behavior. For translated providers, the proxy maps between the public OpenAI-style contract and the provider-native request and response shape."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Pass-through providers preserve request JSON values and unknown extension fields, rewriting only the top-level ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " value before forwarding. Malformed JSON, non-object JSON bodies, and duplicate top-level ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " keys are rejected."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Translated providers intentionally support a conservative OpenAI-style request subset. Unsupported top-level controls such as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tools"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tool_choice"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "response_format"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "logprobs"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "parallel_tool_calls"
      }), ", and unknown extension fields are rejected with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "invalid_request"
      }), " instead of being silently dropped."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Translated chat completions support these top-level request fields: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "messages"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "max_tokens"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "temperature"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "top_p"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "stream"
      }), ". Message roles are limited to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "system"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "assistant"
      }), ", and content may be text or arrays of text parts."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Translated responses support these top-level request fields: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "input"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "instructions"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "max_output_tokens"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "temperature"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "top_p"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "stream"
      }), ". Input may be a string or an array of message items with text content."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Gemini translated embeddings support these top-level request fields: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "input"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "dimensions"
      }), ". Input may be a string or an array of strings."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "opencode-zen-and-go",
      children: "OpenCode Zen And Go"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-go"
      }), " share one adapter behind two explicit types.\nThe type selects the service, never the URL or credential:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-zen"
        }), " defaults to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "https://opencode.ai/zen/v1"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-go"
        }), " defaults to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "https://opencode.ai/zen/go/v1"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), " is an optional transport override only (same absolute-URL and\nloopback rules as other providers). An override never reclassifies the\nservice: auth, header, and protocol behavior stay type-driven."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Every model declares a required ", (0,jsx_runtime.jsx)(_components.code, {
        children: "protocol"
      }), ":"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "protocol"
            })
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Upstream request"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Serves public operations"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "POST <base>/chat/completions"
            }), ", model rewrite, JSON/SSE pass-through"]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            }), " only"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "POST <base>/responses"
            }), ", model rewrite, JSON/SSE pass-through"]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            }), " only"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "messages"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "POST <base>/messages"
            }), ", existing Messages translation subset"]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            }), " and ", (0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "gemini"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "POST <base>/models/<upstream>:generateContent"
            }), " (JSON) / ", (0,jsx_runtime.jsx)(_components.code, {
              children: ":streamGenerateContent?alt=sse"
            }), " (SSE); Zen only"]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            }), " and ", (0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            })]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["A public operation the model's protocol does not serve is rejected before any\nupstream I/O, as are ", (0,jsx_runtime.jsx)(_components.code, {
        children: "embeddings"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "images"
      }), ", and audio operations on both\nOpenCode types. The same model name may use different protocols per service\n(for example ", (0,jsx_runtime.jsx)(_components.code, {
        children: "minimax-m3"
      }), ", which is chat-protocol on Zen but\nmessages-protocol on Go), so routing comes from explicit configuration, never\nfrom the model name. There is no generic ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode"
      }), " type."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "session-and-client-requirements",
      children: "Session And Client Requirements"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Every upstream request sends ", (0,jsx_runtime.jsx)(_components.code, {
        children: "User-Agent: aiproxy/<version>"
      }), " unless the\nprovider declares a ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_agent"
      }), " override (supported on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-go"
      }), " only, validated as 1-256 printable ASCII characters); the\ninbound ", (0,jsx_runtime.jsx)(_components.code, {
        children: "User-Agent"
      }), " is never forwarded implicitly, so matching a first-party\nclient fingerprint is always an explicit operator choice. Both\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-go"
      }), " additionally send ", (0,jsx_runtime.jsx)(_components.code, {
        children: "x-opencode-session"
      }), " for\nprompt caching: a caller-supplied ", (0,jsx_runtime.jsx)(_components.code, {
        children: "x-opencode-session"
      }), " is forwarded as-is when it is 1-128\ncharacters of ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[A-Za-z0-9_-]"
      }), "; otherwise the caller's ", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Session-Id"
      }), " is adopted\nwhen valid, and only then does the proxy generate a fresh\nper-request ", (0,jsx_runtime.jsx)(_components.code, {
        children: "ses_"
      }), " + 128-bit hex ID. A caller-supplied ", (0,jsx_runtime.jsx)(_components.code, {
        children: "x-opencode-client"
      }), "\nis forwarded under the same validity rule and omitted otherwise. Missing or invalid values never fail the\nrequest and never create shared cross-client state. No other inbound headers\nor credentials are forwarded."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "quota-errors-failover-and-static-catalogs",
      children: "Quota Errors, Failover, And Static Catalogs"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Direct ", (0,jsx_runtime.jsx)(_components.code, {
        children: "<provider>/<model>"
      }), " requests never cross services: a request for\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "zen/glm-5.3"
      }), " either reaches Zen or fails with a clear error. Upstream Go\nquota/limit errors are returned to the client like any other upstream error;\nonly explicitly configured aliases retry another target, for example an alias\nspanning ", (0,jsx_runtime.jsx)(_components.code, {
        children: "zen"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "go"
      }), " targets. The upstream console setting that spends Zen\nbalance past Go limits is an account setting, not permission for the proxy to\nreroute requests. Model catalogs are static configuration validated at load;\nthe proxy performs no runtime catalog sync and advertises no universal model\nsupport beyond what is configured."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "github-copilot",
      children: "GitHub Copilot"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "github-copilot"
      }), " is a chat-only provider backed by a device-flow login. It\nserves ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/chat/completions"
      }), " (JSON and SSE); ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "embeddings"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "images"
      }), ", and audio are rejected before upstream I/O. The default origin is\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://api.githubcopilot.com"
      }), "; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), " is an optional transport override\nonly and never changes auth or header behavior."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Provisioning uses your own public OAuth client ID (no secret):"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy login github-copilot --client-id YOUR_GITHUB_OAUTH_CLIENT_ID --credential copilot-main\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The command prints the verification URI and user code, waits for\nauthorization, then writes ", (0,jsx_runtime.jsx)(_components.code, {
        children: "<secrets-dir>/copilot-<name>.json"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "0600"
      }), ")\nwithout editing HCL or signaling a server. It is headless-friendly over SSH:\ncopy the URI/code to a browser, authorize, and return. Never reuse another\napplication's client ID."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Reference the saved login from config:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"github-copilot\" \"copilot\" {\n  credential_ref {\n    name = \"copilot-main\"\n  }\n\n  model \"gpt-5.4-nano\" {}\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref.path"
      }), " defaults to the shared secrets path. Derived Copilot\nproviders require their own local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref"
      }), ". The token activates on\nrestart/", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), "; sidecar-only changes are inert until reload, failed reloads\nkeep the old runtime, and upstream ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), " means re-run ", (0,jsx_runtime.jsx)(_components.code, {
        children: "login"
      }), " with the\nsame client ID/name and reload. Upstream headers are an allowlist only\n(", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization"
      }), " from the stored login, proxy ", (0,jsx_runtime.jsx)(_components.code, {
        children: "User-Agent"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-GitHub-Api-Version"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Openai-Intent"
      }), ", derived ", (0,jsx_runtime.jsx)(_components.code, {
        children: "x-initiator: user"
      }), ", vision\nonly on image bodies); inbound auth/cookies/", (0,jsx_runtime.jsx)(_components.code, {
        children: "x-api-key"
      }), "/caller Copilot\nmetadata are stripped. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "GET {base}/models"
      }), " listing shares the same auth\nwithout changing the static proxy inventory. See ", (0,jsx_runtime.jsx)(_components.code, {
        children: "examples/github-copilot.hcl"
      }), "\nfor a complete config."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "model-capabilities",
      children: "Model Capabilities"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Capabilities describe which proxy operations a model may serve."
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Supported values:"
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
      children: ["If ", (0,jsx_runtime.jsx)(_components.code, {
        children: "capabilities"
      }), " is omitted, the proxy derives defaults from the provider type and then enforces operation support at request time."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Provider type"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Default capabilities when omitted"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Additional supported capabilities"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "openai"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "embeddings"
            })]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "images"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "audio_transcriptions"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "audio_speech"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "openai-compatible"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "embeddings"
            })]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "images"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "audio_transcriptions"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "audio_speech"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "anthropic"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "None"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "gemini"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "embeddings"
            })
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "opencode-zen"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            }), ", or both (by protocol)"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "None"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "opencode-go"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "responses"
            }), ", or both (by protocol)"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "None"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "github-copilot"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "chat"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "None"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Set explicit capabilities when you want the public catalog to reflect a narrower contract than the provider's default behavior, or to opt into one of the additional supported capabilities for that provider type. On ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-go"
      }), " the omitted default is protocol-aware (", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), " serves ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), " serves ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "messages"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " serve both), and capabilities outside the protocol-served set fail validation."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.h2, {
      id: "get-v1models-metadata",
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "GET /v1/models"
      }), " Metadata"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The model catalog includes both direct models and aliases."
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "For direct models, the response includes metadata such as:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "display_name"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "provider_type"
        })
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["effective ", (0,jsx_runtime.jsx)(_components.code, {
          children: "capabilities"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "For aliases, the response includes:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["effective ", (0,jsx_runtime.jsx)(_components.code, {
          children: "capabilities"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "alias_targets"
        }), " summaries with provider, model, and resolved display name"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Alias capabilities are the intersection of every target's capabilities. That means an alias only advertises operations that all of its targets can safely serve."
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