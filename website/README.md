# Website

This directory contains the Docusaurus site for `aiproxy`.

## Styling Approach

The site uses TailwindCSS as the primary styling tool.

Use this order of preference when editing UI:

1. Tailwind utility classes directly in React components under `src/`
2. Tailwind `@apply` in `src/css/custom.css` for Docusaurus chrome and selector-based theming
3. Small swizzled theme components under `src/theme/` when Docusaurus markup needs structural customization

Avoid adding new CSS modules unless Tailwind cannot express the behavior cleanly. A few CSS modules remain in swizzled Docusaurus components where stateful or generated class behavior is still the smallest correct option.

## Key Files

- `tailwind.config.js`: Tailwind content scan and theme extensions
- `postcss.config.js`: Tailwind and Autoprefixer integration
- `src/css/custom.css`: global Tailwind layers plus Docusaurus shell styling
- `src/pages/index.tsx`: homepage layout
- `src/theme/`: targeted swizzles for Docusaurus components such as code blocks, doc cards, tabs, admonitions, and details

## Verification

Run these before considering website changes done:

```sh
pnpm typecheck
pnpm build
```

Use the local dev server when iterating on layout details:

```sh
pnpm start
```
