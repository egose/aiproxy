import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import { useTabs } from '@docusaurus/theme-common/internal';
import type { Props } from '@theme/TabItem';

function TabItemPanel({ children, className, hidden }: { children: ReactNode; className?: string; hidden?: boolean }) {
  return (
    <div
      role="tabpanel"
      className={clsx(
        'rounded-[1.5rem] border border-slate-200/80 bg-white/85 p-6 shadow-panel backdrop-blur dark:border-slate-800 dark:bg-slate-950/60 [&>*:last-child]:mb-0',
        className,
      )}
      {...{ hidden }}
    >
            {children}

    </div>
  );
}

export default function TabItem({ children, className, value }: Props): ReactNode {
  const { selectedValue, lazy } = useTabs();
  const isSelected = value === selectedValue;

  if (!isSelected && lazy) {
    return null;
  }

  return (
    <TabItemPanel className={className} hidden={!isSelected}>
            {children}

    </TabItemPanel>
  );
}
