import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import { ThemeClassNames } from '@docusaurus/theme-common';
import Heading from '@theme/Heading';
import Icon from '@theme/DocCard/Heading/Icon';
import Text from '@theme/DocCard/Heading/Text';
import type { Props } from '@theme/DocCard/Heading';

export default function DocCardHeading({ item, title, icon }: Props): ReactNode {
  return (
    <Heading
      as="h2"
      className={clsx(
        ThemeClassNames.docs.docCard.heading,
        'mb-3 flex items-start gap-4 text-xl font-bold tracking-tight text-slate-950 dark:text-slate-50',
      )}
      title={title}
    >
            {icon && <Icon item={item} icon={icon} />}

      <Text item={item} title={title} />

    </Heading>
  );
}
