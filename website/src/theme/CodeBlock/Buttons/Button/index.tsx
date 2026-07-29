import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import type { Props } from '@theme/CodeBlock/Buttons/Button';

export default function CodeBlockButton({ className, ...props }: Props): ReactNode {
  return (
    <button
      type="button"
      {...props}
      className={clsx(
        'clean-btn inline-flex h-9 w-9 items-center justify-center rounded-full border border-white/10 bg-slate-900/80 text-slate-300 backdrop-blur transition hover:border-blue-400/50 hover:bg-slate-800 hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-400',
        className,
      )}
    />
  );
}
