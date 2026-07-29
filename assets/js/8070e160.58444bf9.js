"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[822],{

/***/ 6718
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_quickstart_md_807_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-quickstart-md-807.json
const site_docs_quickstart_md_807_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"quickstart","title":"Quickstart","description":"This quickstart brings up aiproxy locally with one OpenAI model and no inbound auth.","source":"@site/docs/quickstart.md","sourceDirName":".","slug":"/quickstart","permalink":"/docs/quickstart","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":2,"frontMatter":{"sidebar_position":2},"sidebar":"docsSidebar","previous":{"title":"aiproxy","permalink":"/docs/intro"},"next":{"title":"Configuration","permalink":"/docs/configuration"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/quickstart.md


const frontMatter = {
	sidebar_position: 2
};
const contentTitle = 'Quickstart';

const assets = {

};



const toc = [{
  "value": "Before You Start",
  "id": "before-you-start",
  "level": 2
}, {
  "value": "Install",
  "id": "install",
  "level": 2
}, {
  "value": "Minimal Config",
  "id": "minimal-config",
  "level": 2
}, {
  "value": "Validate And Run",
  "id": "validate-and-run",
  "level": 2
}, {
  "value": "Send A Request",
  "id": "send-a-request",
  "level": 2
}, {
  "value": "Add Static Bearer Auth",
  "id": "add-static-bearer-auth",
  "level": 2
}, {
  "value": "Next Steps",
  "id": "next-steps",
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
    p: "p",
    pre: "pre",
    ul: "ul",
    ...(0,lib/* useMDXComponents */.R)(),
    ...props.components
  };
  return (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
    children: [(0,jsx_runtime.jsx)(_components.header, {
      children: (0,jsx_runtime.jsx)(_components.h1, {
        id: "quickstart",
        children: "Quickstart"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["This quickstart brings up ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " locally with one OpenAI model and no inbound auth."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "before-you-start",
      children: "Before You Start"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "You need:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["an ", (0,jsx_runtime.jsx)(_components.code, {
          children: "OPENAI_API_KEY"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["either an installed ", (0,jsx_runtime.jsx)(_components.code, {
          children: "aiproxy"
        }), " binary or a local checkout of the repository"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "install",
      children: "Install"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Install via ", (0,jsx_runtime.jsx)(_components.code, {
        children: "asdf"
      }), ":"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "asdf plugin add aiproxy\n# or\nasdf plugin add aiproxy https://github.com/egose/aiproxy.git\n\nasdf install aiproxy latest\nasdf global aiproxy latest\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Or run directly from source:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "make build\n./dist/aiproxy version\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "minimal-config",
      children: "Minimal Config"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Create ", (0,jsx_runtime.jsx)(_components.code, {
        children: "config.hcl"
      }), ":"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "listener \"http\" \"public\" {\n  address = \":8080\"\n}\n\nauth \"main\" {\n  mode = \"none\"\n}\n\nprovider \"openai\" \"openai\" {\n  display_name = \"OpenAI\"\n  api_key      = env(\"OPENAI_API_KEY\")\n\n  model \"gpt-4o-mini\" {\n    display_name = \"GPT-4o mini\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "validate-and-run",
      children: "Validate And Run"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Export any environment variables referenced by ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env(\"...\")"
      }), " before starting the proxy:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "export OPENAI_API_KEY=sk-...\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["If you keep secrets in a local ", (0,jsx_runtime.jsx)(_components.code, {
        children: ".env"
      }), " file, load them before running the CLI:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "set -a; . ./.env; set +a\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Validate the configuration:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy validate --config ./config.hcl\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Start the server:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "aiproxy serve --config ./config.hcl\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "From source, the equivalent command is:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "go run ./cmd/aiproxy serve --config ./config.hcl\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "send-a-request",
      children: "Send A Request"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/chat/completions \\\n  -H 'Content-Type: application/json' \\\n  -d '{\n    \"model\": \"openai/gpt-4o-mini\",\n    \"messages\": [\n      {\"role\": \"user\", \"content\": \"Say hello in one sentence.\"}\n    ]\n  }'\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "You can also inspect the exposed model catalog:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/models\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "add-static-bearer-auth",
      children: "Add Static Bearer Auth"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For anything beyond a trusted local environment, switch from ", (0,jsx_runtime.jsx)(_components.code, {
        children: "mode = \"none\""
      }), " to ", (0,jsx_runtime.jsx)(_components.code, {
        children: "bearer_static"
      }), ":"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "auth \"main\" {\n  mode = \"bearer_static\"\n\n  client \"local-dev\" {\n    token = env(\"AIPROXY_CLIENT_TOKEN\")\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Requests then need an ", (0,jsx_runtime.jsx)(_components.code, {
        children: "Authorization"
      }), " header:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "curl http://127.0.0.1:8080/v1/models \\\n  -H 'Authorization: Bearer your-token'\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "That same token must be sent to every protected endpoint, including chat, responses, embeddings, and model discovery."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "next-steps",
      children: "Next Steps"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Add more providers in ", (0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/configuration",
          children: "Configuration"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Expose alias-backed models in ", (0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/providers-and-routing",
          children: "Providers and Routing"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Check the endpoint and provider matrix in ", (0,jsx_runtime.jsx)(_components.a, {
          href: "/docs/api-reference",
          children: "API Reference"
        })]
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