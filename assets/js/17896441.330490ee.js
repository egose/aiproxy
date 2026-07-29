"use strict";
(globalThis["webpackChunkwebsite"] = globalThis["webpackChunkwebsite"] || []).push([[401],{

/***/ 348
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   A: () => (/* binding */ AdmonitionLayout)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(489);
/* harmony import */ var clsx__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(3526);
/* harmony import */ var _docusaurus_theme_common__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(1853);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(1325);
const toneClassNames={note:'border-slate-200 bg-slate-50/90 dark:border-slate-700 dark:bg-slate-900/80',info:'border-blue-200 bg-blue-50/80 dark:border-blue-900 dark:bg-blue-950/30',tip:'border-emerald-200 bg-emerald-50/80 dark:border-emerald-900 dark:bg-emerald-950/30',warning:'border-amber-200 bg-amber-50/80 dark:border-amber-900 dark:bg-amber-950/30',danger:'border-rose-200 bg-rose-50/80 dark:border-rose-900 dark:bg-rose-950/30',caution:'border-orange-200 bg-orange-50/80 dark:border-orange-900 dark:bg-orange-950/30'};const iconToneClassNames={note:'text-slate-700 dark:text-slate-200',info:'text-blue-700 dark:text-blue-200',tip:'text-emerald-700 dark:text-emerald-200',warning:'text-amber-700 dark:text-amber-200',danger:'text-rose-700 dark:text-rose-200',caution:'text-orange-700 dark:text-orange-200'};function AdmonitionContainer({type,className,children,id}){return/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_3__.jsx)("div",{className:(0,clsx__WEBPACK_IMPORTED_MODULE_1__/* ["default"] */ .A)(_docusaurus_theme_common__WEBPACK_IMPORTED_MODULE_2__/* .ThemeClassNames */ .G.common.admonition,_docusaurus_theme_common__WEBPACK_IMPORTED_MODULE_2__/* .ThemeClassNames */ .G.common.admonitionType(type),'my-6 overflow-hidden rounded-[1.5rem] border shadow-panel backdrop-blur',toneClassNames[type]??toneClassNames.note,className),id:id,children:children});}function AdmonitionHeading({type,icon,title}){return/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_3__.jsxs)("div",{className:"flex items-center gap-3 border-b border-black/5 px-5 py-4 text-sm font-semibold uppercase tracking-[0.16em] text-slate-900 dark:border-white/10 dark:text-slate-50",children:[/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_3__.jsx)("span",{className:(0,clsx__WEBPACK_IMPORTED_MODULE_1__/* ["default"] */ .A)('inline-flex h-9 w-9 items-center justify-center rounded-full border border-black/5 bg-white/70 dark:border-white/10 dark:bg-white/5',iconToneClassNames[type]??iconToneClassNames.note),children:icon}),title]});}function AdmonitionContent({children}){return children?/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_3__.jsx)("div",{className:"px-5 py-5 text-sm leading-7 text-slate-700 dark:text-slate-300 [&>*:last-child]:mb-0",children:children}):null;}function AdmonitionLayout(props){const{type,icon,title,children,className,id}=props;return/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_3__.jsxs)(AdmonitionContainer,{type:type,className:className,id:id,children:[title||icon?/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_3__.jsx)(AdmonitionHeading,{type:type,title:title,icon:icon}):null,/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_3__.jsx)(AdmonitionContent,{children:children})]});}

/***/ },

/***/ 259
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  A: () => (/* binding */ CodeBlock)
});

// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/index.js
var react = __webpack_require__(489);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+core@3.10.1_@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6__clean-cs_c6bb8a92892675e531eb6c2d31c3baf0/node_modules/@docusaurus/core/lib/client/exports/useIsBrowser.js
var useIsBrowser = __webpack_require__(5997);
// EXTERNAL MODULE: ./node_modules/.pnpm/clsx@2.1.1/node_modules/clsx/dist/clsx.mjs
var clsx = __webpack_require__(3526);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-common@3.10.1_@docusaurus+plugin-content-docs@3.10.1_@mdx-js+react@3._14d5ed44ce36e3ea60836e364ababf16/node_modules/@docusaurus/theme-common/lib/hooks/usePrismTheme.js
var usePrismTheme = __webpack_require__(7460);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-common@3.10.1_@docusaurus+plugin-content-docs@3.10.1_@mdx-js+react@3._14d5ed44ce36e3ea60836e364ababf16/node_modules/@docusaurus/theme-common/lib/utils/ThemeClassNames.js
var ThemeClassNames = __webpack_require__(1853);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-common@3.10.1_@docusaurus+plugin-content-docs@3.10.1_@mdx-js+react@3._14d5ed44ce36e3ea60836e364ababf16/node_modules/@docusaurus/theme-common/lib/utils/codeBlockUtils.js
var codeBlockUtils = __webpack_require__(3669);
// EXTERNAL MODULE: ./node_modules/.pnpm/react@19.2.6/node_modules/react/jsx-runtime.js
var jsx_runtime = __webpack_require__(1325);
;// ./src/theme/CodeBlock/Container/index.tsx
function CodeBlockContainer({as:As,...props}){const prismTheme=(0,usePrismTheme/* usePrismTheme */.A)();const prismCssVariables=(0,codeBlockUtils/* getPrismCssVariables */.M$)(prismTheme);return/*#__PURE__*/(0,jsx_runtime.jsx)(As,{...props,style:prismCssVariables,className:(0,clsx/* default */.A)(props.className,'mb-6 overflow-hidden rounded-[1.5rem] border border-slate-200 bg-slate-950 text-slate-100 shadow-panel dark:border-slate-800',ThemeClassNames/* ThemeClassNames */.G.common.codeBlock)});}
;// ./src/theme/CodeBlock/Content/styles.module.css
// extracted by mini-css-extract-plugin
/* harmony default export */ const styles_module = ({"codeBlock":"codeBlock_qGQc","codeBlockStandalone":"codeBlockStandalone_zC50","codeBlockLines":"codeBlockLines_p187","codeBlockLinesWithNumbering":"codeBlockLinesWithNumbering_OFgW"});
;// ./src/theme/CodeBlock/Content/Element.tsx
// TODO Docusaurus v4: move this component at the root?
// This component only handles a rare edge-case: <pre><MyComp/></pre> in MDX
// <pre> tags in markdown map to CodeBlocks. They may contain JSX children.
// When children is not a simple string, we just return a styled block without
// actually highlighting.
function CodeBlockJSX({children,className}){return/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockContainer,{as:"pre",tabIndex:0,className:(0,clsx/* default */.A)(styles_module.codeBlockStandalone,'thin-scrollbar',className),children:/*#__PURE__*/(0,jsx_runtime.jsx)("code",{className:styles_module.codeBlockLines,children:children})});}
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-common@3.10.1_@docusaurus+plugin-content-docs@3.10.1_@mdx-js+react@3._14d5ed44ce36e3ea60836e364ababf16/node_modules/@docusaurus/theme-common/lib/utils/useThemeConfig.js
var useThemeConfig = __webpack_require__(2140);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-common@3.10.1_@docusaurus+plugin-content-docs@3.10.1_@mdx-js+react@3._14d5ed44ce36e3ea60836e364ababf16/node_modules/@docusaurus/theme-common/lib/hooks/useCodeWordWrap.js + 1 modules
var useCodeWordWrap = __webpack_require__(7414);
;// ./src/theme/CodeBlock/Title/index.tsx
function CodeBlockTitle({children}){return/*#__PURE__*/(0,jsx_runtime.jsx)("span",{className:"inline-flex items-center gap-2 text-slate-200",children:children});}
// EXTERNAL MODULE: ./node_modules/.pnpm/prism-react-renderer@2.4.1_react@19.2.6/node_modules/prism-react-renderer/dist/index.mjs
var dist = __webpack_require__(383);
;// ./src/theme/CodeBlock/Line/Token/index.tsx
// Pass-through components that users can swizzle and customize
function CodeBlockLineToken({line,token,...props}){return/*#__PURE__*/(0,jsx_runtime.jsx)("span",{...props});}
;// ./src/theme/CodeBlock/Line/styles.module.css
// extracted by mini-css-extract-plugin
/* harmony default export */ const Line_styles_module = ({"codeLine":"codeLine_iPqp","codeLineNumber":"codeLineNumber_F4P7","codeLineContent":"codeLineContent_pOih"});
;// ./src/theme/CodeBlock/Line/index.tsx
// This <br/ seems useful when the line has no content to prevent collapsing.
// For code blocks with "diff" languages, this makes the empty lines collapse to
// zero height lines, which is undesirable.
// See also https://github.com/facebook/docusaurus/pull/11565
function LineBreak(){return/*#__PURE__*/(0,jsx_runtime.jsx)("br",{});}// Replaces single lines with '\n' by '' so that we don't end up with
// duplicate line breaks (the '\n' + the artificial <br/> above)
// see also https://github.com/facebook/docusaurus/pull/11565
function fixLineBreak(line){const singleLineBreakToken=line.length===1&&line[0].content==='\n'?line[0]:undefined;if(singleLineBreakToken){return[{...singleLineBreakToken,content:''}];}return line;}function CodeBlockLine({line:lineProp,classNames,showLineNumbers,getLineProps,getTokenProps}){const line=fixLineBreak(lineProp);const lineProps=getLineProps({line,className:(0,clsx/* default */.A)(classNames,showLineNumbers&&Line_styles_module.codeLine)});const lineTokens=line.map((token,key)=>{const tokenProps=getTokenProps({token});return/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockLineToken,{...tokenProps,line:line,token:token,children:tokenProps.children},key);});return/*#__PURE__*/(0,jsx_runtime.jsxs)("div",{...lineProps,children:[showLineNumbers?/*#__PURE__*/(0,jsx_runtime.jsxs)(jsx_runtime.Fragment,{children:[/*#__PURE__*/(0,jsx_runtime.jsx)("span",{className:Line_styles_module.codeLineNumber}),/*#__PURE__*/(0,jsx_runtime.jsx)("span",{className:Line_styles_module.codeLineContent,children:lineTokens})]}):lineTokens,/*#__PURE__*/(0,jsx_runtime.jsx)(LineBreak,{})]});}
;// ./src/theme/CodeBlock/Content/index.tsx
// TODO Docusaurus v4: remove useless forwardRef
const Pre=/*#__PURE__*/react.forwardRef((props,ref)=>{return/*#__PURE__*/(0,jsx_runtime.jsx)("pre",{ref:ref/* eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex */,tabIndex:0,...props,className:(0,clsx/* default */.A)(props.className,styles_module.codeBlock,'thin-scrollbar')});});function Code(props){const{metadata}=(0,codeBlockUtils/* useCodeBlockContext */.Ph)();return/*#__PURE__*/(0,jsx_runtime.jsx)("code",{...props,className:(0,clsx/* default */.A)(props.className,styles_module.codeBlockLines,metadata.lineNumbersStart!==undefined&&styles_module.codeBlockLinesWithNumbering),style:{...props.style,counterReset:metadata.lineNumbersStart===undefined?undefined:`line-count ${metadata.lineNumbersStart-1}`}});}function CodeBlockContent({className:classNameProp}){const{metadata,wordWrap}=(0,codeBlockUtils/* useCodeBlockContext */.Ph)();const prismTheme=(0,usePrismTheme/* usePrismTheme */.A)();const{code,language,lineNumbersStart,lineClassNames}=metadata;return/*#__PURE__*/(0,jsx_runtime.jsx)(dist/* Highlight */.f4,{theme:prismTheme,code:code,language:language,children:({className,style,tokens:lines,getLineProps,getTokenProps})=>/*#__PURE__*/(0,jsx_runtime.jsx)(Pre,{ref:wordWrap.codeBlockRef,className:(0,clsx/* default */.A)(classNameProp,className),style:style,children:/*#__PURE__*/(0,jsx_runtime.jsx)(Code,{children:lines.map((line,i)=>/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockLine,{line:line,getLineProps:getLineProps,getTokenProps:getTokenProps,classNames:lineClassNames[i],showLineNumbers:lineNumbersStart!==undefined},i))})})});}
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+core@3.10.1_@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6__clean-cs_c6bb8a92892675e531eb6c2d31c3baf0/node_modules/@docusaurus/core/lib/client/exports/BrowserOnly.js
var BrowserOnly = __webpack_require__(8120);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+core@3.10.1_@mdx-js+react@3.1.1_@types+react@19.2.14_react@19.2.6__clean-cs_c6bb8a92892675e531eb6c2d31c3baf0/node_modules/@docusaurus/core/lib/client/exports/Translate.js + 1 modules
var Translate = __webpack_require__(4341);
;// ./src/theme/CodeBlock/Buttons/Button/index.tsx
function CodeBlockButton({className,...props}){return/*#__PURE__*/(0,jsx_runtime.jsx)("button",{type:"button",...props,className:(0,clsx/* default */.A)('clean-btn inline-flex h-9 w-9 items-center justify-center rounded-full border border-white/10 bg-slate-900/80 text-slate-300 backdrop-blur transition hover:border-blue-400/50 hover:bg-slate-800 hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-400',className)});}
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-classic@3.10.1_@types+react@19.2.14_clean-css@5.3.3_cssnano@6.1.2_pos_396db09c651a0e29ce8d5e6e3f45d168/node_modules/@docusaurus/theme-classic/lib/theme/Icon/Copy/index.js
var Copy = __webpack_require__(418);
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-classic@3.10.1_@types+react@19.2.14_clean-css@5.3.3_cssnano@6.1.2_pos_396db09c651a0e29ce8d5e6e3f45d168/node_modules/@docusaurus/theme-classic/lib/theme/Icon/Success/index.js
var Success = __webpack_require__(3550);
;// ./src/theme/CodeBlock/Buttons/CopyButton/styles.module.css
// extracted by mini-css-extract-plugin
/* harmony default export */ const CopyButton_styles_module = ({"copyButtonCopied":"copyButtonCopied_KAEU","copyButtonIcons":"copyButtonIcons_CCmD","copyButtonIcon":"copyButtonIcon_cWY_","copyButtonSuccessIcon":"copyButtonSuccessIcon_YaqN"});
;// ./src/theme/CodeBlock/Buttons/CopyButton/index.tsx
function title(){return (0,Translate/* translate */.T)({id:'theme.CodeBlock.copy',message:'Copy',description:'The copy button label on code blocks'});}function ariaLabel(isCopied){return isCopied?(0,Translate/* translate */.T)({id:'theme.CodeBlock.copied',message:'Copied',description:'The copied button label on code blocks'}):(0,Translate/* translate */.T)({id:'theme.CodeBlock.copyButtonAriaLabel',message:'Copy code to clipboard',description:'The ARIA label for copy code blocks button'});}async function copyToClipboard(text){// The clipboard API is only defined in secure contexts (HTTPS / localhost).
// See https://developer.mozilla.org/en-US/docs/Web/API/Clipboard
if(navigator.clipboard){return navigator.clipboard.writeText(text);}// Fall back to copy-text-to-clipboard for non-secure contexts (e.g. HTTP
// on a local network). The fallback is lazily loaded to avoid bundle
// overhead for the common HTTPS case.
const{default:copy}=await __webpack_require__.e(/* import() */ 821).then(__webpack_require__.bind(__webpack_require__, 821));return copy(text);}function useCopyButton(){const{metadata:{code}}=(0,codeBlockUtils/* useCodeBlockContext */.Ph)();const[isCopied,setIsCopied]=(0,react.useState)(false);const copyTimeout=(0,react.useRef)(undefined);const copyCode=(0,react.useCallback)(()=>{copyToClipboard(code).then(()=>{setIsCopied(true);copyTimeout.current=window.setTimeout(()=>{setIsCopied(false);},1000);});// Errors are intentionally not caught so they remain unhandled and can
// be captured by observability tools (e.g. Sentry, PostHog).
},[code]);(0,react.useEffect)(()=>()=>window.clearTimeout(copyTimeout.current),[]);return{copyCode,isCopied};}function CopyButton({className}){const{copyCode,isCopied}=useCopyButton();return/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockButton,{"aria-label":ariaLabel(isCopied),title:title(),className:(0,clsx/* default */.A)(className,CopyButton_styles_module.copyButton,isCopied&&CopyButton_styles_module.copyButtonCopied),onClick:copyCode,children:/*#__PURE__*/(0,jsx_runtime.jsxs)("span",{className:CopyButton_styles_module.copyButtonIcons,"aria-hidden":"true",children:[/*#__PURE__*/(0,jsx_runtime.jsx)(Copy/* default */.A,{className:CopyButton_styles_module.copyButtonIcon}),/*#__PURE__*/(0,jsx_runtime.jsx)(Success/* default */.A,{className:CopyButton_styles_module.copyButtonSuccessIcon})]})});}
// EXTERNAL MODULE: ./node_modules/.pnpm/@docusaurus+theme-classic@3.10.1_@types+react@19.2.14_clean-css@5.3.3_cssnano@6.1.2_pos_396db09c651a0e29ce8d5e6e3f45d168/node_modules/@docusaurus/theme-classic/lib/theme/Icon/WordWrap/index.js
var WordWrap = __webpack_require__(9879);
;// ./src/theme/CodeBlock/Buttons/WordWrapButton/styles.module.css
// extracted by mini-css-extract-plugin
/* harmony default export */ const WordWrapButton_styles_module = ({"wordWrapButtonIcon":"wordWrapButtonIcon_zAao","wordWrapButtonEnabled":"wordWrapButtonEnabled_RxZc"});
;// ./src/theme/CodeBlock/Buttons/WordWrapButton/index.tsx
function WordWrapButton({className}){const{wordWrap}=(0,codeBlockUtils/* useCodeBlockContext */.Ph)();const canShowButton=wordWrap.isEnabled||wordWrap.isCodeScrollable;if(!canShowButton){return false;}const title=(0,Translate/* translate */.T)({id:'theme.CodeBlock.wordWrapToggle',message:'Toggle word wrap',description:'The title attribute for toggle word wrapping button of code block lines'});return/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockButton,{onClick:()=>wordWrap.toggle(),className:(0,clsx/* default */.A)(className,wordWrap.isEnabled&&WordWrapButton_styles_module.wordWrapButtonEnabled),"aria-label":title,title:title,children:/*#__PURE__*/(0,jsx_runtime.jsx)(WordWrap/* default */.A,{className:WordWrapButton_styles_module.wordWrapButtonIcon,"aria-hidden":"true"})});}
;// ./src/theme/CodeBlock/Buttons/index.tsx
function CodeBlockButtons({className}){return/*#__PURE__*/(0,jsx_runtime.jsx)(BrowserOnly/* default */.A,{children:()=>/*#__PURE__*/(0,jsx_runtime.jsxs)("div",{className:(0,clsx/* default */.A)(className,'absolute right-3 top-3 z-10 flex items-center gap-2'),children:[/*#__PURE__*/(0,jsx_runtime.jsx)(WordWrapButton,{}),/*#__PURE__*/(0,jsx_runtime.jsx)(CopyButton,{})]})});}
;// ./src/theme/CodeBlock/Layout/index.tsx
function CodeBlockLayout({className}){const{metadata}=(0,codeBlockUtils/* useCodeBlockContext */.Ph)();return/*#__PURE__*/(0,jsx_runtime.jsxs)(CodeBlockContainer,{as:"div",className:(0,clsx/* default */.A)(className,metadata.className),children:[metadata.title&&/*#__PURE__*/(0,jsx_runtime.jsx)("div",{className:"border-b border-white/10 bg-slate-900/90 px-5 py-3 text-xs font-semibold uppercase tracking-[0.18em] text-slate-300",children:/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockTitle,{children:metadata.title})}),/*#__PURE__*/(0,jsx_runtime.jsxs)("div",{className:"relative rounded-[inherit]",children:[/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockContent,{}),/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockButtons,{})]})]});}
;// ./src/theme/CodeBlock/Content/String.tsx
function useCodeBlockMetadata(props){const{prism}=(0,useThemeConfig/* useThemeConfig */.p)();return (0,codeBlockUtils/* createCodeBlockMetadata */.mU)({code:props.children,className:props.className,metastring:props.metastring,magicComments:prism.magicComments,defaultLanguage:prism.defaultLanguage,language:props.language,title:props.title,showLineNumbers:props.showLineNumbers});}// TODO Docusaurus v4: move this component at the root?
function CodeBlockString(props){const metadata=useCodeBlockMetadata(props);const wordWrap=(0,useCodeWordWrap/* useCodeWordWrap */.f)();return/*#__PURE__*/(0,jsx_runtime.jsx)(codeBlockUtils/* CodeBlockContextProvider */.l8,{metadata:metadata,wordWrap:wordWrap,children:/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockLayout,{})});}
;// ./src/theme/CodeBlock/index.tsx
/**
 * Best attempt to make the children a plain string so it is copyable. If there
 * are react elements, we will not be able to copy the content, and it will
 * return `children` as-is; otherwise, it concatenates the string children
 * together.
 */function maybeStringifyChildren(children){if(react.Children.toArray(children).some(el=>/*#__PURE__*/(0,react.isValidElement)(el))){return children;}// The children is now guaranteed to be one/more plain strings
return Array.isArray(children)?children.join(''):children;}function CodeBlock({children:rawChildren,...props}){// The Prism theme on SSR is always the default theme but the site theme can
// be in a different mode. React hydration doesn't update DOM styles that come
// from SSR. Hence force a re-render after mounting to apply the current
// relevant styles.
const isBrowser=(0,useIsBrowser/* default */.A)();const children=maybeStringifyChildren(rawChildren);const CodeBlockComp=typeof children==='string'?CodeBlockString:CodeBlockJSX;return/*#__PURE__*/(0,jsx_runtime.jsx)(CodeBlockComp,{...props,children:children},String(isBrowser));}

