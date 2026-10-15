import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import BrowserOnly from '@docusaurus/BrowserOnly';

import CopyButton from '@theme/CodeBlock/Buttons/CopyButton';
import WordWrapButton from '@theme/CodeBlock/Buttons/WordWrapButton';
import type { Props } from '@theme/CodeBlock/Buttons';

export default function CodeBlockButtons({ className }: Props): ReactNode {
  return (
    <BrowserOnly>
      {() => (
        <div className={clsx(className, 'absolute right-3 top-3 z-10 flex items-center gap-2')}>
          <WordWrapButton />

          <CopyButton />
        </div>
      )}
    </BrowserOnly>
  );
}
