"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[976],{

/***/ 3394
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_intro_md_0e3_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-intro-md-0e3.json
const site_docs_intro_md_0e3_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"intro","title":"aiproxy","description":"aiproxy is a Go service that puts multiple AI providers behind one OpenAI-compatible HTTP API.","source":"@site/docs/intro.md","sourceDirName":".","slug":"/intro","permalink":"/docs/intro","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":1,"frontMatter":{"sidebar_position":1},"sidebar":"docsSidebar","next":{"title":"Quickstart","permalink":"/docs/quickstart"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/intro.md


const frontMatter = {
	sidebar_position: 1
};
const contentTitle = 'aiproxy';

const assets = {

};



const toc = [{
  "value": "Why It Exists",
  "id": "why-it-exists",
  "level": 2
}, {
  "value": "What It Does",
  "id": "what-it-does",
  "level": 2
}, {
  "value": "Supported Provider Types",
  "id": "supported-provider-types",
  "level": 2
}, {
  "value": "Supported Public API",
  "id": "supported-public-api",
  "level": 2
}, {
  "value": "How Clients Address Models",
  "id": "how-clients-address-models",
  "level": 2
}, {
  "value": "Typical Flow",
  "id": "typical-flow",
  "level": 2
}, {
  "value": "Documentation Map",
  "id": "documentation-map",
  "level": 2
}, {
  "value": "Design Notes",
  "id": "design-notes",
  "level": 2
}];
function _createMdxContent(props) {
  const _components = {
    a: "a",
    code: "code",
    h1: "h1",
    h2: "h2",
    header: "header",
    li: "li",
    ol: "ol",
    p: "p",
    ul: "ul",
    ...(0,lib/* useMDXComponents */.R)(),
    ...props.components
  };
  return (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
    children: [(0,jsx_runtime.jsx)(_components.header, {
      children: (0,jsx_runtime.jsx)(_components.h1, {
        id: "aiproxy",
        children: "aiproxy"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " is a Go service that puts multiple AI providers behind one OpenAI-compatible HTTP API."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Clients send standard OpenAI-style requests to the proxy, and the proxy resolves the requested model to a configured provider-backed model or alias target. When needed, it translates the request and response for provider-native APIs such as Anthropic and Gemini."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "why-it-exists",
      children: "Why It Exists"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " is useful when you want one stable client integration while your upstream AI inventory is split across providers, self-hosted gateways, or model pools."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "It lets the proxy own:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "the public model catalog clients see"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "inbound authentication and model allow-lists"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "alias-based balancing and failover"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "provider credential handling"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "observability and health state"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "what-it-does",
      children: "What It Does"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Exposes a single OpenAI-compatible API surface to clients"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Routes requests to multiple upstream providers"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Supports direct model addressing and alias-based routing"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Retries alias targets on transport failures and upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "5xx"
        }), " responses"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Preserves streaming responses through OpenAI-compatible SSE framing"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Keeps auth, provider credentials, routing, and observability in one service"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "supported-provider-types",
      children: "Supported Provider Types"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "openai"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "openai-compatible"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "anthropic"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "gemini"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "supported-public-api",
      children: "Supported Public API"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /v1/models"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /v1/billing/usage"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /metrics"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/chat/completions"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/embeddings"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/responses"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/images/generations"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/audio/transcriptions"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/audio/speech"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Provider support varies by operation. See ", (0,jsx_runtime.jsx)(_components.a, {
        href: "/docs/api-reference",
        children: "API Reference"
      }), " for the exact matrix."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "how-clients-address-models",
      children: "How Clients Address Models"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The proxy exposes model names in two forms:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Direct provider-backed models: ", (0,jsx_runtime.jsx)(_components.code, {
          children: "<provider-name>/<model-name>"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Alias-backed virtual models: ", (0,jsx_runtime.jsx)(_components.code, {
          children: "alias/<alias-name>"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Examples:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "openai/gpt-4o-mini"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "gemini/gemini-2.5-pro"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "alias/chat_default"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "typical-flow",
      children: "Typical Flow"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ol, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["A client sends an OpenAI-compatible request to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "aiproxy"
        }), "."]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "The proxy authenticates the caller if auth is enabled."
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "The proxy resolves the requested model string to a direct model or an alias target."
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "The proxy forwards the request upstream, translating it when the provider is not OpenAI-compatible."
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "The proxy returns an OpenAI-compatible response to the client."
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "documentation-map",
      children: "Documentation Map"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/intro",
          children: "Introduction"
        }), " for the high-level model and terminology"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/quickstart",
          children: "Quickstart"
        }), " for local setup and a first request"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/configuration",
          children: "Configuration"
        }), " for HCL structure, auth, and secrets"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/config-examples",
          children: "Config Examples"
        }), " for complete deployment-oriented HCL examples"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/providers-and-routing",
          children: "Providers and Routing"
        }), " for provider types, capabilities, aliases, and failover"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/request-examples",
          children: "Request Examples"
        }), " for common OpenAI-compatible calls through the proxy"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/api-reference",
          children: "API Reference"
        }), " for endpoint coverage and behavior"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/operations",
          children: "Operations"
        }), " for build, run, reload, Docker, and tests"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/deployment",
          children: "Deployment"
        }), " for Docker, Compose, systemd, and rollout guidance"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "design-notes",
      children: "Design Notes"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This site focuses on practical usage. For deeper implementation rationale and design details, see the full design document in the repository:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.a, {
          href: "https://github.com/egose/aiproxy/blob/main/docs/design.md",
          children: "github.com/egose/aiproxy/blob/main/docs/design.md"
        })
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