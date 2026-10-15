import React, { type ReactNode } from 'react';
import clsx from 'clsx';
import Translate from '@docusaurus/Translate';
import Link from '@docusaurus/Link';
import type { Props } from '@theme/NotFound/Content';
import Heading from '@theme/Heading';

export default function NotFoundContent({ className }: Props): ReactNode {
  return (
    <main className={clsx('mx-auto max-w-7xl px-4 py-20 sm:px-6 lg:px-8', className)}>

      <div className="mx-auto max-w-3xl rounded-[2rem] border border-slate-200/80 bg-white/85 p-8 text-center shadow-panel backdrop-blur dark:border-slate-800 dark:bg-slate-950/70 sm:p-12">

        <div className="mb-4 inline-flex rounded-full border border-blue-200 bg-blue-50 px-4 py-1.5 text-xs font-semibold uppercase tracking-[0.18em] text-blue-800 dark:border-blue-900 dark:bg-blue-950/30 dark:text-blue-200">
                    404
        </div>

        <Heading
          as="h1"
          className="mb-4 text-4xl font-black tracking-tight text-slate-950 dark:text-slate-50 sm:text-5xl"
        >

          <Translate id="theme.NotFound.title" description="The title of the 404 page">
                        Page Not Found
          </Translate>

        </Heading>

        <p className="mx-auto max-w-2xl text-base leading-8 text-slate-600 dark:text-slate-400">

          <Translate id="theme.NotFound.p1" description="The first paragraph of the 404 page">
                        We could not find what you were looking for.
          </Translate>

        </p>

        <p className="mx-auto mt-3 max-w-2xl text-base leading-8 text-slate-600 dark:text-slate-400">

          <Translate id="theme.NotFound.p2" description="The 2nd paragraph of the 404 page">
                        Please contact the owner of the site that linked you to the original URL and let them know their link is
                        broken.
          </Translate>

        </p>

        <div className="mt-8 flex flex-wrap items-center justify-center gap-3">

          <Link
            className="inline-flex items-center justify-center rounded-full bg-blue-600 px-5 py-3 text-sm font-semibold text-white no-underline transition hover:bg-blue-500 hover:text-white"
            to="/docs/intro"
          >
                        Read the docs
          </Link>

          <Link
            className="inline-flex items-center justify-center rounded-full border border-slate-300 bg-slate-50 px-5 py-3 text-sm font-semibold text-slate-900 no-underline transition hover:border-slate-400 hover:bg-slate-100 hover:text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:hover:border-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-100"
            to="/docs/quickstart"
          >
                        Open quickstart
          </Link>

        </div>

      </div>

    </main>
  );
}
