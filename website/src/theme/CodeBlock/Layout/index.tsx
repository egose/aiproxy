import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import { useCodeBlockContext } from '@docusaurus/theme-common/internal';
import Container from '@theme/CodeBlock/Container';
import Title from '@theme/CodeBlock/Title';
import Content from '@theme/CodeBlock/Content';
import type { Props } from '@theme/CodeBlock/Layout';
import Buttons from '@theme/CodeBlock/Buttons';

export default function CodeBlockLayout({ className }: Props): ReactNode {
  const { metadata } = useCodeBlockContext();
  return (
    <Container as="div" className={clsx(className, metadata.className)}>

      {metadata.title && (
        <div className="border-b border-white/10 bg-slate-900/90 px-5 py-3 text-xs font-semibold uppercase tracking-[0.18em] text-slate-300">
                    <Title>{metadata.title}</Title>

        </div>
      )}

      <div className="relative rounded-[inherit]">

        <Content />

        <Buttons />

      </div>

    </Container>
  );
}
