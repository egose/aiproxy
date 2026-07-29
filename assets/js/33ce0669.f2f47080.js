"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[68],{

/***/ 5978
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_request_examples_md_33c_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-request-examples-md-33c.json
const site_docs_request_examples_md_33c_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"request-examples","title":"Request Examples","description":"These examples show how clients typically call aiproxy through its OpenAI-compatible API.","source":"@site/docs/request-examples.md","sourceDirName":".","slug":"/request-examples","permalink":"/docs/request-examples","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":6,"frontMatter":{"sidebar_position":6},"sidebar":"docsSidebar","previous":{"title":"Providers and Routing","permalink":"/docs/providers-and-routing"},"next":{"title":"API Reference","permalink":"/docs/api-reference"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/request-examples.md


const frontMatter = {
	sidebar_position: 6
};
const contentTitle = 'Request Examples';

const assets = {

};



const toc = [{
  "value": "Common Headers",
  "id": "common-headers",
  "level": 2
}, {
  "value": "List Models",
  "id": "list-models",
  "level": 2
}, {
  "value": "Chat Completions",
  "id": "chat-completions",
  "level": 2
}, {
  "value": "Streaming Chat Completions",
  "id": "streaming-chat-completions",
  "level": 2
}, {
  "value": "Responses API",
  "id": "responses-api",
  "level": 2
}, {
  "value": "Embeddings",
  "id": "embeddings",
  "level": 2
}, {
  "value": "Image Generation",
  "id": "image-generation",
  "level": 2
}, {
  "value": "Audio Endpoints",
  "id": "audio-endpoints",
  "level": 2
}, {
  "value": "Billing Usage",
  "id": "billing-usage",
  "level": 2
}, {
  "value": "Error Behavior Examples",
  "id": "error-behavior-examples",
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
        id: "request-examples",
        children: "Request Examples"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["These examples show how clients typically call ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " through its OpenAI-compatible API."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["All request bodies use proxy-visible model names such as ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai/gpt-4.1"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "alias/chat_default"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "common-headers",
      children: "Common Headers"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["With ", (0,jsx_runtime.jsx)(_components.code, {
        children: "bearer_static"
      }), " auth enabled:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-http",
        children: "Authorization: Bearer your-token\nContent-Type: application/json\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["With ", (0,jsx_runtime.jsx)(_components.code, {
        children: "none"
      }), " auth enabled, omit the ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization"
      }), " header."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "list-models",
      children: "List Models"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/models \\\n  -H 'Authorization: Bearer your-token'\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Representative response shape:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-json",
        children: "{\n  \"object\": \"list\",\n  \"data\": [\n    {\n      \"id\": \"openai/gpt-4.1\",\n      \"object\": \"model\",\n      \"display_name\": \"GPT-4.1\",\n      \"provider_type\": \"openai\",\n      \"capabilities\": [\"chat\", \"responses\"]\n    },\n    {\n      \"id\": \"alias/chat_default\",\n      \"object\": \"model\",\n      \"capabilities\": [\"chat\", \"responses\"],\n      \"alias_targets\": [\n        { \"provider\": \"openai\", \"model\": \"gpt-4.1\", \"display_name\": \"GPT-4.1\" },\n        { \"provider\": \"localai\", \"model\": \"qwen3-32b\", \"display_name\": \"Qwen 3 32B\" }\n      ]\n    }\n  ]\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "chat-completions",
      children: "Chat Completions"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Request:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/chat/completions \\\n  -H 'Authorization: Bearer your-token' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n    \"model\": \"alias/chat_default\",\n    \"messages\": [\n      {\"role\": \"system\", \"content\": \"Be concise.\"},\n      {\"role\": \"user\", \"content\": \"Give me three rollout checks for a new proxy deployment.\"}\n    ]\n  }'\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Representative response shape:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-json",
        children: "{\n  \"id\": \"chatcmpl-123\",\n  \"object\": \"chat.completion\",\n  \"model\": \"alias/chat_default\",\n  \"choices\": [\n    {\n      \"index\": 0,\n      \"message\": {\n        \"role\": \"assistant\",\n        \"content\": \"1. Verify /v1/models and /metrics respond. 2. Test one direct model and one alias. 3. Confirm upstream failover and auth behavior.\"\n      },\n      \"finish_reason\": \"stop\"\n    }\n  ]\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "streaming-chat-completions",
      children: "Streaming Chat Completions"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/chat/completions \\\n  -H 'Authorization: Bearer your-token' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n    \"model\": \"alias/chat_default\",\n    \"stream\": true,\n    \"messages\": [\n      {\"role\": \"user\", \"content\": \"Write a short status update.\"}\n    ]\n  }'\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The response is an OpenAI-compatible SSE stream. Translated providers are converted back into the same public streaming format."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "responses-api",
      children: "Responses API"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/responses \\\n  -H 'Authorization: Bearer your-token' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n    \"model\": \"openai/gpt-4.1\",\n    \"input\": \"Summarize the purpose of an alias-backed model in one paragraph.\"\n  }'\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use this endpoint only with provider/model combinations that support ", (0,jsx_runtime.jsx)(_components.code, {
        children: "responses"
      }), " through the proxy."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "embeddings",
      children: "Embeddings"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Supported for ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai-compatible"
      }), ", and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "gemini"
      }), " providers."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/embeddings \\\n  -H 'Authorization: Bearer your-token' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n    \"model\": \"openai/text-embedding-3-large\",\n    \"input\": [\n      \"first document\",\n      \"second document\"\n    ]\n  }'\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "image-generation",
      children: "Image Generation"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Supported only for ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai-compatible"
      }), " providers."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/images/generations \\\n  -H 'Authorization: Bearer your-token' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n    \"model\": \"openai/gpt-image-1\",\n    \"prompt\": \"A blueprint-style illustration of a service mesh\"\n  }'\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Only expose image-capable models through the proxy if you actually want clients to use that operation."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "audio-endpoints",
      children: "Audio Endpoints"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "/v1/audio/transcriptions"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "/v1/audio/speech"
      }), " are supported only for ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai"
      }), " and ", (0,jsx_runtime.jsx)(_components.code, {
        children: "openai-compatible"
      }), " providers."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "These endpoints usually involve multipart upload or binary output handling on the client side, but the proxy-visible model naming and auth behavior stay the same."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "billing-usage",
      children: "Billing Usage"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/billing/usage \\\n  -H 'Authorization: Bearer your-token'\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["In ", (0,jsx_runtime.jsx)(_components.code, {
        children: "bearer_static"
      }), " mode, results are scoped to the caller's tenant when present, otherwise to the caller's client identity."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "error-behavior-examples",
      children: "Error Behavior Examples"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["direct request to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "openai/gpt-4.1"
        }), ": never rerouted to another provider"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["alias request to ", (0,jsx_runtime.jsx)(_components.code, {
          children: "alias/chat_default"
        }), ": may retry the next target on timeout or upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "5xx"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["alias request returning upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "4xx"
        }), ": returned to the client without failover"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "request to an unsupported operation: returned as a client-visible proxy error"
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