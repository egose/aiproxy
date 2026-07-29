"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[873],{

/***/ 2420
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  assets: () => (/* binding */ assets),
  contentTitle: () => (/* binding */ contentTitle),
  "default": () => (/* binding */ MDXContent),
  frontMatter: () => (/* binding */ frontMatter),
  metadata: () => (/* reexport */ site_docs_configuration_md_9ed_namespaceObject),
  toc: () => (/* binding */ toc)
});

;// ./.docusaurus/docusaurus-plugin-content-docs/default/site-docs-configuration-md-9ed.json
const site_docs_configuration_md_9ed_namespaceObject = /*#__PURE__*/JSON.parse('{"id":"configuration","title":"Configuration","description":"aiproxy uses labeled HCL blocks. The core building blocks are:","source":"@site/docs/configuration.md","sourceDirName":".","slug":"/configuration","permalink":"/docs/configuration","draft":false,"unlisted":false,"tags":[],"version":"current","sidebarPosition":3,"frontMatter":{"sidebar_position":3},"sidebar":"docsSidebar","previous":{"title":"Quickstart","permalink":"/docs/quickstart"},"next":{"title":"Config Examples","permalink":"/docs/config-examples"}}');
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
// EXTERNAL MODULE: ./node_modules/.pnpm/@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6/node_modules/@mdx-js/react/lib/index.js
var lib = __webpack_require__(1982);
;// ./docs/configuration.md


const frontMatter = {
	sidebar_position: 3
};
const contentTitle = 'Configuration';

const assets = {

};



