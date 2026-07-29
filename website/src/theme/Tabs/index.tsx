import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import { ThemeClassNames } from '@docusaurus/theme-common';
import {
  useScrollPositionBlocker,
  useTabsContextValue,
  useTabs,
  sanitizeTabsChildren,
  TabsProvider,
} from '@docusaurus/theme-common/internal';
import useIsBrowser from '@docusaurus/useIsBrowser';
import type { Props } from '@theme/Tabs';

function TabList({ className }: { className?: string }) {
  const { selectedValue, selectValue, tabValues, block } = useTabs();

  const tabRefs: (HTMLLIElement | null)[] = [];
  const { blockElementScrollPositionUntilNextRender } = useScrollPositionBlocker();

  const handleTabChange = (
    event: React.FocusEvent<HTMLLIElement> | React.MouseEvent<HTMLLIElement> | React.KeyboardEvent<HTMLLIElement>,
  ) => {
    const newTab = event.currentTarget;
    const newTabIndex = tabRefs.indexOf(newTab);
    const newTabValue = tabValues[newTabIndex]!.value;

    if (newTabValue !== selectedValue) {
      blockElementScrollPositionUntilNextRender(newTab);
      selectValue(newTabValue);
    }
  };

  const handleKeydown = (event: React.KeyboardEvent<HTMLLIElement>) => {
    let focusElement: HTMLLIElement | null = null;

    switch (event.key) {
      case 'Enter': {
        handleTabChange(event);
        break;
      }
      case 'ArrowRight': {
        const nextTab = tabRefs.indexOf(event.currentTarget) + 1;
        focusElement = tabRefs[nextTab] ?? tabRefs[0]!;
        break;
      }
      case 'ArrowLeft': {
        const prevTab = tabRefs.indexOf(event.currentTarget) - 1;
        focusElement = tabRefs[prevTab] ?? tabRefs[tabRefs.length - 1]!;
        break;
      }
      default:
        break;
    }

    focusElement?.focus();
  };

  return (
    <ul
      role="tablist"
      aria-orientation="horizontal"
      className={clsx(
        'tabs flex flex-wrap gap-2 rounded-[1.25rem] border border-slate-200/80 bg-white/80 p-2 shadow-panel backdrop-blur dark:border-slate-800 dark:bg-slate-950/60',
        {
          'tabs--block grid w-full grid-cols-1 sm:grid-cols-2': block,
        },
        className,
      )}
    >
      {tabValues.map(({ value, label, attributes }) => (
        <li
          role="tab"
          tabIndex={selectedValue === value ? 0 : -1}
          aria-selected={selectedValue === value}
          key={value}
          ref={(ref) => {
            tabRefs.push(ref);
          }}
          onKeyDown={handleKeydown}
          onClick={handleTabChange}
          {...attributes}
          className={clsx(
            'tabs__item m-0 rounded-xl border border-transparent px-4 py-2.5 text-sm font-semibold text-slate-600 transition hover:bg-slate-100 hover:text-slate-950 dark:text-slate-300 dark:hover:bg-slate-900 dark:hover:text-slate-50',
            attributes?.className as string,
            {
              'tabs__item--active border-blue-200 bg-blue-50 text-blue-700 shadow-sm dark:border-blue-900 dark:bg-blue-950/40 dark:text-blue-200':
                selectedValue === value,
            },
          )}
        >
          {label ?? value}
        </li>
      ))}
    </ul>
  );
}

function TabContent({ children }: { children: ReactNode }) {
  return <div className="mt-4">{children}</div>;
}

function TabsContainer({ className, children }: { className?: string; children: ReactNode }): ReactNode {
  return (
    <div className={clsx(ThemeClassNames.tabs.container, 'tabs-container', 'my-6')}>
      <TabList className={className} />
      <TabContent>{children}</TabContent>
    </div>
  );
}

export default function Tabs(props: Props): ReactNode {
  const isBrowser = useIsBrowser();
  const value = useTabsContextValue(props);
  return (
    <TabsProvider value={value} key={String(isBrowser)}>
      <TabsContainer className={props.className}>{sanitizeTabsChildren(props.children)}</TabsContainer>
    </TabsProvider>
  );
}
