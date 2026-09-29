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
  "value": "Authentication",
  "id": "authentication",
  "level": 2
}, {
  "value": "Invitation Onboarding",
  "id": "invitation-onboarding",
  "level": 3
}, {
  "value": "Workspace Model",
  "id": "workspace-model",
  "level": 3
}, {
  "value": "Workspace And Team Membership Changes",
  "id": "workspace-and-team-membership-changes",
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
      }), " auth is enabled, no inbound authentication is performed."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["In ", (0,jsx_runtime.jsx)(_components.code, {
        children: "bearer_static"
      }), " mode, individual clients may also be restricted with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h3, {
      id: "invitation-onboarding",
      children: "Invitation Onboarding"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["With multi-tenancy enabled, ", (0,jsx_runtime.jsx)(_components.code, {
        children: "POST /_internal/admin/invites/accept"
      }), " accepts a JSON\nbody with ", (0,jsx_runtime.jsx)(_components.code, {
        children: "token"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
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
      }), " with\nretry guidance and saves nothing; retry uses the current invitation scope."]
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
        }), ") that owns global config-backed\ninventory. It is hidden from membership listings and cannot be deleted."]
      }), "\n"]
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
      children: ["Current workspace membership is required for non-system key owners/team admins\nto read or manage keys and for team quota access, even with a known resource ID or\nan existing login token. After removal, key detail and team quota GET/PUT return\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "404"
      }), "; key update/rotate/revoke/delete return ", (0,jsx_runtime.jsx)(_components.code, {
        children: "403"
      }), ". System administrators retain\naccess. Personal ownership is preserved and becomes usable again if the owner is\nre-added to the workspace. Already-admitted operations are not retroactively\ncanceled."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.strong, {
        children: "API bearer credentials are separate:"
      }), " removing management access does not\nautomatically revoke, rotate or disable any key, including a personal key. To stop\nuse of previously distributed API tokens, explicitly rotate or revoke the key."]
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
        }), " removes the model restriction."]
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
      }), ". Current manager authority and grant\neligibility are rechecked inside the transaction, serialized with workspace\noffboarding and membership role changes. Lost manager authority returns ", (0,jsx_runtime.jsx)(_components.code, {
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
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["When the authenticated client has a ", (0,jsx_runtime.jsx)(_components.code, {
          children: "tenant"
        }), ", results are scoped to that tenant"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Otherwise, results are scoped to the caller's tenantless client identity; a\nsame-named client in another tenant is excluded"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "This endpoint is local accounting only; it is not an external billing,\ninvoicing, or quota system"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Rows are grouped by tenant, client, requested public model, operation, and HTTP\nstatus. The optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "estimated_cost_usd"
      }), " uses current configured model prices.\nFor an alias, it sums the recorded token usage of the targets actually used for\nthat alias's row. Traffic through another alias or a direct model sharing those\ntargets is excluded. Changing alias membership does not redistribute past traffic;\nchanging model prices reprices the retained estimate."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The estimate is ", (0,jsx_runtime.jsx)(_components.strong, {
        children: "omitted"
      }), ", rather than returned as a partial sum, when any\ncontributing target lacks pricing or when target attribution does not account for\nall the row's recorded requests and tokens. There is no even-split or request-count\nfallback. Unused targets with missing prices do not affect the estimate. Direct and\nalias estimates also require positive rates for the token categories consumed:\ninput, output, and generic cached tokens. Explicit cache-read and cache-write usage\ncan use the configured input-rate fallback. An omitted rate and an explicit zero\nrate cannot be distinguished, so either makes the estimate unavailable when needed.\nA numeric zero means the fully priced recorded usage costs zero; it does not prove\nthat the upstream reported every billable token."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Usage and cost share one rolling snapshot, using one-minute buckets retained while\ntheir start is at or after ", (0,jsx_runtime.jsx)(_components.code, {
        children: "now - 24h"
      }), ". Consequently, events can expire up to one\nminute before their exact 24-hour age. Data is process-local and resets on restart.\nThe dashboard's lifetime provider/upstream counters and the durable quota spend\nledger have separate lifetimes and pricing semantics."]
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
        }), " responses are returned verbatim"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Unsupported operations return client-visible proxy errors"
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
      }), " (or sending null) preserves models; a supplied array is a\nreplacement subject to normal provider validation. Omitted fields and write-only\ncredentials are preserved on update. Credential-only PUT uses the same concurrency\nprotection and preserves models. Alias POST/PUT likewise save metadata and targets\nin one transaction, preserving omitted update fields."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider PUT supports explicit ", (0,jsx_runtime.jsx)(_components.code, {
        children: "true"
      }), "/", (0,jsx_runtime.jsx)(_components.code, {
        children: "false"
      }), " for ", (0,jsx_runtime.jsx)(_components.code, {
        children: "enabled"
      }), " and\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "forward_user_agent"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
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
      children: "Catalog POST/PUT outcomes:"
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
            }), " (PUT)"]
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Saved, activation requested successfully, and the response view is available."
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["After each valid commit, activation is requested once before response-view reads.\nOperational failure responses omit storage and credential details. Persistence and\nruntime activation are separate: later reloads can activate a complete saved edit\nafter an activation failure. The concurrency check protects the edited aggregate,\nnot an atomic snapshot of every catalog dependency or another proxy's runtime.\nSuccessful provider POST/PUT and credential PUT responses carry\n", (0,jsx_runtime.jsx)(_components.code, {
        children: "X-Aiproxy-Catalog-Saved: true"
      }), " once the database commit is durable, including\nwhen activation itself fails."]
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
      }), " scope, and the\nfixed device-verification page, then stores the result as an encrypted database\ncredential. Saving the provider consumes the ready authorization exactly once and\nactivates it through the normal catalog reload path. The CLI sidecar login remains\navailable as an alternative."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "POST /_internal/admin/copilot-device-flows"
        }), " starts a flow with\n", (0,jsx_runtime.jsx)(_components.code, {
          children: "{client_id, workspace_id?, provider_name?}"
        }), ". The workspace is resolved with the\nnormal provider write-workspace policy; ", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider_name"
        }), " pins an edit target and must\nbelong to the same workspace. Responses are ", (0,jsx_runtime.jsx)(_components.code, {
          children: "201"
        }), " flow-status bodies."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "GET /_internal/admin/copilot-device-flows/{id}"
        }), " returns the sanitized flow\nstatus without any upstream calls."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "POST /_internal/admin/copilot-device-flows/{id}/poll"
        }), " performs at most one\nupstream token request when the stored schedule and lease permit; early or\ncompeting calls return the pending status and delay with no extra upstream call."]
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
      }), " and preserves the ready flow for\nan explicit reread and retry. Provider views expose\n", (0,jsx_runtime.jsx)(_components.code, {
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
      }), ". The legacy credential\nendpoint switches sources the same way (a nonempty sidecar reference clears the\ndatabase ciphertext) but never consumes flows. ", (0,jsx_runtime.jsx)(_components.strong, {
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
        }), " mix native and translated handling per\nmodel ", (0,jsx_runtime.jsx)(_components.code, {
          children: "protocol"
        }), ": ", (0,jsx_runtime.jsx)(_components.code, {
          children: "chat"
        }), "/", (0,jsx_runtime.jsx)(_components.code, {
          children: "responses"
        }), " protocols are native pass-through for\none public operation each, ", (0,jsx_runtime.jsx)(_components.code, {
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