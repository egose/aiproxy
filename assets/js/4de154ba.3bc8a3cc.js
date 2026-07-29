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
  "value": "Security Defaults",
  "id": "security-defaults",
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
        children: "aiproxy serve\naiproxy validate\naiproxy serve --config /etc/aiproxy/config.hcl\naiproxy validate --config /etc/aiproxy/config.hcl\naiproxy version\n"
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
      }), " is unset."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "When running locally with env-based secrets, load your environment before invoking the binary:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "set -a; . ./.env; set +a\n"
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
        children: "make vet\nmake test\nmake test-race\nmake cover\n"
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
        children: "alias routing state"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "metrics-backed inventory state"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "These changes still require a restart:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "listener address changes"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "listener timeout changes"
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
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Transient transport failures and upstream ", (0,jsx_runtime.jsx)(_components.code, {
        children: "5xx"
      }), " responses can mark a provider unhealthy for routing and readiness decisions."]
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
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Without Redis-backed sharing, each instance tracks transient health independently."
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
        children: ["scrape ", (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /metrics"
        })]
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