/***/ },

/***/ 7060
(__unused_webpack_module, __webpack_exports__, __webpack_require__) {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   A: () => (/* binding */ MDXDetails)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(489);
/* harmony import */ var clsx__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(3526);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(1325);
function MDXDetails(props){const items=react__WEBPACK_IMPORTED_MODULE_0__.Children.toArray(props.children);const summary=items.find(item=>/*#__PURE__*/react__WEBPACK_IMPORTED_MODULE_0__.isValidElement(item)&&item.type==='summary');const children=/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_2__.jsx)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_2__.Fragment,{children:items.filter(item=>item!==summary)});const summaryChildren=summary?.props.children??'Details';return/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_2__.jsxs)("details",{...props,className:(0,clsx__WEBPACK_IMPORTED_MODULE_1__/* ["default"] */ .A)('group my-6 overflow-hidden rounded-[1.5rem] border border-slate-200/80 bg-white/80 shadow-panel backdrop-blur dark:border-slate-800 dark:bg-slate-950/60',props.className),children:[/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_2__.jsx)("summary",{className:"cursor-pointer list-none px-5 py-4 text-sm font-semibold text-slate-900 transition marker:hidden hover:bg-slate-50 dark:text-slate-100 dark:hover:bg-slate-900/80",children:/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_2__.jsxs)("span",{className:"flex items-center justify-between gap-4",children:[/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_2__.jsx)("span",{children:summaryChildren}),/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_2__.jsx)("span",{className:"text-xs uppercase tracking-[0.18em] text-blue-700 transition group-open:rotate-180 dark:text-blue-300",children:"v"})]})}),/*#__PURE__*/(0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_2__.jsx)("div",{className:"border-t border-slate-200/80 px-5 py-5 text-sm leading-7 text-slate-600 dark:border-slate-800 dark:text-slate-300",children:children})]});}

/***/ }

}]);