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
  "value": "Provider Types",
  "id": "provider-types",
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
    h1: "h1",
    h2: "h2",
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
      children: ["Names are lowercase and must not contain spaces or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/"
      }), "."]
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
      children: "Alias requests retry the next target only when the selected target fails with:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "transport errors"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "timeouts"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "5xx"
        }), " responses"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "4xx"
        }), " responses whose status code is listed in ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry_status_codes"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["By default ", (0,jsx_runtime.jsx)(_components.code, {
        children: "retry_status_codes"
      }), " is ", (0,jsx_runtime.jsx)(_components.code, {
        children: "[\"500\", \"502\", \"503\", \"504\"]"
      }), ", so only ", (0,jsx_runtime.jsx)(_components.code, {
        children: "5xx"
      }), " responses trigger failover. Add codes like ", (0,jsx_runtime.jsx)(_components.code, {
        children: "\"429\""
      }), " to also retry on rate-limited responses."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Alias requests do not fail over on other upstream ", (0,jsx_runtime.jsx)(_components.code, {
        children: "4xx"
      }), " responses. Those are returned to the client as-is."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This avoids masking client-side request problems as routing problems."
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
        })]
      })]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai-compatible"
      }), ", the proxy stays close to pass-through behavior. For translated providers, the proxy maps between the public OpenAI-style contract and the provider-native request and response shape."]
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
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Set explicit capabilities when you want the public catalog to reflect a narrower, safer contract than the provider's default behavior."
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