import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import { ThemeClassNames } from '@docusaurus/theme-common';
import type { Props } from '@theme/DocCard/Heading/Text';

export default function DocCardHeadingText({ title }: Props): ReactNode {
  return (
    <span className={clsx(ThemeClassNames.docs.docCard.title, 'min-w-0 flex-1 text-balance leading-7')}>{title}</span>
  );
}
