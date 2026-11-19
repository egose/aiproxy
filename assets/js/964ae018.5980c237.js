"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[443],{

/***/ 2700
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_api_reference_md_964_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-api-reference-md-964.json
const site_docs_api_reference_md_964_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"api-reference","title":"API Reference","description":"aiproxy exposes an OpenAI-compatible HTTP API.","source":"@site/docs/api-reference.md","sourceDirName":".","slug":"/api-reference","permalink":"/docs/api-reference","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":5,"frontMatter":{"sidebar_position":5},"sidebar":"docsSidebar","previous":{"title":"Request Examples","permalink":"/docs/request-examples"},"next":{"title":"Operations","permalink":"/docs/operations"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/api-reference.md


const frontMatter = {
	sidebar_position: 5
};
const contentTitle = 'API Reference';

const assets = {

};



const toc = [{
  "value": "Endpoint And Provider Support Matrix",
  "id": "endpoint-and-provider-support-matrix",
  "level": 2
}, {
  "value": "Provider Capability Defaults",
  "id": "provider-capability-defaults",
  "level": 2
}, {
  "value": "Streaming",
  "id": "streaming",
  "level": 2
}, {
  "value": "Direct Model-ID Passthrough",
  "id": "direct-model-id-passthrough",
  "level": 2
}, {
  "value": "Peer Deferred Transport v1",
  "id": "peer-deferred-transport-v1",
  "level": 2
}, {
  "value": "Peer Routes And Recognition",
  "id": "peer-routes-and-recognition",
  "level": 3
}, {
  "value": "Peer JSON Schema",
  "id": "peer-json-schema",
  "level": 3
}, {
  "value": "Peer State And Byte Delivery",
  "id": "peer-state-and-byte-delivery",
  "level": 3
}, {
  "value": "Acceptance And Deduplication",
  "id": "acceptance-and-deduplication",
  "level": 3
}, {
  "value": "Peer Control Errors",
  "id": "peer-control-errors",
  "level": 3
}, {
  "value": "Authentication",
  "id": "authentication",
  "level": 2
}, {
  "value": "Model Access With Multi-tenancy",
  "id": "model-access-with-multi-tenancy",
  "level": 3
}, {
  "value": "Invitation Onboarding",
  "id": "invitation-onboarding",
  "level": 3
}, {
  "value": "SSO Admission",
  "id": "sso-admission",
  "level": 3
}, {
  "value": "Workspace Model",
  "id": "workspace-model",
  "level": 3
}, {
  "value": "Workspace Catalog References",
  "id": "workspace-catalog-references",
  "level": 3
}, {
  "value": "Direct Catalog Writes",
  "id": "direct-catalog-writes",
  "level": 3
}, {
  "value": "Workspace And Team Membership Changes",
  "id": "workspace-and-team-membership-changes",
  "level": 3
}, {
  "value": "Account Deletion And Personal Bearers",
  "id": "account-deletion-and-personal-bearers",
  "level": 3
}, {
  "value": "Atomic Key Issuance And Bearer Recovery",
  "id": "atomic-key-issuance-and-bearer-recovery",
  "level": 3
}, {
  "value": "Direct Key Writes",
  "id": "direct-key-writes",
  "level": 3
}, {
  "value": "Key Policy And Sharing Updates",
  "id": "key-policy-and-sharing-updates",
  "level": 3
}, {
  "value": "Usage And Scope",
  "id": "usage-and-scope",
  "level": 2
}, {
  "value": "Error Behavior",
  "id": "error-behavior",
  "level": 2
}, {
  "value": "Administrative Provider And Alias Saves",
  "id": "administrative-provider-and-alias-saves",
  "level": 2
}, {
  "value": "Tenant Provider Credential And Destination Policy",
  "id": "tenant-provider-credential-and-destination-policy",
  "level": 3
}, {
  "value": "Managed Catalog Dependency Edits",
  "id": "managed-catalog-dependency-edits",
  "level": 3
}, {
  "value": "Encrypted Reasoning Policy Intent",
  "id": "encrypted-reasoning-policy-intent",
  "level": 3
}, {
  "value": "Provider TLS ClientHello Settings",
  "id": "provider-tls-clienthello-settings",
  "level": 3
}, {
  "value": "GitHub Copilot Device Authorization",
  "id": "github-copilot-device-authorization",
  "level": 2
}, {
  "value": "Provider Coverage Notes",
  "id": "provider-coverage-notes",
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
        id: "api-reference",
        children: "API Reference"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " exposes an OpenAI-compatible HTTP API."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This page focuses on the proxy-facing contract and operation coverage. It does not attempt to restate every upstream provider-specific field or option."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "endpoint-and-provider-support-matrix",
      children: "Endpoint And Provider Support Matrix"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Surface"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "openai"
            })
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "openai-compatible"
            })
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "anthropic"
            })
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "gemini"
            })
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "opencode-zen"
            })
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "opencode-go"
            })
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "github-copilot"
            })
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "zenmux"
            })
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "openrouter"
            })
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET /v1/models"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET /v1/billing/usage"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned local usage accounting"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned local usage accounting"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned local usage accounting"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned local usage accounting"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned local usage accounting"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned local usage accounting"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned local usage accounting"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned local usage accounting"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned local usage accounting"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET /metrics"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned Prometheus metrics"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned Prometheus metrics"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned Prometheus metrics"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned Prometheus metrics"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned Prometheus metrics"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned Prometheus metrics"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned Prometheus metrics"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned Prometheus metrics"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Proxy-owned Prometheus metrics"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /v1/chat/completions"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE translated"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE translated"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE native or translated subset"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE native or translated subset"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /v1/messages"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE native (messages protocol only)"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE native (messages protocol only)"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /v1/embeddings"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /v1/responses"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE translated subset"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE translated subset"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE native or translated subset"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE native or translated subset"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /v1/images/generations"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /v1/audio/transcriptions"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /v1/audio/speech"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "provider-capability-defaults",
      children: "Provider Capability Defaults"
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
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "messages"
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
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "messages"
            }), ", or more (by protocol)"]
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
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "messages"
            }), ", or more (by protocol)"]
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
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "zenmux"
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
              children: "openrouter"
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
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When a model omits ", (0,jsx_runtime.jsx)(_components.code, {
        children: "capabilities"
      }), ", the provider-type defaults are used. Explicit\ncapabilities can narrow that default or opt into an additional supported\ncapability listed above. On ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-zen"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "opencode-go"
      }), " the omitted\ndefault is protocol-aware: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), " models default to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), " models\nto ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "messages"
      }), " models to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "messages"
      }), ", and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " (Zen only) models to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chat"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "streaming",
      children: "Streaming"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Streaming uses OpenAI-compatible Server-Sent Events, except ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/messages"
      }), "\nwhich passes Anthropic SSE through verbatim."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/chat/completions"
        }), " supports JSON and SSE streaming"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/responses"
        }), " supports JSON and SSE streaming where implemented"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/messages"
        }), " supports JSON and SSE (Anthropic shape, Anthropic-family targets only)"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "translated providers map their upstream streaming format back into OpenAI-compatible SSE chunks"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "If an upstream stream fails after partial output, the proxy terminates the stream instead of fabricating a full JSON response."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "direct-model-id-passthrough",
      children: "Direct Model-ID Passthrough"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "<provider>/<upstream-id>"
      }), " in the ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " field when an eligible direct\nrequest can use a ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/configuration#model-id-passthrough",
        children: "model-ID passthrough rule"
      }), ".\nExact literal models take priority. Rule matches use the existing adapter,\ndestination, credential policy and ingress guardrails. Provider/root\nencrypted-reasoning policy still applies to chat and Responses where supported,\nnot native Messages. Rules add no public operations, do not contact discovery,\nand do not use peer transport."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["A rule call requires exactly one top-level ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " string in a JSON object,\nor one text form field named ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " in multipart audio transcriptions. Use the\nexact lowercase field name and the unmodified ID, with no duplicates. Top-level\nJSON keys or multipart fields named ", (0,jsx_runtime.jsx)(_components.code, {
        children: "models"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "preset"
      }), " (including case\nvariants) are rejected on all rule calls. On native ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/messages"
      }), " rule\ncalls only, top-level ", (0,jsx_runtime.jsx)(_components.code, {
        children: "fallbacks"
      }), " (including case variants) is also rejected.\nThese checks run before upstream I/O, including after guardrail rewrites.\nLiteral and alias payload handling and existing translation limits are unchanged."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Body-based adapters use the admitted suffix as the upstream ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " value.\nGemini and the Zen ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " protocol use a single bare ID segment in the\nadapter's fixed model path. Only the adapter adds ", (0,jsx_runtime.jsx)(_components.code, {
        children: "alt=sse"
      }), " for streaming.\nFor Gemini batched embeddings, send the normal OpenAI-style array of strings\nin ", (0,jsx_runtime.jsx)(_components.code, {
        children: "input"
      }), "; the adapter sets every upstream ", (0,jsx_runtime.jsx)(_components.code, {
        children: "requests[].model"
      }), " to\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "models/<admitted-id>"
      }), ", matching the ", (0,jsx_runtime.jsx)(_components.code, {
        children: ":batchEmbedContents"
      }), " path. Callers cannot\nprovide their own subrequest model. Literal and alias batches are unchanged."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The proxy does not verify that the upstream offers the ID, how it interprets\nvirtual IDs or other parameters, or what it actually runs or charges. Use a\ntrusted upstream and enforce model and spending restrictions there as well."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "peer-deferred-transport-v1",
      children: "Peer Deferred Transport v1"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["This private opt-in extension serves chat completions between a statically\nconfigured caller and executor. It adds ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "no provider/model capability"
      }), " and is\nnot Batch or Responses background mode. Ordinary clients keep\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /v1/chat/completions"
      }), ", JSON/SSE and their original ", (0,jsx_runtime.jsx)(_components.code, {
        children: "stream"
      }), " choice; they do\nnot receive peer receipts, cursors or private headers. Other operations keep\nordinary transport. See ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/configuration#peer-deferred-transport-v1",
        children: "configuration"
      }), "\nand ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#peer-deferred-transport-v1",
        children: "operational limits"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Caller chat transport follows the resolved model's local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_transport"
      }), ",\notherwise its provider's local compatibility default, otherwise ordinary\ntransport. A model block replaces the whole default, including when empty or\npartial. Literal direct and alias-target requests use the same precedence. The\nexecutor's ", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_receiver"
      }), " is provider-only and additive: it permits private\nadmission without forcing ordinary calls for the same models into jobs. Model\nplacement changes neither these v1 routes/schema nor the public support/capability\nmatrices."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "peer-routes-and-recognition",
      children: "Peer Routes And Recognition"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The executor serves the unmounted prefix ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/_internal/deferred/v1"
      }), ". Private paths\nare intercepted before web UI fallback. All controls require inference bearer\nauthentication and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Deferred-Version: 1"
      }), "; job controls additionally\nrequire one ", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Deferred-Generation"
      }), " header matching their receipt.\nSubmit carries the generation in its envelope."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.th, {
            children: ["Method / path under ", (0,jsx_runtime.jsx)(_components.code, {
              children: "/_internal/deferred/v1"
            })]
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Success status / body"
          }), (0,jsx_runtime.jsxs)(_components.th, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "Allow"
            }), " on a wrong method"]
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET /capabilities?model=<public-model>"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "200 Capability"
            }), "; current model/receiver eligibility, no inference quota charge"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET"
            })
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /submit"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "202 Receipt"
            }), " after bounded admission/scheduling, or accepted dedup replay"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST"
            })
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET /jobs/{id}"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "200 JobInfo"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET, DELETE"
            })
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET /jobs/{id}/result"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "202 JobInfo"
            }), " while active; ", (0,jsx_runtime.jsx)(_components.code, {
              children: "200 ResultManifest"
            }), " when terminal; no body bytes here"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET"
            })
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET /jobs/{id}/read?cursor=<offset>&limit=<bytes>"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "200 ReadFrame"
            }), ", immediate finite snapshot; required cursor, optional limit 1–65536, default 65536 bytes"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET"
            })
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "DELETE /jobs/{id}"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Empty ", (0,jsx_runtime.jsx)(_components.code, {
              children: "204"
            }), "; cancel-if-running and release, including repeated known release"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET, DELETE"
            })
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Only submit has a request body, with\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "Content-Type: application/vnd.aiproxy.deferred+json"
      }), ". Query fields must be\nrecognized and unique; submit has no query. Every recognized response carries:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-http",
        children: "Content-Type: application/vnd.aiproxy.deferred+json\nX-Aiproxy-Deferred-Version: 1\nCache-Control: private, no-store\nPragma: no-cache\nVary: Authorization\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The empty ", (0,jsx_runtime.jsx)(_components.code, {
        children: "204"
      }), " still carries these headers. Recognition requires the dedicated\npath, exact media/version markers, matching envelope and expected identity,\n", (0,jsx_runtime.jsxs)(_components.strong, {
        children: ["not just HTTP ", (0,jsx_runtime.jsx)(_components.code, {
          children: "202"
        })]
      }), ". The caller validates a positive capability for its\nconfigured executor public model before any submit. Missing/wrong-version\ncapabilities cause zero submits. An unrecognized submit response is\nindeterminate, never permission to use synchronous fallback or a new attempt."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For a caller ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), ", remove one terminal ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/v1"
      }), " segment and append the private\nprefix: ", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://executor.example/mount/v1"
      }), " becomes\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://executor.example/mount/_internal/deferred/v1"
      }), ". An external mount must\nstrip ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/mount"
      }), " before forwarding to the executor. Query strings, fragments, dot\nsegments, backslashes and encoded path separators are invalid in a deferred\ncaller base URL. Control URLs are derived locally, never from arbitrary peer\nlinks. Submit's ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Location"
      }), " must match the expected origin, mount and job path\n(the exact absolute URL or path). ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "All control redirects are forbidden."
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For the ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/configuration#full-caller-configuration",
        children: "full configuration examples"
      }), ",\ncapability discovery is:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-http",
        children: "GET /_internal/deferred/v1/capabilities?model=alias%2Fpeer_chat HTTP/1.1\nHost: executor.example\nAuthorization: Bearer <peer-token>\nAccept: application/vnd.aiproxy.deferred+json\nX-Aiproxy-Deferred-Version: 1\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Substitute the configured peer token; use the returned generation/ticket and\nreceipt ID, not invented identifiers, for subsequent controls. A read request is:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-http",
        children: "GET /_internal/deferred/v1/jobs/<receipt-id>/read?cursor=0&limit=65536 HTTP/1.1\nHost: executor.example\nAuthorization: Bearer <peer-token>\nAccept: application/vnd.aiproxy.deferred+json\nX-Aiproxy-Deferred-Version: 1\nX-Aiproxy-Deferred-Generation: <receipt-generation>\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "peer-json-schema",
      children: "Peer JSON Schema"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Objects below use flattened fields, not wrapper objects named ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Receipt"
      }), " or\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "JobInfo"
      }), ". Every envelope has ", (0,jsx_runtime.jsx)(_components.code, {
        children: "protocol: \"aiproxy-deferred\""
      }), " and numeric\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "version: 1"
      }), ". Unknown/duplicate fields and numeric overflow are rejected.\nTimestamps are integer Unix milliseconds; durations are integer milliseconds.\nOptional fields are omitted when absent."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Object"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Required fields in addition to the common envelope"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "Capability"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "generation"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "public_model"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "submit_token"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "submit_until_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "limits"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "poll_after_ms"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "SubmitRequest"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "generation"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "submit_token"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "attempt_key"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "headers"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "body"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "Receipt"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "generation"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "id"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "public_model"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "attempt_key"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "scope_digest"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "fingerprint"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "state"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "execution_deadline_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "dedup_until_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "poll_after_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "replayed"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "JobInfo"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["All receipt fields plus ", (0,jsx_runtime.jsx)(_components.code, {
              children: "available_bytes"
            }), "; optional ", (0,jsx_runtime.jsx)(_components.code, {
              children: "response"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "outcome"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "retain_until_ms"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "ResultManifest"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["All job-info fields plus ", (0,jsx_runtime.jsx)(_components.code, {
              children: "body_sha256"
            }), "; terminal only"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "ReadFrame"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["All job-info fields plus ", (0,jsx_runtime.jsx)(_components.code, {
              children: "cursor"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "next_cursor"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "data"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "done"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "ControlError"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "error"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "acceptance"
            }), "; optional ", (0,jsx_runtime.jsx)(_components.code, {
              children: "poll_after_ms"
            })]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Nested object"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Fields / meaning"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "limits"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "request_bytes"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "result_bytes"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "execution_timeout_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "retention_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "control_timeout_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "finalization_timeout_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "read_bytes"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "response"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "status_code"
            }), " (200–599), ", (0,jsx_runtime.jsx)(_components.code, {
              children: "headers"
            }), " (name → string array), ", (0,jsx_runtime.jsx)(_components.code, {
              children: "streaming"
            }), " (boolean); optional ", (0,jsx_runtime.jsx)(_components.code, {
              children: "retry"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "retry"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "observed_at_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "deadline_ms"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "remaining_ms"
            }), "; original inference retry timing, not control poll advice"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "outcome"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "kind"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "usage"
            }), "; optional ", (0,jsx_runtime.jsx)(_components.code, {
              children: "error"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "usage"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Nonnegative observed integers: ", (0,jsx_runtime.jsx)(_components.code, {
              children: "prompt_tokens"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "completion_tokens"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "total_tokens"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "cached_tokens"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "cache_creation_tokens"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "cache_read_tokens"
            }), "; no estimates"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "error"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "type"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "message"
            }), "; optional ", (0,jsx_runtime.jsx)(_components.code, {
              children: "block_id"
            })]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "body"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "data"
      }), " are non-null standard JSON base64 strings; empty bytes are\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"\""
      }), ". The decoded submit body is a JSON object with one required ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " string\nand zero/one boolean ", (0,jsx_runtime.jsx)(_components.code, {
        children: "stream"
      }), "; other inference fields are retained. It contains\nthe caller adapter's model rewrite, then undergoes executor admission and\nexisting adapter processing. The bounded ", (0,jsx_runtime.jsx)(_components.code, {
        children: "headers"
      }), " bag has JSON content type\nand safe prepared inference metadata, never credentials, cookies, hop-by-hop or\nprivate deferred headers."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "cursor"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "next_cursor"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "available_bytes"
      }), " are ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "strings"
      }), " containing canonical\nunsigned decimal byte offsets/counts, with no sign or leading zeros except ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"0\""
      }), ".\nGeneration and job IDs are canonical unpadded base64url encodings of 16 random\nbytes. Attempt keys are 22–64 base64url characters, generated with at least\n128 random bits. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "scope_digest"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "fingerprint"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "body_sha256"
      }), " are lowercase\nSHA-256 hex strings. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "public_model"
      }), " is at most 512 bytes."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Header bags allow at most 64 case-insensitively distinct names, 128 values total\nand 4096 bytes/value. Non-data serialized metadata is at most 16 KiB; error detail\nis at most 1 KiB. Finite control responses are at most 128 KiB, reads at most\n64 KiB decoded data, and submission tickets at most 512 bytes. Submit input has\nits separate decoded request ceiling plus bounded base64/metadata scratch;\nthe 128 KiB response ceiling is not a submit input ceiling."
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "peer-state-and-byte-delivery",
      children: "Peer State And Byte Delivery"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "State"
          }), (0,jsx_runtime.jsxs)(_components.th, {
            children: ["Terminal ", (0,jsx_runtime.jsx)(_components.code, {
              children: "outcome.kind"
            })]
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Meaning"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "running"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Absent"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["No selected response; reads can have empty ", (0,jsx_runtime.jsx)(_components.code, {
              children: "data"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "headers_ready"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Absent"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Final selected adapter head and incremental bytes, not an intermediate alias attempt"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "completed"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "response"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Complete normalized adapter result, including provider HTTP errors"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "failed"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "upstream_error"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "invalid_request"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "execution_timeout"
            }), " or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "limit_exceeded"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Known failed execution; an exact streamed prefix may be retained"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "cancelled"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "cancelled"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Local cancellation, not proof external execution/billing stopped"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Released/expired tombstones are not result states and disclose no retained head\nor body. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "response"
      }), " is the original selected status and filtered headers after\nexisting adapter/non-JSON-error normalization, with private headers stripped.\nA terminal provider ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " is a ", (0,jsx_runtime.jsx)(_components.code, {
        children: "completed"
      }), " / ", (0,jsx_runtime.jsx)(_components.code, {
        children: "response"
      }), " outcome, not a peer\ncontrol ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Each read atomically captures state/head/outcome/available tail and one finite\nbyte slice. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "next_cursor = cursor + len(decoded_data)"
      }), ". A cursor beyond that\ncaptured tail is ", (0,jsx_runtime.jsx)(_components.code, {
        children: "400 invalid_cursor"
      }), ". ", (0,jsx_runtime.jsx)(_components.code, {
        children: "done"
      }), " is true only when terminal and\nthe frame reaches the final byte; an empty pending frame or HTTP EOF is not\ncompletion. Decode and validate the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "whole transfer"
      }), " before delivering bytes\nand advancing the cursor; discard partial transfers and retry the unchanged\ncursor."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The terminal manifest hashes all final retained body bytes, including an empty\nbody or failed prefix. Unary callers wait for that manifest. Streaming callers\ndeliver the existing adapter/native bytes incrementally without synthetic SSE\nevents, IDs, heartbeats or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[DONE]"
      }), "; terminal validation cannot retract already\ndelivered bytes. After downstream commitment, errors terminate the stream rather\nthan selecting another inference. This is peer byte recovery, not client-session\nresume."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "acceptance-and-deduplication",
      children: "Acceptance And Deduplication"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Capabilities issue a stateless signed ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "30-second"
      }), " ticket binding protocol,\nprocess generation, owner/model digests, nonce, expiry and advertised limits.\nQualified dedup identity is ", (0,jsx_runtime.jsx)(_components.code, {
        children: "(generation, SHA256(ticket), owner, attempt_key)"
      }), ".\nThe fingerprint covers fixed operation/model, exact decoded body and canonical\nheader bag, not mutable credentials or post-guardrail output."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The same fingerprint replays an accepted receipt without repeating inference\nadmission, rate/quota charges or guardrails. A conflict returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "409"
      }), "; a still\npreparing duplicate returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "409 submission_in_progress"
      }), ". Preparation rejections\nare cached through the ticket window. Accepted records/tombstones remain protected\nthrough ", (0,jsx_runtime.jsx)(_components.code, {
        children: "dedup_until_ms = submit_until_ms + execution_timeout_ms + retention_ms + finalization_timeout_ms"
      }), ". Existing accepted entries can replay after ticket\nexpiry; absent expired scopes or old generations dispatch nothing."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "With multi-tenancy, discovery and new admissions use the bearer credential's\nworkspace. Accepted replays and job controls require the same admitted\ncredential identity and workspace, and still check the current allowlist for\nthe admitted public model. They do not use a replacement catalog to re-resolve\naccepted work. No workspace fields or headers are added to the peer protocol."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Once submit begins, the caller freezes generation/ticket/key/prepared input.\nLost acknowledgments retry only that qualified submit, never a fresh capability\nor key. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "acceptance"
      }), " is ", (0,jsx_runtime.jsx)(_components.code, {
        children: "not_accepted"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "accepted"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "indeterminate"
      }), "; later\nauth/control failures never downgrade an accepted/indeterminate logical attempt.\nMissing restart evidence means unavailable/indeterminate, not invented proof of\nexpiry, cancellation or exactly-once external work."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "peer-control-errors",
      children: "Peer Control Errors"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Protocol errors use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "ControlError"
      }), ". Existing ordinary inference admission\nrejections instead preserve their ordinary ", (0,jsx_runtime.jsx)(_components.code, {
        children: "{\"error\": ...}"
      }), " shape, status/type\nand optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "block_id"
      }), ", without protocol fields. Neither is an inference\nsuccess or provider outcome."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Status / type"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Meaning"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "400"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Invalid envelope, body, headers, model/receiver eligibility or query; ", (0,jsx_runtime.jsx)(_components.code, {
              children: "invalid_cursor"
            }), " for invalid reads"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "401 auth_failed"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "403 forbidden"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Current inference authentication / admitted-model authorization failed; no result disclosure"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "404 model_not_found"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Model or alias unavailable to the caller, including missing names and out-of-workspace inventory"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "404 job_unavailable"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Unknown or different-owner job, including unknown DELETE"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "404 deferred_disabled"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "New deferred admission is disabled"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "405 method_not_allowed"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Recognized route, unsupported method; carries ", (0,jsx_runtime.jsx)(_components.code, {
              children: "Allow"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "409 attempt_conflict"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "409 submission_in_progress"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Conflicting qualified request / preparation still active"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "410 job_released"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "410 job_expired"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Protected release / retention expiry"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "410 generation_mismatch"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "410 submit_scope_expired"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Old generation / absent expired submission scope; zero dispatch"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "413 request_too_large"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Oversized decoded/wire submit input"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "426 unsupported_deferred_version"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Wrong path version or missing/wrong version header"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "429 deferred_owner_limit"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "429 deferred_control_limited"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Owner admission / control pressure"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "503 deferred_saturated"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Global admission/resource pressure"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The ordinary caller sees standard JSON errors before downstream commitment:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Caller status / error type"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Meaning"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "502 deferred_protocol_error"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Capability/version/marker/schema/identity or terminal-hash validation failed"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "502 deferred_submit_indeterminate"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Unacknowledged submission uncertainty; no fresh inference"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "502 deferred_request_lost"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Accepted work became unavailable; no fresh inference"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "502 deferred_control_error"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Control failure or recovery budgets exhausted"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "504 deferred_wait_timeout"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Overall caller wait expired"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "504 deferred_execution_timeout"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Fixed executor job deadline expired"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "502 deferred_limit_exceeded"
            }), " / ", (0,jsx_runtime.jsx)(_components.code, {
              children: "502 deferred_cancelled"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Bounded execution failed / observable local cancellation"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Definite terminal provider HTTP/transport outcomes ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "before downstream\ncommitment"
      }), " still use ordinary inference retry rules. Peer errors, polling\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "Retry-After"
      }), ", admission pressure and accepted/indeterminate retrieval failures\ndo not trigger alias/encrypted-reasoning inference retries, provider-health\nmutation or model cooldown. After commitment, failure only terminates the stream.\nSee ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#ownership-cancellation-and-release",
        children: "ownership and cleanup"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "authentication",
      children: "Authentication"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When ", (0,jsx_runtime.jsx)(_components.code, {
        children: "bearer_static"
      }), " auth is enabled, requests must send:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-http",
        children: "Authorization: Bearer <token>\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When ", (0,jsx_runtime.jsx)(_components.code, {
        children: "none"
      }), " auth is enabled and multi-tenancy is disabled, inference does not\nauthenticate incoming requests. With multi-tenancy enabled, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "none"
      }), " is\nmanagement-only. Model discovery, inference, billing usage, and the private\ndeferred API return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), " without a valid inference bearer with workspace scope.\nBrowser login JWTs and workspace-selection headers are not inference credentials."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["In ", (0,jsx_runtime.jsx)(_components.code, {
        children: "bearer_static"
      }), " mode, configured clients and database keys each have their\nown ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), " policy and request rate-limit bucket (if configured).\nMatching names or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tenant"
      }), " labels do not share policy or capacity. No bearer\nheader or token format changes are required."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "model-access-with-multi-tenancy",
      children: "Model Access With Multi-tenancy"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "GET /v1/models"
      }), " lists only literal models and aliases the key can use in its own\nworkspace. An empty workspace returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "{\"object\":\"list\",\"data\":[]}"
      }), ". HCL\nproviders, models and aliases are System-only, and HCL client tokens are scoped\nto System. A database key is scoped to its owning workspace regardless of the\nissuer's roles."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), " narrows the workspace by exact public name; omitting it or\nsetting ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[]"
      }), " never grants access to another workspace. For literal resolution a\nname excluded by the allowlist returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "403 forbidden"
      }), " regardless of whether\nit exists. Otherwise missing and out-of-scope names return the same\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "404 model_not_found"
      }), ". An alias with a missing or out-of-workspace target is\nentirely unavailable: it is not listed, and no part of its pool is tried. A\nderived provider whose base is missing or belongs to another workspace is\nlikewise unavailable. Allowing an alias does not require listing its target\nnames in ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), ", but cannot widen workspace scope."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Only a System static bearer with a separate ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_model_passthroughs"
      }), " grant\ncan use a passthrough rule in hosted mode. Even a System DB key with the ID in\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), " cannot use one. Rule matches do not appear in ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/v1/models"
      }), ";\nrequests that cannot use a rule keep the same ", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "404"
      }), " as a literal miss.\nExisting authentication, workspace and database freshness checks still apply."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "invitation-onboarding",
      children: "Invitation Onboarding"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The signed-out browser route ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/invite"
      }), " works with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allow_public_registration = false"
      }), ". Open the clean ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/invite"
      }), " page on your trusted deployment origin and\npaste the one-time code delivered separately by your inviter. Invitation codes\nare not read from URLs, including query strings and fragments. Treat previously\nshared code-bearing links as exposed: ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#dashboard-web-ui-and-multi-tenancy",
        children: "revoke and reissue pending invitations"
      }), ".\nOpening the form does not erase URLs that were already visited."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The app keeps redemption form values in component memory, not in browser\nstorage or React Query mutation/query state. The UI uses a ", (0,jsx_runtime.jsx)(_components.code, {
        children: "no-referrer"
      }), " policy.\nReloading or leaving the form discards the app's copy of the code. After\nacceptance, sign in with the invited email and new password. The portal selects\nthe authorized invitation workspace, not System or a stale workspace saved by a\ndifferent session."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "POST /_internal/admin/invites"
      }), " returns the one-time code as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "token"
      }), ". Deliver it\nseparately over a trusted channel. With multi-tenancy enabled,\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /_internal/admin/invites/accept"
      }), " accepts the code only as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "token"
      }), " in the\nJSON body, with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "password"
      }), " (at least eight characters). A successful ", (0,jsx_runtime.jsx)(_components.code, {
        children: "201"
      }), "\ncreates the account, adds the invitation's workspace membership when present,\nand consumes the invitation in one database transaction. Email, system-admin\nauthority, workspace, and workspace role come from the current stored\ninvitation; an omitted or unrecognized workspace role defaults to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "member"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Acceptance is complete-or-no-change. Revocation, expiry, and prior acceptance are\nchecked again at consumption, including after waiting for another acceptance.\nOnly one competing request can consume an eligible invitation. Missing, revoked,\nor replaced tokens return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "404"
      }), "; expired/already accepted invitations and an\nalready registered email return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "400"
      }), ". Database failures return a controlled\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " without database details. An account or membership write failure rolls back\nthe whole acceptance, leaving an otherwise eligible invitation available to retry\nafter the cause is resolved. Successful onboarding supports the normal login flow.\nAcceptance also serializes with workspace deletion. If the stored invitation's\nworkspace changes while acceptance is acquiring locks, it returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "400"
      }), " with\nretry guidance and saves nothing; retry uses the current invitation scope.\nInvitation creation and acceptance responses use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Cache-Control: no-store"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "sso-admission",
      children: "SSO Admission"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["OIDC is ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "existing accounts only"
      }), ", including invitees after redemption creates\ntheir accounts. A pending invitation alone is not admission.\nVerified IdP identities without an existing local account receive an error\nredirect, never a user, workspace or refresh session. This rule also applies\nwhen public password registration is enabled. A user lookup database error is\ndistinct from a missing account and fails closed without database details.\nDisabled accounts cannot sign in. Returning accounts keep their memberships;\nSSO does not create personal workspaces or enroll tenant users into System.\nExisting configured admin-claim mapping still applies to admitted accounts;\nit is not authority to provision an unknown identity."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "workspace-model",
      children: "Workspace Model"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Tenancy is built on workspaces. Tables are ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspaces"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_members"
      }), ",\nand ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_teams"
      }), "; owning rows carry ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_id"
      }), ". Admin routes live under\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "/_internal/admin/workspaces"
      }), ", resource selection accepts ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_id"
      }), " or the\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Workspace-ID"
      }), " header, and mutable fields use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_id"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_role"
      }), ",\nand ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_name"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Kinds are ", (0,jsx_runtime.jsx)(_components.code, {
        children: "personal"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "organization"
      }), ", plus a ", (0,jsx_runtime.jsx)(_components.code, {
        children: "system"
      }), " flag:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "personal"
        }), ": exactly one per user, auto-created at registration. It holds only\nits owner, so teams and additional members are rejected."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "organization"
        }), " (kind): created afterwards via ", (0,jsx_runtime.jsx)(_components.code, {
          children: "POST /_internal/admin/workspaces"
        }), "\nfor collaboration with members and teams."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "system"
        }), ": the built-in workspace (", (0,jsx_runtime.jsx)(_components.code, {
          children: "is_system"
        }), ") that owns config-backed\ninventory. Global administrators and explicit System members can read that\ninventory through the management API. This does not grant System\nadministration, and System cannot be deleted."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The provider, alias, and key management lists and detail reads apply workspace\naccess before returning metadata. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_id"
      }), " takes precedence over\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Workspace-ID"
      }), "; either can narrow access, not grant it. An inaccessible\nworkspace or resource returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "404"
      }), ". Workspace and invitation lists cover the\ncaller's authorized workspaces; only global administrators see invitations\nwithout a workspace. A caller with no memberships and no global authority gets\nempty unfiltered lists. If a required membership lookup fails, these reads\nreturn ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " without inventory or database error details."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "workspace-catalog-references",
      children: "Workspace Catalog References"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider inheritance and every alias target must belong to the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "destination\nworkspace"
      }), ", not merely a workspace the author can administer. This applies to\nprovider/alias POST and PUT, including alias ", (0,jsx_runtime.jsx)(_components.code, {
        children: "providers"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " shorthand and\nomitted targets retained on update. A global administrator creating a tenant\nresource cannot use a System or another tenant's base or models. Config-backed\nproviders, models and aliases belong only to System; a managed System resource\nmay reference a compatible System base, subject to the ordinary inheritance\nrestrictions."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Missing and foreign references return the same controlled ", (0,jsx_runtime.jsx)(_components.code, {
        children: "400"
      }), ", without\nforeign provider types, models, credentials or enabled-state metadata. Rejected\nedits persist nothing and request no activation."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider and alias names remain globally unique, including disabled providers.\nStartup, reload and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "validate --check-db"
      }), " reject cross-workspace graphs and\nstatic/managed name collisions rather than overwriting a resource or changing\nits owner. Invalid legacy graph views fail without disclosing referenced\ninventory. See ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#workspace-isolation-upgrade-and-recovery",
        children: "upgrade and recovery"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The portal honors a saved choice only when it is authorized. Otherwise global\nadministrators start in System; tenant users prefer an authorized non-System\nworkspace. Account and workspace resolution must complete before inventory\nqueries run. No memberships means no tenant write target. Explicit System\nmembership permits reads, not System administration; tenant membership roles\nare retained. Forms, reference pickers, key/credential requests and mutations\nuse the selected workspace, and stale forms cannot retarget themselves after a\nworkspace or account change."
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "direct-catalog-writes",
      children: "Direct Catalog Writes"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["This applies to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "PUT"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "DELETE"
      }), " on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/_internal/admin/providers/{name}"
      }), " and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "/_internal/admin/aliases/{name}"
      }), ", and to\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "PUT /_internal/admin/providers/{name}/credential"
      }), ". It also applies to\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "#github-copilot-device-authorization",
        children: "starting a Copilot device flow"
      }), " with\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "provider_name"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Each supplied ", (0,jsx_runtime.jsx)(_components.code, {
          children: "workspace_id"
        }), " query parameter, ", (0,jsx_runtime.jsx)(_components.code, {
          children: "X-Workspace-ID"
        }), " header, and\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "workspace_id"
        }), " body field (provider/alias PUT and named Copilot starts) must\nmatch the resource's workspace. Even global administrators cannot override a\nconflicting selector. Omit unused selectors; empty or invalid IDs return\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "400"
        }), " (", (0,jsx_runtime.jsx)(_components.code, {
          children: "invalid workspace id"
        }), "). These operations cannot move a resource."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["For valid requests, missing names, names outside the caller's visibility\n(including hidden System HCL resources), and valid selector mismatches all\nreturn ", (0,jsx_runtime.jsx)(_components.code, {
          children: "404"
        }), ". The plain-text body is ", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider not found"
        }), " or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "alias not found"
        }), ",\nfollowed by a newline. These denials save nothing, request no reload, and\ninclude no save or activation headers."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["A required membership lookup failure returns ", (0,jsx_runtime.jsx)(_components.code, {
          children: "500"
        }), " (", (0,jsx_runtime.jsx)(_components.code, {
          children: "could not resolve workspace visibility"
        }), ") without database details. For visible targets in the\nselected scope, write-role requirements are unchanged. Visible HCL resources\nremain read-only (", (0,jsx_runtime.jsx)(_components.code, {
          children: "400"
        }), " for PUT and named Copilot starts, ", (0,jsx_runtime.jsx)(_components.code, {
          children: "403"
        }), " for DELETE)."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Scoped GET selection, graph-reference validation and global name uniqueness on\ncreation are unchanged."
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "workspace-and-team-membership-changes",
      children: "Workspace And Team Membership Changes"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["With multi-tenancy enabled, Add member is insert-only on both\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /_internal/admin/workspaces/{workspace_id}/members"
      }), " and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /_internal/admin/workspaces/{workspace_id}/teams/{team_id}/members"
      }), ". Supply ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_id"
      }), "\nor ", (0,jsx_runtime.jsx)(_components.code, {
        children: "email"
      }), ", plus optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "role"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "admin"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "member"
      }), ", default ", (0,jsx_runtime.jsx)(_components.code, {
        children: "member"
      }), "). A new\nmembership returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "201"
      }), "; an existing membership returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "409"
      }), " and keeps its role,\neven if a different role was supplied. In the web UI, use the existing member's\nrole control; API clients should use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "PUT"
      }), " on the same path with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/{user_id}"
      }), "\nappended and an explicit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "role"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Role changes and removals check the current administrator set in a serialized\ndatabase transaction. Once a workspace or team has an administrator, these\noperations cannot remove or demote its last one, including competing requests.\nAssign another administrator before retrying a rejected handoff (", (0,jsx_runtime.jsx)(_components.code, {
        children: "400"
      }), "). A\nworkspace administrator cannot demote their own workspace role; another\nadministrator must make that edit. Team administrators may demote themselves\nwhen another team administrator remains. These guards also apply when deleting\na user account would cascade away their administrator memberships. New teams\ncan still start without a team administrator and receive their first one later."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Team additions require current membership in the containing workspace. A\nsuccessful role edit or removal returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "200"
      }), "; a missing membership returns\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "404"
      }), ". Unexpected storage failures return a controlled ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " with no committed\nmembership change. The UI displays the server's conflict/handoff guidance."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Workspace offboarding (", (0,jsx_runtime.jsx)(_components.code, {
        children: "DELETE /_internal/admin/workspaces/{workspace_id}/members/{user_id}"
      }), ")\nalso removes that user's team memberships and direct key-sharing grants in the\nworkspace, in the same transaction. If they are the sole admin of any affected\nteam, the request returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "400"
      }), " with handoff guidance and changes nothing. Assign\nanother team administrator, then retry. Other workspaces, key ownership,\nteam-wide key shares, quota records and historical spend are preserved. Re-adding\nan ordinary member does not restore prior team roles or direct sharing grants."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Current workspace membership is required for non-system key owners/team admins\nto read or manage keys and for team quota access, even with a known resource ID or\nan existing login token. After removal, key detail, ", (0,jsx_runtime.jsx)(_components.a, {
        href: "#direct-key-writes",
        children: "direct key writes"
      }), ",\nand team quota GET/PUT return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "404"
      }), ". System administrators retain access. Personal\nownership is preserved and becomes usable again if the owner is re-added to the\nworkspace. Already-admitted operations are not retroactively canceled."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.strong, {
        children: "API bearer credentials are separate:"
      }), " removing management access does not\nautomatically revoke, rotate or disable any key, including a personal key. To stop\nuse of previously distributed API tokens, explicitly rotate or revoke the key."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "account-deletion-and-personal-bearers",
      children: "Account Deletion And Personal Bearers"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "DELETE /_internal/admin/users/{user_id}"
      }), " is a global-administrator action.\nSelf-deletion and deleting the last enabled global administrator are rejected;\nworkspace/team last-administrator checks remain transactional. Rejected deletion\nrequests no activation and preserves the account and its keys."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["After confirmed deletion, the response sets ", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-User-Deleted: true"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-User-ID: <user_id>"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Cache-Control: no-store"
      }), ", then requests local\nactivation once. A ", (0,jsx_runtime.jsx)(_components.code, {
        children: "200"
      }), " means new requests using personally owned keys removed\nby the account cascade now receive ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), " on that instance. Team-owned and other\nusers' keys retain their identity and workspace; historical spend remains, with\nthe deleted user's ledger reference set to null. Already-admitted work can finish.\nWorkspace membership removal still does not revoke bearers."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["A ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500 saved but activation failed"
      }), " with the deletion headers means the account\nand personal keys are deleted in PostgreSQL, but the previous runtime remains\nactive and may still accept those bearers. Do not repeat account deletion to\nactivate it: resolve the logged failure and reload/restart the receiving instance.\nDB changes also reconcile independently on other replicas, without cluster acknowledgement."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "GET /_internal/admin/serving-status"
      }), " requires live system-administrator authorization.\nIt returns local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "applied_revisions"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "catalog"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "keys"
      }), "), ", (0,jsx_runtime.jsx)(_components.code, {
        children: "keys_confirmed_at"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "catalog_confirmed_at"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "keys_fresh"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "catalog_fresh"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "freshness_deadline_seconds"
      }), " (30) and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "cluster_acknowledged"
      }), " (false).\nMutation activation headers distinguish ", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Local-Active"
      }), " from\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Cluster-Acknowledged"
      }), " (always false), alongside\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Applied-Catalog-Revision"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Applied-Key-Revision"
      }), ".\nThey do not replace durable-save headers or acknowledge other replicas.\nExpired DB-serving freshness returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "503 serving_stale"
      }), " on new DB inference,\nlisting/billing and applicable peer controls; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/readyz"
      }), " becomes unavailable.\nIndependent key refresh may revoke keys even when catalog activation fails.\nSee ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#bounded-replica-convergence",
        children: "bounded replica convergence"
      }), ".\nAfter response loss, reconcile the account's presence through the authorized user\nlist; missing headers do not establish rollback. This is not a cluster-wide\nrevocation acknowledgement."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "atomic-key-issuance-and-bearer-recovery",
      children: "Atomic Key Issuance And Bearer Recovery"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "POST /_internal/admin/keys"
      }), " commits the new key, its intended user/team owner,\nand both sharing lists in one authorized transaction. The owner's share is included\nautomatically; repeated binding IDs are deduplicated. Current actor authority,\nowner eligibility and user/team workspace membership are rechecked under the\nmembership locks, including concurrent offboarding, demotion and deletion. A\nnon-administrator can issue only their own personal key or a key for a team they\ncurrently administer in that workspace; unowned keys require workspace or global\nadministrator authority."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Validation/authorization failures and key/grant persistence failures roll back\nthe entire issuance and request no activation. Lost authority returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), ",\ninvalid bindings ", (0,jsx_runtime.jsx)(_components.code, {
        children: "400"
      }), ", and storage failures a controlled ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " without database\ndetails. An existing key name returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "409"
      }), ": read that key rather than issuing\nunder a new name to recover a missing bearer."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["After confirmed commit, creation and rotation responses set\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Key-Saved: true"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Key-ID: <key_id>"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "Location: /_internal/admin/keys/<key_id>"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Cache-Control: no-store"
      }), ".\nActivation is requested exactly once after commit, before creating the response\nview. Creation returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "201"
      }), " with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "key"
      }), " and the one-time ", (0,jsx_runtime.jsx)(_components.code, {
        children: "token"
      }), "; rotation returns\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "200"
      }), " with a new one-time ", (0,jsx_runtime.jsx)(_components.code, {
        children: "token"
      }), ". Only the bearer hash and display prefix are\nstored, never a recoverable plaintext bearer."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Post-commit activation or creation-view failures return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " with those saved\nheaders and an explicit key ID and rotation recovery instruction. The complete\nissuance remains saved; an activation failure retains the previous runtime.\nRead the authorized key detail/list, resolve any activation problem, then use\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /_internal/admin/keys/<key_id>/rotate"
      }), " to obtain a new bearer. Do not repeat\ncreation. Rotation preserves immutable credential identity, workspace, ownership\nand grants, and invalidates the prior bearer after successful local activation.\nIf rotation itself commits but activation fails or its response is lost, rotate\nthe same key again after resolving the problem; plaintext tokens are not replayed."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "A lost connection/response (including an indeterminate database commit result)\ndoes not prove rollback, and absence of saved headers alone is not proof either.\nDiscover the key by its globally unique name through the authorized list before\nretrying. A duplicate-name rejection does not overwrite or rotate it. These are\nsingle-instance activation outcomes, not cluster-wide acknowledgements."
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "direct-key-writes",
      children: "Direct Key Writes"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "PUT"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "DELETE"
      }), " on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/_internal/admin/keys/{key_id}"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST"
      }), " on its\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "/rotate"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/revoke"
      }), " subpaths require current visibility and key manager\nauthority. Each supplied ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_id"
      }), " query parameter and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Workspace-ID"
      }), "\nheader must match the key's workspace, even for global administrators. Omitting\nselectors preserves authorized access by ID. An empty or malformed selector\nreturns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "400"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "invalid workspace id"
      }), ") before the key is looked up."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Missing keys, keys no longer visible to the caller after offboarding, other\nhidden keys, and valid selector mismatches all return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "404"
      }), " with the plain-text\nbody ", (0,jsx_runtime.jsx)(_components.code, {
        children: "key not found"
      }), " followed by a newline. A visible read-only share still\nreturns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "key manager required"
      }), "). Required membership lookups that fail\nreturn ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "could not resolve workspace visibility"
      }), "); key or sharing lookup\nfailures return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "could not load key"
      }), "). These denials save nothing, request\nno activation, and include no save or activation headers. Scoped GET continues\nto give the query selector precedence over the header."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "key-policy-and-sharing-updates",
      children: "Key Policy And Sharing Updates"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "PUT /_internal/admin/keys/{key_id}"
      }), " accepts ", (0,jsx_runtime.jsx)(_components.code, {
        children: "description"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tenant"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "expires_at"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_ids"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "team_ids"
      }), ". Policy fields and an\noptional sharing replacement commit in one database transaction. The update keeps\nthe key's name, workspace, ownership, bearer credential, and enabled state."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Omitted or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "null"
        }), " fields preserve their current values. An empty ", (0,jsx_runtime.jsx)(_components.code, {
          children: "description"
        }), "\nor ", (0,jsx_runtime.jsx)(_components.code, {
          children: "tenant"
        }), " clears it; ", (0,jsx_runtime.jsx)(_components.code, {
          children: "allowed_models: []"
        }), " removes the explicit list\nrestriction, not the key's workspace restriction."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "expires_at"
        }), " accepts RFC3339; an empty string clears expiry. A past timestamp\nis valid and makes the key stop authenticating after successful activation."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Supplying either non-null binding list replaces ", (0,jsx_runtime.jsx)(_components.strong, {
          children: "both"
        }), " user and team shares.\nThe other omitted list is treated as empty. Supplying ", (0,jsx_runtime.jsx)(_components.code, {
          children: "user_ids: []"
        }), " alone,\nfor example, clears both sets. Omitting both lists preserves both sets."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Malformed UUIDs, nonmember users, and teams from another workspace return\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "400"
      }), "; missing keys/teams return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "404"
      }), ". Current manager authority and grant\neligibility are rechecked inside the transaction, serialized with workspace\noffboarding and membership role changes. If manager authority is lost after the\ninitial access check, the transactional recheck still returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), ".\nRejected validation and binding-storage failures leave all fields and both sharing\nsets unchanged and do not request runtime activation. Storage failures return\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "could not update key"
      }), ") without database details. A later reload cannot\nactivate a rejected edit."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["After a valid commit the server requests runtime activation once, before building\nthe success response. Successful activation applies authentication expiry, tenant,\nand allowed-model policy to subsequent requests. If activation fails, the response\nis ", (0,jsx_runtime.jsx)(_components.code, {
        children: "500"
      }), " with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "saved but activation failed: ..."
      }), ": ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "the complete edit is saved"
      }), ",\nthe previous runtime remains active, and a later successful reload applies it.\nResolve the reported activation problem and reload; this differs from a mutation\nfailure that saved nothing."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "usage-and-scope",
      children: "Usage And Scope"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "GET /v1/billing/usage"
      }), " returns aggregated in-process usage summaries over the\nproxy's rolling 24-hour accounting window."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Hosted requests require a workspace-scoped inference bearer."
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["A DB bearer sees only its own immutable key/workspace usage, even if its labels\noverlap another key or HCL client, or its creator is a global administrator.\nNeither a ", (0,jsx_runtime.jsx)(_components.code, {
          children: "tenant"
        }), " label nor an allowed-model list grants workspace-wide reporting."]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Label edits do not hide that key's earlier usage. Token rotation preserves its\nidentity; deleting and recreating a same-named key gives it a new identity and\nno access to the deleted key's usage."
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Trusted HCL clients with a ", (0,jsx_runtime.jsx)(_components.code, {
          children: "tenant"
        }), " see same-tenant ", (0,jsx_runtime.jsx)(_components.strong, {
          children: "static"
        }), " clients in System\nonly, never DB keys with matching labels. Tenantless HCL clients see their own\ntenantless static usage. These file-only reporting contracts remain supported."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["File-only ", (0,jsx_runtime.jsx)(_components.code, {
          children: "auth.mode = \"none\""
        }), " retains unfiltered anonymous reporting. Hosted\ninference never uses this anonymous view."]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Workspace-wide or global reads require separate explicit management/operator\nauthority. This endpoint does not accept scope-broadening query/header selectors\nor inherit browser roles from a key's creator."
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Rows are grouped first by internal credential identity and explicit workspace,\nthen by the recorded tenant/client labels, requested public model, operation and\nHTTP status. Earlier and later labels can therefore appear as separate own-key\nrows. Credential/workspace identifiers are not emitted in these JSON summaries.\nOrdinary and deferred completions use the identity, scope and labels captured at\nadmission, including after a reload, edit, rotation or deletion."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Passthrough calls aggregate usage and metrics under the non-callable model\nidentity ", (0,jsx_runtime.jsx)(_components.code, {
        children: "<provider>/@passthrough/<rule>"
      }), " (provider-local target\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "@passthrough/<rule>"
      }), "), not by each caller-supplied ID. These rows report usage\nbut omit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "estimated_cost_usd"
      }), ": rules have no configured prices. Literal prices\nand credential/workspace reporting scopes do not change."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "estimated_cost_usd"
      }), " uses current configured model prices.\nFor an alias, it sums the recorded token usage of the targets actually used for\nthat alias's identity-scoped row. Other credentials/workspaces, another alias or\na direct model sharing those targets are excluded. Changing alias membership does\nnot redistribute past traffic; changing model prices reprices the retained estimate.\nHosted estimates use only current prices from the caller's workspace. If a\nretained provider/model name is recreated in another workspace, its foreign\nprice is not used; the retained own-key row remains visible without an estimate."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The estimate is ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "omitted"
      }), ", rather than returned as a partial sum, when any\ncontributing target lacks pricing or when target attribution does not account for\nall the row's recorded requests and tokens. There is no even-split or request-count\nfallback. Unused targets with missing prices do not affect the estimate. Direct and\nalias estimates also require positive rates for the token categories consumed:\ninput, output, and generic cached tokens. Explicit cache-read and cache-write usage\ncan use the configured input-rate fallback. An omitted rate and an explicit zero\nrate cannot be distinguished, so either makes the estimate unavailable when needed.\nA numeric zero means the fully priced recorded usage costs zero; it does not prove\nthat the upstream reported every billable token."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Usage and cost share one rolling snapshot, using one-minute buckets retained while\ntheir start is at or after ", (0,jsx_runtime.jsx)(_components.code, {
        children: "now - 24h"
      }), ". Consequently, events can expire up to one\nminute before their exact 24-hour age. Data is process-local and resets on restart.\nThis is a rolling usage/cost ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "estimate, not an invoice"
      }), ", cross-instance total,\nexternal billing service or quota system.\nThe dashboard's lifetime provider/upstream counters and the durable quota spend\nledger have separate lifetimes and pricing semantics."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Database user/team quotas are completion-based admission thresholds, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "not hard\nspending limits or funded workspace caps"
      }), ". Ownerless DB keys and static/HCL or\nanonymous traffic have no DB owner quota. Workspace reporting does not assign\na payer or grant workspace budget authority. See the\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#spending-contract-owner-quotas-not-funded-workspace-caps",
        children: "spending contract"
      }), "\nfor quota permissions, cache/stream overshoot, missing data and deletion gaps."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "error-behavior",
      children: "Error Behavior"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Direct requests never fail over to another provider"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Alias requests retry the next target on transport errors, timeouts, and status\ncodes listed in ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry_status_codes"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["The default ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry_status_codes"
        }), " list is ", (0,jsx_runtime.jsx)(_components.code, {
          children: "500"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "502"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "503"
        }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "504"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Configured retry statuses may include ", (0,jsx_runtime.jsx)(_components.code, {
          children: "4xx"
        }), " responses such as ", (0,jsx_runtime.jsx)(_components.code, {
          children: "429"
        }), "; retryable\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "4xx"
        }), " statuses do not mark providers unhealthy"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Other upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "4xx"
        }), " responses are returned verbatim, except for the narrowly\ngated same-target reasoning recovery below"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Chat completions/Responses may make one conditional same-identity\nencrypted-reasoning recovery attempt: direct requests make at most two\ninference attempts; aliases share one extra attempt across all targets, not\none per target. An ordinary immediate 400 to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "stream: true"
        }), " may qualify;\nestablished streams and peer control failures never do. See\n", (0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/providers-and-routing#scoped-encrypted-reasoning",
          children: "all gates and stripping limits"
        })]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Unsupported operations return client-visible proxy errors"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Peer deferred control failures are not provider outcomes and do not authorize\nalias failover; definite terminal inference outcomes retain the rules above\nbefore downstream commitment (see ", (0,jsx_runtime.jsx)(_components.a, {
          href: "#peer-control-errors",
          children: "peer errors"
        }), ")"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["For database-owned user/team keys, unavailable required quota policy or spend\ndata returns JSON ", (0,jsx_runtime.jsx)(_components.code, {
          children: "503"
        }), "\n(", (0,jsx_runtime.jsx)(_components.code, {
          children: "{\"error\":{\"type\":\"quota_unavailable\",\"message\":\"quota data temporarily unavailable\"}}"
        }), ")\nbefore upstream dispatch, including direct/alias and streaming requests. This\nis a temporary failure that can be retried after storage recovers. Confirmed\nbudget exhaustion instead returns ", (0,jsx_runtime.jsx)(_components.code, {
          children: "403"
        }), " ", (0,jsx_runtime.jsx)(_components.code, {
          children: "budget_exceeded"
        }), "; confirmed TPM\nexhaustion returns ", (0,jsx_runtime.jsx)(_components.code, {
          children: "429"
        }), " ", (0,jsx_runtime.jsx)(_components.code, {
          children: "tpm_exceeded"
        }), " with ", (0,jsx_runtime.jsx)(_components.code, {
          children: "Retry-After"
        }), "."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Alias targets carrying valid upstream retry advice (", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry-after-ms"
        }), ", else\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "Retry-After"
        }), ", else exhausted Meta quota headers) cool down across requests:\nlater alias requests skip cooling\ntargets until expiry. When every pool target is actively cooling, the proxy\nreturns a generated JSON ", (0,jsx_runtime.jsx)(_components.code, {
          children: "429"
        }), "\n(", (0,jsx_runtime.jsx)(_components.code, {
          children: "{\"error\":{\"type\":\"upstream_rate_limited\",\"message\":\"all alias targets cooling, retry after <N>ms\"}}"
        }), ")\nwith ", (0,jsx_runtime.jsx)(_components.code, {
          children: "Retry-After"
        }), " (ceiling seconds, min 1) and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry-after-ms"
        }), " (ceiling\nmilliseconds, min 1) from the same earliest remaining delay, without upstream\ncalls. Direct requests never consult or populate this state."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This behavior is deliberate: direct model requests are explicit, while alias requests are the only place where the proxy is allowed to choose another target."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "administrative-provider-and-alias-saves",
      children: "Administrative Provider And Alias Saves"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "POST /_internal/admin/providers"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "PUT /_internal/admin/providers/{name}"
      }), "\nsave provider metadata and supplied models atomically. A rejected model write\nleaves no new provider on POST and preserves the exact previous provider and models\non PUT. Omitting ", (0,jsx_runtime.jsx)(_components.code, {
        children: "models"
      }), " (or sending null) preserves models; a supplied array is a\nreplacement subject to normal provider validation. Omitted fields and write-only\ncredentials are preserved on update. Policy values have the explicit null-removal\nexception below. Credential-only PUT uses the same concurrency\nprotection and preserves models/policies; it rejects policy/model edits rather\nthan silently accepting them. Alias POST/PUT likewise save metadata and targets\nin one transaction, preserving omitted update fields."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider PUT supports explicit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "true"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "false"
      }), " for ", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled"
      }), " and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_user_agent"
      }), " (also ", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_authorization"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allow_insecure_base_url"
      }), "\nin System), ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[]"
      }), " to clear local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_headers"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"\""
      }), " to clear\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "display_name"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_header_timeout"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_agent"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "extends"
      }), ",\nsubject to normal validation. Clearing an override restores applicable defaults;\nit does not disable root-level forwarding. Omit untouched ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), "\nand ", (0,jsx_runtime.jsx)(_components.code, {
        children: "credential_ref"
      }), " fields to retain them. Supplied reference objects replace\ntheir path and key/name together; an empty path selects the default secrets path."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "tenant-provider-credential-and-destination-policy",
      children: "Tenant Provider Credential And Destination Policy"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For non-System database providers, create, update and credential-only PUT\nvalidate the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "destination workspace's"
      }), " effective configuration before\ncredential decryption/file resolution or persistence. Global administrators\nauthoring tenant rows do not bypass this policy. The operator-controlled\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "multi_tenancy.allowed_upstream_origins"
      }), " config attribute approves exact public\nHTTPS upstream/probe origins; omission or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[]"
      }), " denies all. It is not a writable\nprovider/API field and does not grant access to a System or foreign resource or\ncredential. See ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/configuration#tenant-managed-upstreams",
        children: "configuration"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Tenant ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key"
        }), " values remain write-only and encrypted in the database.\nOwn Copilot database credentials still use the actor/workspace-bound device\nauthorization flow, never a raw-token or foreign-credential input."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Nonempty ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key_ref"
        }), " and sidecar ", (0,jsx_runtime.jsx)(_components.code, {
          children: "credential_ref"
        }), " references are\nSystem-only, including the flat reference fields on credential PUT.\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "forward_authorization: true"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "allow_insecure_base_url: true"
        }), " are likewise\nforbidden in tenant workspaces, even with an approved origin."]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Both effective upstream and health destinations must be approved; provider\ndefaults and inherited endpoints/healthchecks cannot bypass the check.\nDisabled rows and old stored rows are checked too."
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["A forbidden edit returns ", (0,jsx_runtime.jsx)(_components.code, {
          children: "400"
        }), ", has no ", (0,jsx_runtime.jsx)(_components.code, {
          children: "X-Aiproxy-Catalog-Saved"
        }), " header,\nrequests no activation and leaves provider/models/credentials/revision\nunchanged. Correcting legacy intent may explicitly clear both members of a\nreference object and set reserved flags to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "false"
        }), ". Omission does not repair\na forbidden old setting."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["At connection time, tenant transports reject any non-public DNS answer and\npin dialing to the validated IPs, ignore environment proxies and keep TLS\nverification and same-origin redirect limits. A syntactically approved\nhostname can still fail at runtime because DNS or certificate verification\nfails; successful save/preflight is not an upstream compatibility check.\nTrusted System/HCL behavior is unchanged. Run the candidate binary's\n", (0,jsx_runtime.jsxs)(_components.a, {
        href: "/docs/operations#tenant-provider-policy-upgrade",
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "validate --check-db"
        }), " preflight"]
      }), "\nbefore upgrading."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "allow_insecure_base_url"
      }), " is a provider-only opt-in that accepts a non-loopback\nplain-HTTP ", (0,jsx_runtime.jsx)(_components.code, {
        children: "base_url"
      }), " (see\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/configuration#allowing-plain-http-upstreams",
        children: "plain-HTTP upstreams"
      }), ").\nThe opt-in is ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "System-only"
      }), " for database providers. Omitting the flag preserves\nits current value on concrete providers. Enabling\nit does not relax any other URL validation. Derived provider POST/PUT rejects\nexplicit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "true"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "false"
      }), "; setting a non-empty ", (0,jsx_runtime.jsx)(_components.code, {
        children: "extends"
      }), " without declaring the\nflag clears any previously stored local opt-in so the base's value is inherited."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "forward_authorization"
      }), " is a System-only database provider opt-in for supported\nbearer-header\nupstreams, independent of inherited defaults. It cannot coexist with stored\ncredentials. To switch an existing keyed provider, explicitly send\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"api_key\": \"\""
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"api_key_ref\": {\"path\": \"\", \"key\": \"\"}"
      }), " along with\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"forward_authorization\": true"
      }), ". Omitting the flag preserves its current value.\nSee ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/configuration#forwarding-client-authorization",
        children: "client authorization forwarding"
      }), "\nfor supported types and credential-exposure warnings."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_transport"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_receiver"
      }), " are static-config-only.\nAdministrative provider POST/PUT and credential PUT reject either field before\ncommit, including nested model declarations, null/empty objects and case variants,\nfor every admin role. Managed derivatives cannot extend a static base carrying\nany provider/model caller setting or receiver opt-in.\nThere are no writable managed/web transport fields; see\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/configuration#peer-deferred-transport-v1",
        children: "peer configuration"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Within an existing ", (0,jsx_runtime.jsx)(_components.code, {
        children: "healthcheck"
      }), ", omitted scalar fields are preserved and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "send_authorization: false"
      }), " disables probe authorization. Empty ", (0,jsx_runtime.jsx)(_components.code, {
        children: "method"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "expected_body"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "interval"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "timeout"
      }), " reset to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "GET"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "*"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "30s"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "5s"
      }), ";\nzero ", (0,jsx_runtime.jsx)(_components.code, {
        children: "expected_status"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "failure_threshold"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "success_threshold"
      }), " reset to\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "200"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "2"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "1"
      }), ". A nonempty ", (0,jsx_runtime.jsx)(_components.code, {
        children: "path"
      }), " replaces the path. ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Removing the entire\nhealthcheck block is unsupported"
      }), ": omission, null, an empty object or an empty\npath does not delete it."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Catalog POST/PUT/DELETE outcomes:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Status"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Meaning and next step"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "400"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Validation rejected the edit; correct the input."
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "400"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "catalog dependency conflict; retarget or remove dependent aliases and providers first"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "The saved dependency graph would become invalid; retarget/delete referencers first. No rows changed and no activation was requested."
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "409"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "The edited aggregate changed concurrently; read current state and retry."
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "500"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "could not save catalog edit"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Storage failed before a successful commit; activation was not requested."
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "500"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "saved but activation failed"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "The complete edit is saved; the previous runtime remains active. Consult server logs, correct the activation problem, then reload."
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "500"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "saved but response view unavailable; read current state before retrying"
            })]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "The edit is saved and activation was requested successfully, but its response view could not be read. Read current state rather than blindly repeating POST."
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "201"
            }), " (POST), ", (0,jsx_runtime.jsx)(_components.code, {
              children: "200"
            }), " (PUT/DELETE)"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Saved, activation requested successfully, and any response view is available."
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["After each valid commit, activation is requested once before response-view reads.\nOperational failure responses omit storage and credential details. Persistence and\nruntime activation are separate: later reloads can activate a complete saved edit\nafter an activation failure. Aggregate revision checks reject stale edits; the\nworkspace database lock also serializes graph-affecting provider/model/alias and\nCopilot aggregate writes. Dependency validation uses current saved definitions and\neffective static System bases, never stale managed runtime inventory.\nSuccessful provider POST/PUT/DELETE and credential PUT responses carry\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Catalog-Saved: true"
      }), " once the database commit is durable, including\nwhen activation itself fails."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "managed-catalog-dependency-edits",
      children: "Managed Catalog Dependency Edits"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Provider deletion, disabling, model removal, concrete/derived transitions and\ntype/capability/protocol changes cannot invalidate an alias or derivative.\nEvery alias target must remain defined and enabled in the alias's destination\nworkspace, with at least one capability shared by all targets. Derivatives must\nretain an enabled, concrete, same-type base; disabled derivatives still require\nthat base and inherited models named by local policy overlays. Aliases targeting\ninherited models are checked when their base changes."
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Retarget or delete referencers before removing the referenced provider/model.\nThere is no force-delete/cascade or automatic retargeting. Dependency errors are\ngeneric and reveal no dependent names or counts. Rejection preserves all aggregate\nrows, revisions and credentials, requests no activation, has no saved header, and\ndoes not consume a ready Copilot device flow."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Only affected dependencies are validated during a save. Unrelated corrupted rows\nremain repairable; they can still cause full runtime activation to fail ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "after a\nvalid commit"
      }), ", which retains the saved-but-activation-failed contract above.\nStatic file edits and direct SQL are not covered by managed-write serialization.\nThis does not acknowledge activation on any other instance. See\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#managed-catalog-dependencies-and-cleanup",
        children: "cleanup and upgrade guidance"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "encrypted-reasoning-policy-intent",
      children: "Encrypted Reasoning Policy Intent"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider POST/PUT, nested ", (0,jsx_runtime.jsx)(_components.code, {
        children: "models"
      }), " entries, and alias POST/PUT accept an optional\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "encrypted_reasoning"
      }), " object. Members are ", (0,jsx_runtime.jsx)(_components.code, {
        children: "passthrough"
      }), " (boolean),\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "on_caller_mismatch"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"fail\""
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"strip_and_retry\""
      }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"\""
      }), " for fail) and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "match_messages"
      }), " (string array). These are ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "whole-block declarations"
      }), ", not\neffective values or member patches; omitted members select built-ins. Pattern\nnormalization/bounds and runtime gates are the same as\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/configuration#scoped-encrypted-reasoning",
        children: "static configuration"
      }), ".\nPolicy members themselves cannot be null."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Policy input"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Create"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Provider / alias PUT"
          }), (0,jsx_runtime.jsxs)(_components.th, {
            children: ["Entry in a supplied replacement ", (0,jsx_runtime.jsx)(_components.code, {
              children: "models"
            }), " array"]
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Omitted"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No local block"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Preserve the exact saved local block"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No local policy; do not preserve the old row's policy"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "null"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No local block"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Remove the local block; resume lower-layer resolution"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "No local policy/overlay"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "{}"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Safe reset"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Replace with passthrough ", (0,jsx_runtime.jsx)(_components.code, {
              children: "true"
            }), ", mismatch ", (0,jsx_runtime.jsx)(_components.code, {
              children: "\"fail\""
            }), ", built-in patterns"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Local reset, not inheritance"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Nonempty object"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Local override"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Replace the entire block, not just supplied members"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Local override"
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.strong, {
        children: "Alias null removal is intentional:"
      }), " omission keeps an explicit alias policy\nand its sticky stripped-body behavior; explicit null removes it, switching to\nper-selected-target policies and original-body-per-target behavior. Existing\nnon-null alias overrides keep their semantics."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Policy null and collection null are different. On provider PUT, omitted ", (0,jsx_runtime.jsxs)(_components.strong, {
        children: ["or\nnull ", (0,jsx_runtime.jsx)(_components.code, {
          children: "models"
        }), " preserves the complete local collection"]
      }), ". An array replaces it\ntransactionally. A derivative's ", (0,jsx_runtime.jsx)(_components.code, {
        children: "models: []"
      }), " clears only local overlays, not\ninherited inventory. A concrete provider still needs at least one valid model.\nWithin a replacement array, omitted/null policy removes that row's local policy;\nretain every other intended local override when submitting the array."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Managed derivatives inherit declared base provider/model policies by value,\nwith local provider overrides and existing-name model overlays. Their nested\nmodel entries accept ", (0,jsx_runtime.jsxs)(_components.strong, {
        children: ["only ", (0,jsx_runtime.jsx)(_components.code, {
          children: "name"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "encrypted_reasoning"
        })]
      }), ":"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-json",
        children: "{\n  \"extends\": \"primary\",\n  \"encrypted_reasoning\": { \"passthrough\": false },\n  \"models\": [{ \"name\": \"recover\", \"encrypted_reasoning\": {} }]\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["This is a provider PUT fragment, not a complete create. The model reset wins\nover both provider policies. Remove the overlay with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "models: []"
      }), " or a replacement\nentry with omitted/null policy to restore the base model policy. Policy-free\nname entries are validated against the base inventory, then canonicalized away.\nNew names, duplicate overlays, model definition keys and all deferred keys are\nrejected, ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "even empty/null"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_name: \"\""
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "capabilities: []"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "pricing: null"
      }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "deferred_transport: null"
      }), " are not allowed). Static overlays\nmay combine reasoning and deferred transport; managed overlays cannot."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The base must be enabled, concrete, same-type and eligible for managed\ninheritance; credentials stay local, chains remain unsupported. Switching\nconcrete ↔ derived with any stored model policy/overlay requires an explicit\nreplacement ", (0,jsx_runtime.jsx)(_components.code, {
        children: "models"
      }), " ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "array"
      }), " in that update; omission or null is insufficient.\nChanging bases validates preserved overlays against the new inventory. No\nprovider or credential relaxation is implied."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Writes reject duplicate JSON keys (including case-variant duplicates),\nnoncanonical reasoning/overlay keys, unknown policy members, wrong\ntypes, invalid patterns and read-only metadata. Do not PUT a GET view unchanged:\ninherited model definitions and effective metadata are not local declarations.\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "PUT /_internal/admin/providers/{name}/credential"
      }), " preserves policy/model intent\nand rejects supplied policy or model edits before commit."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Provider/model GET views (including static entries under existing visibility\nrules) expose:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Field"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Meaning"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "encrypted_reasoning"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Nullable ", (0,jsx_runtime.jsx)(_components.strong, {
              children: "owner-local sparse declaration"
            }), ": null for no local block, ", (0,jsx_runtime.jsx)(_components.code, {
              children: "{}"
            }), " for reset, or only declared members, including explicit false."]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "encrypted_reasoning_effective"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Fully normalized policy; provider views resolve provider → root → built-ins, model views resolve model → provider → root → built-ins. A provider value does not override every model."
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "encrypted_reasoning_source"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Winning scope: ", (0,jsx_runtime.jsx)(_components.code, {
              children: "model"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "provider"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "root"
            }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
              children: "builtin"
            }), "; provider views never report ", (0,jsx_runtime.jsx)(_components.code, {
              children: "model"
            }), "."]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "encrypted_reasoning_inherited"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["True only if the winning declaration was copied from a base provider/model through ", (0,jsx_runtime.jsx)(_components.code, {
              children: "extends"
            }), "; root/built-in fallback is false."]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "models_inherited"
            }), " (provider only)"]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Model definitions come from ", (0,jsx_runtime.jsx)(_components.code, {
              children: "extends"
            }), ", independent of local policy overlays."]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Derivative GET ", (0,jsx_runtime.jsx)(_components.code, {
        children: "models"
      }), " is the ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "full inherited inventory"
      }), ", with local nullable\nintent and effective metadata per model; PUT sends only sparse local overlays.\nFor an inherited model policy, local policy is null, source is ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), ", and\ninherited is true. A local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "{}"
      }), " overlay changes inherited to false without\nchanging ", (0,jsx_runtime.jsx)(_components.code, {
        children: "models_inherited"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Alias views expose local ", (0,jsx_runtime.jsx)(_components.code, {
        children: "encrypted_reasoning"
      }), " and normalized\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "encrypted_reasoning_effective"
      }), ". Without a local alias block, effective is\n", (0,jsx_runtime.jsx)(_components.strong, {
        children: "null"
      }), ": there is no single pool-wide effective policy."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["All effective/source/inherited fields are read-only. Reads describe ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "saved\nintent"
      }), " resolved against the relevant base/root snapshot, not proof that a\ncandidate activated. Existing RBAC, static-edit restrictions, durable-save\noutcomes/headers and receiving-instance-only activation still apply. Root has\nno new admin edit endpoint. Provider/model/alias web policy controls use these\nsame local-intent semantics: unchanged policies are omitted, inherit removes a\nlocal block, safe reset writes ", (0,jsx_runtime.jsx)(_components.code, {
        children: "{}"
      }), ", and custom writes only declared members.\nInherited model definitions and static rows remain read-only; the forms do not\nturn effective context into local declarations."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "provider-tls-clienthello-settings",
      children: "Provider TLS ClientHello Settings"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider POST/PUT accepts ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello"
      }), " for ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "built-ins only"
      }), ":\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "native"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chrome"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "chrome-133"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "safari"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "safari-16.0"
      }), ". Browser aliases resolve\nto the frozen bundled Chrome 133/Safari 16.0 presets in uTLS v1.8.2, not live/latest\nbrowser profiles. This adds no inference capability or new authorization surface."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Input on a concrete provider"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Behavior"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Field omitted on create"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Native Go TLS"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Field omitted on update"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Preserve the saved selection"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "\"tls_client_hello\": \"native\""
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Explicitly reset to native"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: "Empty, null, wrong-type, wrong-case or unknown selection"
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "400"
            }), "; no commit or activation"]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "custom"
            }), ", any spec-file or inline-spec declaration"]
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: [(0,jsx_runtime.jsx)(_components.code, {
              children: "400"
            }), "; static-config-only, no commit or profile-file access"]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Derived providers inherit the effective profile with their base transport. Every\nlocal TLS declaration, including ", (0,jsx_runtime.jsx)(_components.code, {
        children: "native"
      }), ", is rejected. Changing a concrete\nprovider to a derived one clears its previous local selection ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "only if the incoming\nTLS field is omitted"
      }), ". A managed derivative cannot extend a static custom-profile\nbase. Invalid stored selectors or inheritance also reject catalog startup/reload.\nExisting databases require ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy migrate up"
      }), "; the additive selector column's\nempty default means omitted, preserving existing native/inheritance behavior."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello_spec_file"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello_spec"
      }), " and other custom TLS-prefixed\nfields are forbidden, even empty or null, for both workspace and system\nadministrators. ", (0,jsx_runtime.jsx)(_components.code, {
        children: "PUT /_internal/admin/providers/{name}/credential"
      }), " rejects ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "all"
      }), "\nTLS declarations, including built-in selection, without saving a replacement\ncredential. Use provider PUT to change a concrete provider's built-in profile.\nStatic custom configuration requires a file reference; ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello_spec"
      }), "\nis not a supported inline recipe or upload option on either surface."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Provider views expose these metadata fields, never profile paths or recipe contents:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-json",
        children: "{\n  \"tls_client_hello\": \"chrome\",\n  \"tls_client_hello_effective\": \"chrome-133\",\n  \"tls_client_hello_inherited\": false,\n  \"tls_client_hello_static_only\": false\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello"
      }), " describes the selected or inherited base selector (omitted native\nis presented as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "native"
      }), "); ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello_effective"
      }), " is canonical. The booleans\nidentify inheritance and static-only custom profiles. Effective/inherited/static-only\nfields are view metadata, not writable configuration. Each provider-type entry from\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "GET /_internal/admin/provider-types"
      }), " supplies ", (0,jsx_runtime.jsx)(_components.code, {
        children: "tls_client_hello_selectors"
      }), " with the\napproved built-in list. The UI uses that list, shows inheritance and displays static\ncustom profiles read-only. Do not send a derived view's inherited selector back as\na local update. See ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/configuration#upstream-tls-clienthello",
        children: "configuration"
      }), " and\n", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/operations#reload-behavior",
        children: "reload behavior"
      }), "; the saved-versus-activated outcomes\nabove and receiving-instance-only activation remain unchanged."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "github-copilot-device-authorization",
      children: "GitHub Copilot Device Authorization"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "github-copilot"
      }), " providers can be authorized from the admin API without handling\nOAuth tokens in the browser. The server performs the GitHub device challenge over\na fixed issuer (", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://github.com/login/device/code"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "https://github.com/login/oauth/access_token"
      }), "), fixed ", (0,jsx_runtime.jsx)(_components.code, {
        children: "read:user"
      }), " scope, and the\nfixed device-verification page, then stores the result as an encrypted database\ncredential. Saving the provider consumes the ready authorization exactly once and\nactivates it through the normal catalog reload path. The CLI sidecar login remains\navailable for trusted System/HCL workflows, not tenant-managed providers."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "POST /_internal/admin/copilot-device-flows"
        }), " starts a flow with\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "{client_id, workspace_id?, provider_name?}"
        }), ". With ", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider_name"
        }), ", the\n", (0,jsx_runtime.jsx)(_components.a, {
          href: "#direct-catalog-writes",
          children: "direct catalog write rules"
        }), " check visibility, every\nsupplied workspace selector, and write authority before loading the full\nprovider or checking its type and policy. Missing, hidden, and selector-mismatched\nnames return the same ", (0,jsx_runtime.jsx)(_components.code, {
          children: "404"
        }), " (", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider not found"
        }), "). Visible HCL providers cannot\nbe edited here. A visible tenant provider requires workspace admin authority;\nan explicit non-global System member cannot start a flow for a System DB\nprovider (", (0,jsx_runtime.jsx)(_components.code, {
          children: "404"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "workspace not found"
        }), "). A provider-bound tenant edit must\nalso pass effective provider policy. These checks run before saving a flow,\nrecording its start for throttling, or contacting the issuer. Standalone\nauthorization (without ", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider_name"
        }), ") keeps the normal provider write-workspace\nselection and uses the same fixed issuer. Provider destination approval is\nchecked before its credential can be saved. Successful starts return ", (0,jsx_runtime.jsx)(_components.code, {
          children: "201"
        }), "\nflow-status bodies."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "GET /_internal/admin/copilot-device-flows/{id}"
        }), " returns the sanitized flow\nstatus without any upstream calls."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "POST /_internal/admin/copilot-device-flows/{id}/poll"
        }), " performs at most one\nupstream token request when the stored schedule and lease permit; early or\ncompeting calls return the pending status and delay with no extra upstream call.\nProvider-bound tenant policy is checked before claiming/decrypting the flow."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "DELETE /_internal/admin/copilot-device-flows/{id}"
        }), " cancels a live flow and is\nidempotent for terminal flows."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Flow-status bodies carry ", (0,jsx_runtime.jsx)(_components.code, {
        children: "id"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "workspace_id"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "status"
      }), "\n(", (0,jsx_runtime.jsx)(_components.code, {
        children: "starting"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "pending"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "ready"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "consumed"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "denied"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "expired"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "failed"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "cancelled"
      }), "), pending ", (0,jsx_runtime.jsx)(_components.code, {
        children: "user_code"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "verification_uri"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "expires_at"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "poll_after_ms"
      }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "ready_expires_at"
      }), ", bound/consumed provider references, and a safe ", (0,jsx_runtime.jsx)(_components.code, {
        children: "error_code"
      }), ".\nDevice codes, access tokens, refresh tokens, and ciphertext are never returned,\nand responses use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Cache-Control: no-store"
      }), ". Every operation requires a current\napp-user JWT with workspace or system administrator authority, plus\ninitiating-user ownership; another administrator cannot adopt a flow. Unknown or\nforeign IDs return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "404"
      }), ", invalid logins ", (0,jsx_runtime.jsx)(_components.code, {
        children: "401"
      }), ", insufficient rights ", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), "."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "PUT"
      }), " accepts ", (0,jsx_runtime.jsx)(_components.code, {
        children: "copilot_device_flow_id"
      }), " (flow ID only, never a raw\ncredential) instead of a credential reference. Edit saves that consume a flow\nrequire ", (0,jsx_runtime.jsx)(_components.code, {
        children: "expected_updated_at"
      }), ", the opaque revision from the provider view\n(", (0,jsx_runtime.jsx)(_components.code, {
        children: "updated_at"
      }), "). A stale revision returns ", (0,jsx_runtime.jsx)(_components.code, {
        children: "409"
      }), " and preserves the ready flow for\nan explicit reread and retry. The ready flow's workspace and create/edit provider\nbinding are checked before decrypting its credential; an approved tenant\ndestination cannot consume a System or foreign-workspace flow, even if a global\nauthor initiated it. Provider views expose\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "copilot_credential_source"
      }), " (", (0,jsx_runtime.jsx)(_components.code, {
        children: "database"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "sidecar"
      }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "none"
      }), " alongside the\nexisting ", (0,jsx_runtime.jsx)(_components.code, {
        children: "has_credential"
      }), "), and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "GET /_internal/admin/provider-types"
      }), " advertises\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "supports_device_authorization"
      }), " for ", (0,jsx_runtime.jsx)(_components.code, {
        children: "github-copilot"
      }), ". The legacy credential\nendpoint switches sources in System the same way (a nonempty sidecar reference clears the\ndatabase ciphertext) but never consumes flows. ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "Hermetically verified; live\nGitHub compatibility unverified."
      })]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "provider-coverage-notes",
      children: "Provider Coverage Notes"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "openai"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "openai-compatible"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "zenmux"
        }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "openrouter"
        }), " are close to pass-through adapters (", (0,jsx_runtime.jsx)(_components.code, {
          children: "zenmux"
        }), " defaults to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "https://zenmux.ai/api/v1"
        }), "; ", (0,jsx_runtime.jsx)(_components.code, {
          children: "openrouter"
        }), " defaults to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "https://openrouter.ai/api/v1"
        }), " and adds ", (0,jsx_runtime.jsx)(_components.code, {
          children: "HTTP-Referer"
        }), "/", (0,jsx_runtime.jsx)(_components.code, {
          children: "X-Title"
        }), " attribution headers)"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "anthropic"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "gemini"
        }), " use request and response translation"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["translated ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/v1/responses"
        }), " support is intentionally conservative compared with the full upstream provider-native feature set"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-zen"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "opencode-go"
        }), " mix native and translated handling per\nliteral model or rule ", (0,jsx_runtime.jsx)(_components.code, {
          children: "protocol"
        }), ": ", (0,jsx_runtime.jsx)(_components.code, {
          children: "chat"
        }), "/", (0,jsx_runtime.jsx)(_components.code, {
          children: "responses"
        }), " are native pass-through for\none public operation each, ", (0,jsx_runtime.jsx)(_components.code, {
          children: "messages"
        }), " is native Anthropic passthrough for\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/messages"
        }), " (plus conservative translation for chat/responses),\nand ", (0,jsx_runtime.jsx)(_components.code, {
          children: "gemini"
        }), " uses the conservative translation subsets. Operations a protocol does not serve, and\nembeddings/images/audio on both OpenCode types, are rejected before upstream\nI/O. See ", (0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/providers-and-routing",
          children: "Providers and Routing"
        }), " for the protocol\ncontract."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "github-copilot"
        }), " is chat-only pass-through: ", (0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/chat/completions"
        }), "\nserves JSON and SSE, and every other operation (including ", (0,jsx_runtime.jsx)(_components.code, {
          children: "responses"
        }), ",\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "embeddings"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "images"
        }), ", and audio) is rejected before upstream I/O.\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /v1/models"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /v1/billing/usage"
        }), " stay proxy-owned with no\nupstream calls. ", (0,jsx_runtime.jsx)(_components.strong, {
          children: "Hermetically verified; live GitHub compatibility unverified."
        }), "\nThese capability matrices describe implemented behavior, not verified GitHub\naccess. See ", (0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/operations#mock-only-copilot-verification",
          children: "mock-only verification"
        }), "."]
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