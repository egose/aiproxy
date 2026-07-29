import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import useIsBrowser from '@docusaurus/useIsBrowser';
import { translate } from '@docusaurus/Translate';
import IconLightMode from '@theme/Icon/LightMode';
import IconDarkMode from '@theme/Icon/DarkMode';
import IconSystemColorMode from '@theme/Icon/SystemColorMode';
import type { Props } from '@theme/ColorModeToggle';
import type { ColorMode } from '@docusaurus/theme-common';

import styles from './styles.module.css';

function getNextColorMode(colorMode: ColorMode | null, respectPrefersColorScheme: boolean) {
  if (!respectPrefersColorScheme) {
    return colorMode === 'dark' ? 'light' : 'dark';
  }

  switch (colorMode) {
    case null:
      return 'light';
    case 'light':
      return 'dark';
    case 'dark':
      return null;
    default:
      throw new Error(`unexpected color mode ${colorMode}`);
  }
}

function getColorModeLabel(colorMode: ColorMode | null): string {
  switch (colorMode) {
    case null:
      return translate({
        message: 'system mode',
        id: 'theme.colorToggle.ariaLabel.mode.system',
        description: 'The name for the system color mode',
      });
    case 'light':
      return translate({
        message: 'light mode',
        id: 'theme.colorToggle.ariaLabel.mode.light',
        description: 'The name for the light color mode',
      });
    case 'dark':
      return translate({
        message: 'dark mode',
        id: 'theme.colorToggle.ariaLabel.mode.dark',
        description: 'The name for the dark color mode',
      });
    default:
      throw new Error(`unexpected color mode ${colorMode}`);
  }
}

function getColorModeAriaLabel(colorMode: ColorMode | null) {
  return translate(
    {
      message: 'Switch between dark and light mode (currently {mode})',
      id: 'theme.colorToggle.ariaLabel',
      description: 'The ARIA label for the color mode toggle',
    },
    {
      mode: getColorModeLabel(colorMode),
    },
  );
}

function CurrentColorModeIcon(): ReactNode {
  return (
    <>
      <IconLightMode aria-hidden className={clsx(styles.toggleIcon, styles.lightToggleIcon)} />
      <IconDarkMode aria-hidden className={clsx(styles.toggleIcon, styles.darkToggleIcon)} />
      <IconSystemColorMode aria-hidden className={clsx(styles.toggleIcon, styles.systemToggleIcon)} />
    </>
  );
}

function ColorModeToggle({ className, buttonClassName, respectPrefersColorScheme, value, onChange }: Props): ReactNode {
  const isBrowser = useIsBrowser();
  return (
    <div className={clsx('h-10 w-10', className)}>
      <button
        className={clsx(
          'clean-btn',
          'flex h-full w-full items-center justify-center rounded-full border border-slate-200 bg-white/80 text-slate-700 transition hover:border-blue-300 hover:bg-blue-50 hover:text-blue-700 dark:border-slate-700 dark:bg-slate-900/80 dark:text-slate-200 dark:hover:border-blue-900 dark:hover:bg-blue-950/30 dark:hover:text-blue-200',
          !isBrowser && styles.toggleButtonDisabled,
          buttonClassName,
        )}
        type="button"
        onClick={() => onChange(getNextColorMode(value, respectPrefersColorScheme))}
        disabled={!isBrowser}
        title={getColorModeLabel(value)}
        aria-label={getColorModeAriaLabel(value)}
      >
        <CurrentColorModeIcon />
      </button>
    </div>
  );
}

export default React.memo(ColorModeToggle);
