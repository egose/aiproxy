import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import { ThemeClassNames } from '@docusaurus/theme-common';
import Heading from '@theme/DocCard/Heading';
import Description from '@theme/DocCard/Description';
import type { Props } from '@theme/DocCard/Layout';

function getKindLabel(item: Props['item']): string {
  return item.type === 'category' ? 'Category' : 'Doc';
}

function Container({
  className,
  href,
  children,
}: {
  className?: string;
  href: string;
  children: ReactNode;
}): ReactNode {
  return (
    <Link
      href={href}
      className={clsx(
        'group block h-full rounded-[1.75rem] border border-slate-200/80 bg-white/85 p-6 no-underline shadow-panel backdrop-blur transition hover:-translate-y-0.5 hover:border-blue-300 hover:bg-blue-50/70 dark:border-slate-800 dark:bg-slate-950/60 dark:hover:border-blue-900 dark:hover:bg-blue-950/20',
        ThemeClassNames.docs.docCard.container,
        className,
      )}
    >
      {children}
    </Link>
  );
}

export default function DocCardLayout({ item, className, href, icon, title, description }: Props): ReactNode {
  return (
    <Container href={href} className={className}>
      <div className="mb-4 inline-flex w-fit rounded-full border border-blue-200 bg-blue-50 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-blue-800 dark:border-blue-900/70 dark:bg-blue-950/30 dark:text-blue-200">
        {getKindLabel(item)}
      </div>

      <Heading item={item} icon={icon} title={title} />
      {description && <Description item={item} description={description} />}

      <div className="mt-5 inline-flex items-center gap-2 text-sm font-semibold text-slate-500 transition group-hover:text-blue-700 dark:text-slate-400 dark:group-hover:text-blue-300">
        Open page <span aria-hidden="true">&rarr;</span>
      </div>
    </Container>
  );
}
