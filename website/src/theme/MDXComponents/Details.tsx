import React, { type ComponentProps, type ReactElement, type ReactNode } from 'react';
import clsx from 'clsx';
import type { Props } from '@theme/MDXComponents/Details';

export default function MDXDetails(props: Props): ReactNode {
  const items = React.Children.toArray(props.children);
  const summary = items.find(
    (item): item is ReactElement<ComponentProps<'summary'>> => React.isValidElement(item) && item.type === 'summary',
  );
  const children = <>{items.filter((item) => item !== summary)}</>;
  const summaryChildren = summary?.props.children ?? 'Details';

  return (
    <details
      {...props}
      className={clsx(
        'group my-6 overflow-hidden rounded-[1.5rem] border border-slate-200/80 bg-white/80 shadow-panel backdrop-blur dark:border-slate-800 dark:bg-slate-950/60',
        props.className,
      )}
    >

      <summary className="cursor-pointer list-none px-5 py-4 text-sm font-semibold text-slate-900 transition marker:hidden hover:bg-slate-50 dark:text-slate-100 dark:hover:bg-slate-900/80">

        <span className="flex items-center justify-between gap-4">
                    <span>{summaryChildren}</span>

          <span className="text-xs uppercase tracking-[0.18em] text-blue-700 transition group-open:rotate-180 dark:text-blue-300">
                        v
          </span>

        </span>

      </summary>

      <div className="border-t border-slate-200/80 px-5 py-5 text-sm leading-7 text-slate-600 dark:border-slate-800 dark:text-slate-300">
                {children}

      </div>

    </details>
  );
}
