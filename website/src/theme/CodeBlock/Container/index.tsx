import React, { type ComponentProps, type ReactNode } from 'react';
import clsx from 'clsx';
import { ThemeClassNames, usePrismTheme } from '@docusaurus/theme-common';
import { getPrismCssVariables } from '@docusaurus/theme-common/internal';

export default function CodeBlockContainer<T extends 'div' | 'pre'>({
  as: As,
  ...props
}: { as: T } & ComponentProps<T>): ReactNode {
  const prismTheme = usePrismTheme();
  const prismCssVariables = getPrismCssVariables(prismTheme);
  return (
    <As
      {...(props as any)}
      style={prismCssVariables}
      className={clsx(
        props.className,
        'mb-6 overflow-hidden rounded-[1.5rem] border border-slate-200 bg-slate-950 text-slate-100 shadow-panel dark:border-slate-800',
        ThemeClassNames.common.codeBlock,
      )}
    />
  );
}
