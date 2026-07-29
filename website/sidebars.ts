import type { SidebarsConfig } from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    {
      type: 'category',
      label: 'Getting Started',
      items: ['intro', 'quickstart'],
    },
    {
      type: 'category',
      label: 'Guides',
      items: ['configuration', 'config-examples', 'providers-and-routing', 'request-examples'],
    },
    {
      type: 'category',
      label: 'Reference',
      items: ['api-reference', 'operations', 'deployment'],
    },
  ],
};

export default sidebars;
