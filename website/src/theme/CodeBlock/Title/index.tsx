import type { ReactNode } from 'react';

import type { Props } from '@theme/CodeBlock/Title';

export default function CodeBlockTitle({ children }: Props): ReactNode {
  return <span className="inline-flex items-center gap-2 text-slate-200">{children}</span>;
}
