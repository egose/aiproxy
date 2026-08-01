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
  "value": "Endpoints",
  "id": "endpoints",
  "level": 2
}, {
  "value": "Provider Support Matrix",
  "id": "provider-support-matrix",
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
  "value": "Usage And Scope",
  "id": "usage-and-scope",
  "level": 2
}, {
  "value": "Error Behavior",
  "id": "error-behavior",
  "level": 2
}, {
  "value": "Provider Coverage Notes",
  "id": "provider-coverage-notes",
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
      id: "endpoints",
      children: "Endpoints"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Endpoint"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Method"
          }), (0,jsx_runtime.jsx)(_components.th, {
            children: "Notes"
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "/v1/models"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Lists direct models and aliases"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "/v1/billing/usage"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Returns aggregated in-process usage summaries"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "/metrics"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Prometheus metrics"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "/v1/chat/completions"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "JSON and SSE streaming"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "/v1/embeddings"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Supported for ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai-compatible"
            }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
              children: "gemini"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "/v1/responses"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Supported for ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai-compatible"
            }), ", ", (0,jsx_runtime.jsx)(_components.code, {
              children: "anthropic"
            }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
              children: "gemini"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "/v1/images/generations"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Supported for ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai"
            }), " and ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai-compatible"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "/v1/audio/transcriptions"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Supported for ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai"
            }), " and ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai-compatible"
            })]
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "/v1/audio/speech"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST"
            })
          }), (0,jsx_runtime.jsxs)(_components.td, {
            children: ["Supported for ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai"
            }), " and ", (0,jsx_runtime.jsx)(_components.code, {
              children: "openai-compatible"
            })]
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "provider-support-matrix",
      children: "Provider Support Matrix"
    }), "\n", (0,jsx_runtime.jsxs)(_components.table, {
      children: [(0,jsx_runtime.jsx)(_components.thead, {
        children: (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.th, {
            children: "Operation"
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
          })]
        })
      }), (0,jsx_runtime.jsxs)(_components.tbody, {
        children: [(0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET /v1/models"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "GET /v1/billing/usage"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /v1/chat/completions"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
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
          })]
        }), (0,jsx_runtime.jsxs)(_components.tr, {
          children: [(0,jsx_runtime.jsx)(_components.td, {
            children: (0,jsx_runtime.jsx)(_components.code, {
              children: "POST /v1/responses"
            })
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
          }), (0,jsx_runtime.jsx)(_components.td, {
            children: "Yes"
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
          })]
        })]
      })]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "streaming",
      children: "Streaming"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Streaming uses OpenAI-compatible Server-Sent Events."
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/chat/completions"
        }), " supports JSON and SSE streaming"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "POST /v1/responses"
        }), " supports JSON and SSE streaming where implemented"]
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
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "usage-and-scope",
      children: "Usage And Scope"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "GET /v1/billing/usage"
      }), " returns aggregated in-process usage summaries."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["When the authenticated client has a ", (0,jsx_runtime.jsx)(_components.code, {
          children: "tenant"
        }), ", results are scoped to that tenant"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Otherwise, results are scoped to the caller's client identity"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "error-behavior",
      children: "Error Behavior"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Direct requests never fail over to another provider"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Alias requests retry the next target on transport errors and upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "5xx"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Alias requests also retry on upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "4xx"
        }), " responses listed in ", (0,jsx_runtime.jsx)(_components.code, {
          children: "retry_status_codes"
        }), " (default: ", (0,jsx_runtime.jsx)(_components.code, {
          children: "5xx"
        }), " only)"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Other upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "4xx"
        }), " responses are returned verbatim"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Unsupported operations return client-visible proxy errors"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This behavior is deliberate: direct model requests are explicit, while alias requests are the only place where the proxy is allowed to choose another target."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "provider-coverage-notes",
      children: "Provider Coverage Notes"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "openai"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "openai-compatible"
        }), " are close to pass-through adapters"]
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