const toc = [{
  "value": "Mental Model",
  "id": "mental-model",
  "level": 2
}, {
  "value": "Example",
  "id": "example",
  "level": 2
}, {
  "value": "Listener",
  "id": "listener",
  "level": 2
}, {
  "value": "Logging",
  "id": "logging",
  "level": 2
}, {
  "value": "Auth",
  "id": "auth",
  "level": 2
}, {
  "value": "Providers",
  "id": "providers",
  "level": 2
}, {
  "value": "Models",
  "id": "models",
  "level": 2
}, {
  "value": "Secrets And Environment Variables",
  "id": "secrets-and-environment-variables",
  "level": 2
}, {
  "value": "<code>api_key_ref</code>",
  "id": "api_key_ref",
  "level": 2
}, {
  "value": "Naming Rules",
  "id": "naming-rules",
  "level": 2
}, {
  "value": "Validation Rules",
  "id": "validation-rules",
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
        id: "configuration",
        children: "Configuration"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " uses labeled HCL blocks. The core building blocks are:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "listener \"http\" \"public\""
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "auth \"main\""
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "logging"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "provider \"<type>\" \"<name>\""
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "alias \"<name>\""
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "mental-model",
      children: "Mental Model"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Think about the config in five layers:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ol, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "listener"
        }), " defines how the proxy accepts traffic."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "auth"
        }), " defines who may call it."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "logging"
        }), " defines structured log verbosity and request lifecycle access logging."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "provider"
        }), " blocks define upstream systems and their models."]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "alias"
        }), " blocks define the client-facing virtual models used for routing and failover."]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "example",
      children: "Example"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "listener \"http\" \"public\" {\n  address = \":8080\"\n\n  timeouts {\n    read_header = \"10s\"\n    idle        = \"60s\"\n    write       = \"0s\"\n  }\n}\n\nauth \"main\" {\n  mode = \"bearer_static\"\n\n  rate_limit {\n    requests_per_minute = 120\n    burst               = 120\n  }\n\n  client \"internal-app\" {\n    token          = env(\"AIPROXY_CLIENT_TOKEN\")\n    tenant         = \"internal\"\n    allowed_models = [\"alias/chat_default\", \"openai/gpt-4.1\"]\n  }\n}\n\nlogging {\n  level      = \"info\"\n  access_log = true\n}\n\nprovider \"openai\" \"openai\" {\n  display_name = \"OpenAI\"\n  api_key      = env(\"OPENAI_API_KEY\")\n\n  model \"gpt-4.1\" {\n    display_name = \"GPT-4.1\"\n    capabilities = [\"chat\", \"responses\"]\n  }\n\n  model \"text-embedding-3-large\" {\n    display_name = \"text-embedding-3-large\"\n    capabilities = [\"embeddings\"]\n  }\n}\n\nprovider \"openai-compatible\" \"localai\" {\n  display_name = \"LocalAI\"\n  base_url     = \"https://llm.internal/v1\"\n\n  api_key_ref {\n    key = \"localai\"\n  }\n\n  model \"qwen3-32b\" {\n    display_name = \"Qwen 3 32B\"\n  }\n}\n\nalias \"chat_default\" {\n  algorithm = \"round_robin\"\n\n  target {\n    provider = \"openai\"\n    model    = \"gpt-4.1\"\n  }\n\n  target {\n    provider = \"localai\"\n    model    = \"qwen3-32b\"\n  }\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "listener",
      children: "Listener"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The listener block configures the inbound HTTP server."
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "address"
        }), " sets the listen address such as ", (0,jsx_runtime.jsx)(_components.code, {
          children: ":8080"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "timeouts"
        }), " configures read, idle, and write timeouts"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Listener address and timeout changes still require a restart, even though some runtime state can reload on ", (0,jsx_runtime.jsx)(_components.code, {
        children: "SIGHUP"
      }), "."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "For most deployments, one HTTP listener is enough."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "logging",
      children: "Logging"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "logging"
      }), " block controls structured application logs."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "level"
        }), " accepts ", (0,jsx_runtime.jsx)(_components.code, {
          children: "debug"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "info"
        }), ", ", (0,jsx_runtime.jsx)(_components.code, {
          children: "warn"
        }), ", or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "error"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "access_log"
        }), " enables or disables request lifecycle logs such as request received, upstream request start and finish, and response sent or stream start and finish"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Defaults:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "level = \"info\""
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "access_log = true"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "auth",
      children: "Auth"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Supported inbound auth modes:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "none"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "bearer_static"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "none"
      }), " is only appropriate for trusted environments."]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "bearer_static"
      }), " validates client bearer tokens against statically configured ", (0,jsx_runtime.jsx)(_components.code, {
        children: "client"
      }), " blocks. Each client may also define:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "tenant"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "allowed_models"
        })
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["The optional ", (0,jsx_runtime.jsx)(_components.code, {
        children: "rate_limit"
      }), " block is local and in-memory:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["In ", (0,jsx_runtime.jsx)(_components.code, {
          children: "bearer_static"
        }), " mode, it is keyed per authenticated client"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["In ", (0,jsx_runtime.jsx)(_components.code, {
          children: "none"
        }), " mode, it applies to a shared anonymous bucket"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "allowed_models"
      }), " when you want a static allow-list at the proxy boundary rather than relying only on application-level policy."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "providers",
      children: "Providers"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Providers always use two labels:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "provider \"<type>\" \"<name>\" {}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Common attributes:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "display_name"
        })
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "base_url"
        }), " for ", (0,jsx_runtime.jsx)(_components.code, {
          children: "openai-compatible"
        })]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key"
        })
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key_ref"
        })
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["nested ", (0,jsx_runtime.jsx)(_components.code, {
          children: "model"
        }), " blocks"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Exactly one of ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key"
      }), " or ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), " must be set for a provider."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Provider names are part of the public model string, so keep them stable and machine-friendly."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "models",
      children: "Models"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Each provider contains one or more ", (0,jsx_runtime.jsx)(_components.code, {
        children: "model"
      }), " blocks:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "model \"gpt-4.1\" {\n  display_name = \"GPT-4.1\"\n  upstream_name = \"gpt-4.1\"\n  capabilities  = [\"chat\", \"responses\"]\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "The block label is the proxy-visible model name"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "display_name"
        }), " is optional metadata"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "upstream_name"
        }), " lets the upstream identifier differ from the public name"]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "capabilities"
        }), " narrows the operations exposed through the proxy"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "upstream_name"
      }), " when you want a cleaner or more stable public model name than the exact upstream identifier."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Supported capability values:"
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
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "secrets-and-environment-variables",
      children: "Secrets And Environment Variables"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "env(\"VAR\")"
      }), " anywhere a string is allowed. Values are inlined before HCL parsing."]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "That makes it suitable for API keys, bearer tokens, URLs, and other deployment-specific values."
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["For local runs, if your config depends on variables in ", (0,jsx_runtime.jsx)(_components.code, {
        children: ".env"
      }), ", load them first:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-sh",
        children: "set -a; . ./.env; set +a\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "api_key_ref",
      children: (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["When a provider uses ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), ", ", (0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " reads the secret from a JSON file:"]
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-json",
        children: "{\n  \"openai\": \"sk-...\",\n  \"localai\": \"secret\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "The default path is:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: (0,jsx_runtime.jsx)(_components.code, {
          children: "$XDG_CONFIG_HOME/aiproxy/keys.json"
        })
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "~/.config/aiproxy/keys.json"
        }), " when ", (0,jsx_runtime.jsx)(_components.code, {
          children: "XDG_CONFIG_HOME"
        }), " is unset"]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "You can override the file path per provider:"
    }), "\n", (0,jsx_runtime.jsx)(_components.pre, {
      children: (0,jsx_runtime.jsx)(_components.code, {
        className: "language-hcl",
        children: "api_key_ref {\n  path = \"/etc/aiproxy/keys.json\"\n  key  = \"localai\"\n}\n"
      })
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: ["Use ", (0,jsx_runtime.jsx)(_components.code, {
        children: "api_key_ref"
      }), " when you want provider secrets stored outside the main HCL file."]
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "naming-rules",
      children: "Naming Rules"
    }), "\n", (0,jsx_runtime.jsxs)(_components.p, {
      children: [(0,jsx_runtime.jsx)(_components.code, {
        children: "aiproxy"
      }), " keeps public names intentionally strict:"]
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "provider names are lowercase"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "alias names are lowercase"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "names must not contain spaces"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["names must not contain ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/"
        })]
      }), "\n"]
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "These rules keep model parsing simple and unambiguous."
    }), "\n", (0,jsx_runtime.jsx)(_components.h2, {
      id: "validation-rules",
      children: "Validation Rules"
    }), "\n", (0,jsx_runtime.jsx)(_components.p, {
      children: "Startup fails on invalid configuration. Important checks include:"
    }), "\n", (0,jsx_runtime.jsxs)(_components.ul, {
      children: ["\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "duplicate provider or alias names"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "invalid provider types or alias algorithms"
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["names that are not lowercase or contain spaces or ", (0,jsx_runtime.jsx)(_components.code, {
          children: "/"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: [(0,jsx_runtime.jsx)(_components.code, {
          children: "openai-compatible"
        }), " providers missing ", (0,jsx_runtime.jsx)(_components.code, {
          children: "base_url"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["providers with both ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key"
        }), " and ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key_ref"
        })]
      }), "\n", (0,jsx_runtime.jsxs)(_components.li, {
        children: ["providers with neither ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key"
        }), " nor ", (0,jsx_runtime.jsx)(_components.code, {
          children: "api_key_ref"
        })]
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "providers without any models"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "aliases without any targets"
      }), "\n", (0,jsx_runtime.jsx)(_components.li, {
        children: "alias targets that reference unknown providers or models"
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