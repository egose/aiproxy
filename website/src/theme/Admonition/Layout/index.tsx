import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import { ThemeClassNames } from '@docusaurus/theme-common';

import type { Props } from '@theme/Admonition/Layout';

const toneClassNames: Record<string, string> = {
  note: 'border-slate-200 bg-slate-50/90 dark:border-slate-700 dark:bg-slate-900/80',
  info: 'border-blue-200 bg-blue-50/80 dark:border-blue-900 dark:bg-blue-950/30',
  tip: 'border-emerald-200 bg-emerald-50/80 dark:border-emerald-900 dark:bg-emerald-950/30',
  warning: 'border-amber-200 bg-amber-50/80 dark:border-amber-900 dark:bg-amber-950/30',
  danger: 'border-rose-200 bg-rose-50/80 dark:border-rose-900 dark:bg-rose-950/30',
  caution: 'border-orange-200 bg-orange-50/80 dark:border-orange-900 dark:bg-orange-950/30',
};

const iconToneClassNames: Record<string, string> = {
  note: 'text-slate-700 dark:text-slate-200',
  info: 'text-blue-700 dark:text-blue-200',
  tip: 'text-emerald-700 dark:text-emerald-200',
  warning: 'text-amber-700 dark:text-amber-200',
  danger: 'text-rose-700 dark:text-rose-200',
  caution: 'text-orange-700 dark:text-orange-200',
};

function AdmonitionContainer({
  type,
  className,
  children,
  id,
}: Pick<Props, 'type' | 'className' | 'id'> & { children: ReactNode }) {
  return (
    <div
      className={clsx(
        ThemeClassNames.common.admonition,
        ThemeClassNames.common.admonitionType(type),
        'my-6 overflow-hidden rounded-[1.5rem] border shadow-panel backdrop-blur',
        toneClassNames[type] ?? toneClassNames.note,
        className,
      )}
      id={id}
    >
      {children}
    </div>
  );
}

function AdmonitionHeading({ type, icon, title }: Pick<Props, 'type' | 'icon' | 'title'>) {
  return (
    <div className="flex items-center gap-3 border-b border-black/5 px-5 py-4 text-sm font-semibold uppercase tracking-[0.16em] text-slate-900 dark:border-white/10 dark:text-slate-50">
      <span
        className={clsx(
          'inline-flex h-9 w-9 items-center justify-center rounded-full border border-black/5 bg-white/70 dark:border-white/10 dark:bg-white/5',
          iconToneClassNames[type] ?? iconToneClassNames.note,
        )}
      >
        {icon}
      </span>
      {title}
    </div>
  );
}

function AdmonitionContent({ children }: Pick<Props, 'children'>) {
  return children ? (
    <div className="px-5 py-5 text-sm leading-7 text-slate-700 dark:text-slate-300 [&>*:last-child]:mb-0">
      {children}
    </div>
  ) : null;
}

export default function AdmonitionLayout(props: Props): ReactNode {
  const { type, icon, title, children, className, id } = props;
  return (
    <AdmonitionContainer type={type} className={className} id={id}>
      {title || icon ? <AdmonitionHeading type={type} title={title} icon={icon} /> : null}
      <AdmonitionContent>{children}</AdmonitionContent>
    </AdmonitionContainer>
  );
}
