"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[848],{

/***/ 3060
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_config_examples_md_0d0_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-config-examples-md-0d0.json
const site_docs_config_examples_md_0d0_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"config-examples","title":"Config Examples","description":"This page collects complete configuration examples for common aiproxy setups.","source":"@site/docs/config-examples.md","sourceDirName":".","slug":"/config-examples","permalink":"/docs/config-examples","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":5,"frontMatter":{"sidebar_position":5},"sidebar":"docsSidebar","previous":{"title":"Configuration","permalink":"/docs/configuration"},"next":{"title":"Providers and Routing","permalink":"/docs/providers-and-routing"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/config-examples.md


const frontMatter = {
	sidebar_position: 5
};
const contentTitle = 'Config Examples';

const assets = {

};



const toc = [{
  "value": "Local Development With One Provider",
  "id": "local-development-with-one-provider",
  "level": 2
}, {
  "value": "OpenAI Plus OpenAI-Compatible Fallback",
  "id": "openai-plus-openai-compatible-fallback",
  "level": 2
}, {
  "value": "Multi-Provider Chat Pool With Tenant-Aware Auth",
  "id": "multi-provider-chat-pool-with-tenant-aware-auth",
  "level": 2
}, {
  "value": "Shared Provider Health Across Instances",
  "id": "shared-provider-health-across-instances",
  "level": 2
}, {
  "value": "Key File Example",
  "id": "key-file-example",
  "level": 2
}, {
  "value": "Tips",
  "id": "tips",
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
        id: "config-examples",
        children: "Config Examples"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["This page collects complete configuration examples for common ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " setups."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Use these as starting points, then adapt provider names, model names, auth settings, and secret sources for your environment."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "local-development-with-one-provider",
      children: "Local Development With One Provider"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This is the smallest useful config. It exposes one OpenAI-backed model and skips inbound auth."
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "listener \"http\" \"public\" {\n  address = \":8080\"\n}\n\nauth \"main\" {\n  mode = \"none\"\n}\n\nprovider \"openai\" \"openai\" {\n  display_name = \"OpenAI\"\n  api_key      = env(\"OPENAI_API_KEY\")\n\n  model \"gpt-4o-mini\" {\n    display_name = \"GPT-4o mini\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Use this when:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "you are testing locally"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "the proxy sits behind another trusted boundary"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "you want the shortest path to a working setup"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "openai-plus-openai-compatible-fallback",
      children: "OpenAI Plus OpenAI-Compatible Fallback"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This setup exposes a stable alias while routing across a hosted model and a self-hosted OpenAI-compatible backend."
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "listener \"http\" \"public\" {\n  address = \":8080\"\n}\n\nauth \"main\" {\n  mode = \"bearer_static\"\n\n  client \"internal-app\" {\n    token          = env(\"AIPROXY_CLIENT_TOKEN\")\n    allowed_models = [\"alias/chat_default\", \"openai/gpt-4.1\"]\n  }\n}\n\nprovider \"openai\" \"openai\" {\n  display_name = \"OpenAI\"\n  api_key      = env(\"OPENAI_API_KEY\")\n\n  model \"gpt-4.1\" {\n    display_name = \"GPT-4.1\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n}\n\nprovider \"openai-compatible\" \"localai\" {\n  display_name = \"LocalAI\"\n  base_url     = \"https://llm.internal/v1\"\n\n  api_key_ref {\n    key = \"localai\"\n  }\n\n  model \"qwen3-32b\" {\n    display_name = \"Qwen 3 32B\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n}\n\nalias \"chat_default\" {\n  algorithm = \"round_robin\"\n\n  target {\n    provider = \"openai\"\n    model    = \"gpt-4.1\"\n  }\n\n  target {\n    provider = \"localai\"\n    model    = \"qwen3-32b\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Use this when:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "clients should call one stable virtual model"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "you want simple balancing across two backends"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["you want alias retry behavior on transport failures and upstream ", (0,jsx_runtime.jsx)(_components.code, {
          children: "5xx"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "multi-provider-chat-pool-with-tenant-aware-auth",
      children: "Multi-Provider Chat Pool With Tenant-Aware Auth"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "This example mixes translated and pass-through providers and adds tenant metadata plus a local rate limit."
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "listener \"http\" \"public\" {\n  address = \":8080\"\n\n  timeouts {\n    read_header = \"10s\"\n    idle        = \"60s\"\n    write       = \"0s\"\n  }\n}\n\nauth \"main\" {\n  mode = \"bearer_static\"\n\n  rate_limit {\n    requests_per_minute = 240\n    burst               = 240\n  }\n\n  client \"team-a-app\" {\n    token          = env(\"TEAM_A_TOKEN\")\n    tenant         = \"team-a\"\n    allowed_models = [\"alias/chat_default\", \"alias/chat_fallback\"]\n  }\n\n  client \"team-b-app\" {\n    token          = env(\"TEAM_B_TOKEN\")\n    tenant         = \"team-b\"\n    allowed_models = [\"alias/chat_default\"]\n  }\n}\n\nprovider \"openai\" \"openai\" {\n  display_name = \"OpenAI\"\n  api_key      = env(\"OPENAI_API_KEY\")\n\n  model \"gpt-4.1\" {\n    display_name = \"GPT-4.1\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n}\n\nprovider \"anthropic\" \"anthropic\" {\n  display_name = \"Anthropic\"\n\n  api_key_ref {\n    key = \"anthropic\"\n  }\n\n  model \"claude-sonnet\" {\n    display_name  = \"Claude Sonnet\"\n    upstream_name = \"claude-sonnet-4-20250514\"\n    capabilities  = [\"chat\", \"responses\"]\n  }\n}\n\nprovider \"gemini\" \"gemini\" {\n  display_name = \"Gemini\"\n  api_key      = env(\"GEMINI_API_KEY\")\n\n  model \"gemini-2.5-pro\" {\n    display_name = \"Gemini 2.5 Pro\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n}\n\nalias \"chat_default\" {\n  algorithm = \"round_robin\"\n\n  target {\n    provider = \"openai\"\n    model    = \"gpt-4.1\"\n  }\n\n  target {\n    provider = \"anthropic\"\n    model    = \"claude-sonnet\"\n  }\n\n  target {\n    provider = \"gemini\"\n    model    = \"gemini-2.5-pro\"\n  }\n}\n\nalias \"chat_fallback\" {\n  algorithm = \"least_connections\"\n\n  target {\n    provider = \"openai\"\n    model    = \"gpt-4.1\"\n  }\n\n  target {\n    provider = \"gemini\"\n    model    = \"gemini-2.5-pro\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Use this when:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "different teams or clients need separate identities"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "you want a stable chat pool spanning multiple providers"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "you want usage summaries scoped by tenant where present"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "shared-provider-health-across-instances",
      children: "Shared Provider Health Across Instances"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Add ", (0,jsx_runtime.jsx)(_components.code, {
        children: "provider_health"
      }), " when you want transient provider health state shared across instances through Redis."]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider_health {\n  redis_url  = \"redis://127.0.0.1:6379\"\n  key_prefix = \"aiproxy:prod\"\n  cooldown   = \"45s\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Use this when:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "you run more than one proxy instance"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "you want transient provider failure state shared between them"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Without this block, provider health is tracked in-process only."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "key-file-example",
      children: "Key File Example"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When you use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ", the key file is a JSON object mapping key names to secret strings:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-json",
        children: "{\n  \"openai\": \"sk-...\",\n  \"anthropic\": \"sk-ant-...\",\n  \"localai\": \"secret\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Point a provider at one of these entries with:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "api_key_ref {\n  path = \"/etc/aiproxy/keys.json\"\n  key  = \"openai\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "tips",
      children: "Tips"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "keep provider names stable because they appear in public model strings"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "use aliases when clients should not depend on a single concrete upstream model"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["use ", (0,jsx_runtime.jsx)(_components.code, {
          children: "capabilities"
        }), " to narrow the public contract to the operations you actually want to expose"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "keep secrets in environment variables or a key file instead of inline literals"
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