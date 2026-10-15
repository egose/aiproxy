import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import { ThemeClassNames } from '@docusaurus/theme-common';
import type { Props } from '@theme/DocCard/Description';

export default function DocCardDescription({ description }: Props): ReactNode {
  return (
    <p
      className={clsx(
        ThemeClassNames.docs.docCard.description,
        'm-0 text-sm leading-7 text-slate-600 dark:text-slate-400',
      )}
      title={description}
    >
            {description}

    </p>
  );
}
