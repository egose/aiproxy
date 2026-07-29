"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[588],{

/***/ 975
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_deployment_md_a37_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-deployment-md-a37.json
const site_docs_deployment_md_a37_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"deployment","title":"Deployment","description":"This page covers practical ways to run aiproxy in local, containerized, and service-managed environments.","source":"@site/docs/deployment.md","sourceDirName":".","slug":"/deployment","permalink":"/docs/deployment","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":8,"frontMatter":{"sidebar_position":8},"sidebar":"docsSidebar","previous":{"title":"Operations","permalink":"/docs/operations"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/deployment.md


const frontMatter = {
	sidebar_position: 8
};
const contentTitle = 'Deployment';

const assets = {

};



const toc = [{
  "value": "Deployment Defaults",
  "id": "deployment-defaults",
  "level": 2
}, {
  "value": "Local Binary",
  "id": "local-binary",
  "level": 2
}, {
  "value": "Docker",
  "id": "docker",
  "level": 2
}, {
  "value": "Docker Compose",
  "id": "docker-compose",
  "level": 2
}, {
  "value": "systemd",
  "id": "systemd",
  "level": 2
}, {
  "value": "Reloading Config",
  "id": "reloading-config",
  "level": 2
}, {
  "value": "Reverse Proxying",
  "id": "reverse-proxying",
  "level": 2
}, {
  "value": "Production Recommendations",
  "id": "production-recommendations",
  "level": 2
}, {
  "value": "Rollout Checklist",
  "id": "rollout-checklist",
  "level": 2
}];
function _createMdxContent(props) {
  const _components = {
    code: "code",
    h1: "h1",
    h2: "h2",
    header: "header",
    li: "li",
    ol: "ol",
    p: "p",
    pre: "pre",
    ul: "ul",
    ...(0,lib/* useMDXComponents */.R)(),
    ...props.components
  };
  return (0,jsx_runtime.jsxs)(jsx_runtime.Fragment, {
    children: [(0,jsx_runtime.jsx)(_components.header, {
      children: (0,jsx_runtime.jsx)(_components.h1, {
        id: "deployment",
        children: "Deployment"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["This page covers practical ways to run ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " in local, containerized, and service-managed environments."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "deployment-defaults",
      children: "Deployment Defaults"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The project ships as:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["a single Go binary named ", (0,jsx_runtime.jsx)(_components.code, {
          children: "aiproxy"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["a container image built from the repo ", (0,jsx_runtime.jsx)(_components.code, {
          children: "Dockerfile"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The container image:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["exposes port ", (0,jsx_runtime.jsx)(_components.code, {
          children: "8080"
        })]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "runs as non-root"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["starts with ", (0,jsx_runtime.jsx)(_components.code, {
          children: "aiproxy serve --config /etc/aiproxy/config.hcl"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "local-binary",
      children: "Local Binary"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Build the binary:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "make build\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Run it:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "./dist/aiproxy serve --config /etc/aiproxy/config.hcl\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Validate config without starting the server:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "./dist/aiproxy validate --config /etc/aiproxy/config.hcl\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "docker",
      children: "Docker"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Build the image:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "make docker-build\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Run it with a mounted config file:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "docker run --rm \\\n  -p 8080:8080 \\\n  -v ./config.hcl:/etc/aiproxy/config.hcl:ro \\\n  --env-file .env \\\n  aiproxy:latest\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["If you use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ", also mount the key file and point the provider config at that path."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "docker-compose",
      children: "Docker Compose"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Example ", (0,jsx_runtime.jsx)(_components.code, {
        children: "compose.yaml"
      }), ":"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-yaml",
        children: "services:\n  aiproxy:\n    image: aiproxy:latest\n    ports:\n      - '8080:8080'\n    env_file:\n      - .env\n    volumes:\n      - ./config.hcl:/etc/aiproxy/config.hcl:ro\n      - ./keys.json:/etc/aiproxy/keys.json:ro\n    restart: unless-stopped\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "With this setup, a provider can reference the mounted key file with:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "api_key_ref {\n  path = \"/etc/aiproxy/keys.json\"\n  key  = \"openai\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "systemd",
      children: "systemd"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Example unit file:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-ini",
        children: "[Unit]\nDescription=aiproxy\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nUser=aiproxy\nGroup=aiproxy\nWorkingDirectory=/etc/aiproxy\nEnvironmentFile=/etc/aiproxy/aiproxy.env\nExecStart=/usr/local/bin/aiproxy serve --config /etc/aiproxy/config.hcl\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=multi-user.target\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Recommended layout:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["binary at ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/usr/local/bin/aiproxy"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["config at ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/etc/aiproxy/config.hcl"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["env file at ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/etc/aiproxy/aiproxy.env"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["optional key file at ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/etc/aiproxy/keys.json"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "reloading-config",
      children: "Reloading Config"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " supports runtime reload on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), " for auth, providers, models, aliases, and metrics-backed inventory state."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Reload with:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "kill -HUP <pid>\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "With systemd:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "systemctl kill -s HUP aiproxy\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "For a container:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "docker kill --signal HUP <container>\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Listener address and timeout changes still require a full restart."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "reverse-proxying",
      children: "Reverse Proxying"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " is commonly run behind an external load balancer or reverse proxy."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Typical responsibilities of the outer layer:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "TLS termination"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "public DNS and certificates"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "network-level access control"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "request logging outside the application process"
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Keep the proxy-visible auth boundary enabled unless the deployment is fully trusted end to end."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "production-recommendations",
      children: "Production Recommendations"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["use ", (0,jsx_runtime.jsx)(_components.code, {
          children: "bearer_static"
        }), " auth unless another trusted boundary makes it unnecessary"]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "keep secrets out of the HCL file when possible"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "mount config and key files read-only"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["scrape ", (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /metrics"
        })]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "use aliases for stable client-facing models and controlled failover"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["use ", (0,jsx_runtime.jsx)(_components.code, {
          children: "provider_health"
        }), " with Redis when you need transient health sharing across instances"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["validate config before rollout with ", (0,jsx_runtime.jsx)(_components.code, {
          children: "aiproxy validate --config ..."
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "rollout-checklist",
      children: "Rollout Checklist"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ol, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Validate the config before deploy."
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Confirm required environment variables and key files are present."
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Start the service and verify ", (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /v1/models"
        }), "."]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "Test one direct model and one alias-backed model."
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["Confirm ", (0,jsx_runtime.jsx)(_components.code, {
          children: "GET /metrics"
        }), " is scraped successfully."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["If using reloads, test a ", (0,jsx_runtime.jsx)(_components.code, {
          children: "SIGHUP"
        }), " config reload in a non-production environment first."]
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