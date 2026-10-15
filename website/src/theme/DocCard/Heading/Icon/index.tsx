import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import { ThemeClassNames } from '@docusaurus/theme-common';
import type { Props } from '@theme/DocCard/Heading/Icon';

export default function DocCardHeadingIcon({ icon }: Props): ReactNode {
  return (
    <span
      className={clsx(
        ThemeClassNames.docs.docCard.icon,
        'inline-flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl border border-slate-200 bg-slate-100 text-xl text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200',
      )}
    >
            {icon}

    </span>
  );
}
