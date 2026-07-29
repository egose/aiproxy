"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[583],{

/***/ 7394
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ Home)
});

// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+core@3.10.1_@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6__clean-cs_795e68a4f72a2ce324d6ed7bc0d84830/node_modules/@docusaurus/core/lib/client/exports/Link.js
var Link = __webpack_require__(9471);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+core@3.10.1_@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6__clean-cs_795e68a4f72a2ce324d6ed7bc0d84830/node_modules/@docusaurus/core/lib/client/exports/useDocusaurusContext.js
var useDocusaurusContext = __webpack_require__(3959);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-classic@3.10.1_@types+react@19.2.14_clean-css@5.3.3_cssnano@6.1.2_pos_63cba73bf6af7eefa9e89eca4a244dee/node_modules/@docusaurus/theme-classic/lib/theme/Layout/index.js + 72 modules
var Layout = __webpack_require__(5416);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-classic@3.10.1_@types+react@19.2.14_clean-css@5.3.3_cssnano@6.1.2_pos_63cba73bf6af7eefa9e89eca4a244dee/node_modules/@docusaurus/theme-classic/lib/theme/Heading/index.js + 1 modules
var Heading = __webpack_require__(2245);
;// ./src/pages/index.module.css
// extracted by mini-css-extract-plugin
/* harmony default export */ const index_module = ({"heroBanner":"heroBanner_qdFl","heroInner":"heroInner_V4lS","eyebrow":"eyebrow_kY3W","heroTitle":"heroTitle_qg2I","heroSubtitle":"heroSubtitle_jFu1","heroDescription":"heroDescription_UJGW","buttons":"buttons_AeoN","main":"main_iUjq","highlightsSection":"highlightsSection_xw1o","quickLinksSection":"quickLinksSection_U9OU","highlightsGrid":"highlightsGrid_BIrr","card":"card_M5pr","quickLinksPanel":"quickLinksPanel_CSI0","cardTitle":"cardTitle_tke3","cardBody":"cardBody_y6pQ","quickLinksTitle":"quickLinksTitle_hBCr","quickLinksRow":"quickLinksRow_koa0","quickLink":"quickLink_vT2w"});
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
;// ./src/pages/index.tsx
const highlights=[{title:'One API Surface',body:'Expose OpenAI-compatible endpoints while mixing OpenAI, Anthropic, Gemini, and OpenAI-compatible backends.'},{title:'Proxy-Owned Routing',body:'Publish direct models and alias-backed virtual models without coupling clients to a single upstream provider.'},{title:'Built For Operations',body:'Validate config, reload routing state on SIGHUP, export Prometheus metrics, and keep credentials out of logs.'}];function HomepageHeader(){const{siteConfig}=(0,useDocusaurusContext/* default */.A)();return/*#__PURE__*/(0,jsx_runtime.jsx)("header",{className:index_module.heroBanner,children:/*#__PURE__*/(0,jsx_runtime.jsxs)("div",{className:index_module.heroInner,children:[/*#__PURE__*/(0,jsx_runtime.jsx)("p",{className:index_module.eyebrow,children:"Documentation"}),/*#__PURE__*/(0,jsx_runtime.jsx)(Heading/* default */.A,{as:"h1",className:index_module.heroTitle,children:siteConfig.title}),/*#__PURE__*/(0,jsx_runtime.jsx)("p",{className:index_module.heroSubtitle,children:siteConfig.tagline}),/*#__PURE__*/(0,jsx_runtime.jsx)("p",{className:index_module.heroDescription,children:"Route multiple AI providers through one service, keep the public API consistent, and let the proxy own model naming, auth, and failover behavior."}),/*#__PURE__*/(0,jsx_runtime.jsxs)("div",{className:index_module.buttons,children:[/*#__PURE__*/(0,jsx_runtime.jsx)(Link/* default */.A,{className:"button button--primary button--lg",to:"/docs/quickstart",children:"Quickstart"}),/*#__PURE__*/(0,jsx_runtime.jsx)(Link/* default */.A,{className:"button button--secondary button--lg",to:"/docs/intro",children:"Browse Docs"})]})]})});}function Home(){const{siteConfig}=(0,useDocusaurusContext/* default */.A)();return/*#__PURE__*/(0,jsx_runtime.jsxs)(Layout/* default */.A,{title:siteConfig.title,description:"Documentation for aiproxy, an OpenAI-compatible proxy for multiple AI providers.",children:[/*#__PURE__*/(0,jsx_runtime.jsx)(HomepageHeader,{}),/*#__PURE__*/(0,jsx_runtime.jsxs)("main",{className:index_module.main,children:[/*#__PURE__*/(0,jsx_runtime.jsx)("section",{className:index_module.highlightsSection,children:/*#__PURE__*/(0,jsx_runtime.jsx)("div",{className:index_module.highlightsGrid,children:highlights.map(highlight=>/*#__PURE__*/(0,jsx_runtime.jsxs)("article",{className:index_module.card,children:[/*#__PURE__*/(0,jsx_runtime.jsx)(Heading/* default */.A,{as:"h2",className:index_module.cardTitle,children:highlight.title}),/*#__PURE__*/(0,jsx_runtime.jsx)("p",{className:index_module.cardBody,children:highlight.body})]},highlight.title))})}),/*#__PURE__*/(0,jsx_runtime.jsx)("section",{className:index_module.quickLinksSection,children:/*#__PURE__*/(0,jsx_runtime.jsxs)("div",{className:index_module.quickLinksPanel,children:[/*#__PURE__*/(0,jsx_runtime.jsx)(Heading/* default */.A,{as:"h2",className:index_module.quickLinksTitle,children:"Start with the essentials"}),/*#__PURE__*/(0,jsx_runtime.jsxs)("div",{className:index_module.quickLinksRow,children:[/*#__PURE__*/(0,jsx_runtime.jsx)(Link/* default */.A,{className:index_module.quickLink,to:"/docs/configuration",children:"Configuration"}),/*#__PURE__*/(0,jsx_runtime.jsx)(Link/* default */.A,{className:index_module.quickLink,to:"/docs/config-examples",children:"Config Examples"}),/*#__PURE__*/(0,jsx_runtime.jsx)(Link/* default */.A,{className:index_module.quickLink,to:"/docs/providers-and-routing",children:"Providers and Routing"}),/*#__PURE__*/(0,jsx_runtime.jsx)(Link/* default */.A,{className:index_module.quickLink,to:"/docs/request-examples",children:"Request Examples"}),/*#__PURE__*/(0,jsx_runtime.jsx)(Link/* default */.A,{className:index_module.quickLink,to:"/docs/api-reference",children:"API Reference"}),/*#__PURE__*/(0,jsx_runtime.jsx)(Link/* default */.A,{className:index_module.quickLink,to:"/docs/deployment",children:"Deployment"})]})]})})]})]});}

/***/ }

}]);