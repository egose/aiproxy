import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';

const highlights = [
  {
    title: 'One API Surface',
    body: 'Expose OpenAI-compatible endpoints while mixing OpenAI, Anthropic, Gemini, and OpenAI-compatible backends.',
  },
  {
    title: 'Proxy-Owned Routing',
    body: 'Publish direct models and alias-backed virtual models without coupling clients to a single upstream provider.',
  },
  {
    title: 'Built For Operations',
    body: 'Validate config, reload routing state on SIGHUP, export Prometheus metrics, and keep credentials out of logs.',
  },
];

const quickLinks = [
  { href: '/docs/configuration', label: 'Configuration' },
  { href: '/docs/config-examples', label: 'Config Examples' },
  { href: '/docs/providers-and-routing', label: 'Providers and Routing' },
  { href: '/docs/request-examples', label: 'Request Examples' },
  { href: '/docs/api-reference', label: 'API Reference' },
  { href: '/docs/deployment', label: 'Deployment' },
];

const operatingPoints = ['alias routing', 'provider translation', 'streaming SSE', 'usage metrics'];

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();

  return (
    <header className="px-4 pb-8 pt-12 sm:px-6 lg:px-8 lg:pb-12 lg:pt-16">
      <div className="mx-auto grid max-w-6xl gap-6 lg:grid-cols-[minmax(0,1.15fr)_minmax(300px,0.85fr)] lg:items-stretch">
        <div className="overflow-hidden rounded-[2rem] border border-slate-200/80 bg-white/85 p-7 shadow-panel backdrop-blur dark:border-slate-800 dark:bg-slate-950/75 sm:p-10">
          <p className="mb-4 text-xs font-semibold uppercase tracking-[0.22em] text-blue-700 dark:text-blue-300">
            Documentation
          </p>
          <Heading
            as="h1"
            className="m-0 max-w-3xl text-5xl font-black tracking-tight text-slate-950 dark:text-slate-50 sm:text-6xl"
          >
            {siteConfig.title}
          </Heading>
          <p className="mt-4 max-w-2xl text-lg text-slate-700 dark:text-slate-300 sm:text-xl">{siteConfig.tagline}</p>
          <p className="mt-6 max-w-2xl text-base leading-8 text-slate-600 dark:text-slate-400">
            Route multiple AI providers through one service, keep the public API stable, and let the proxy own model
            naming, authentication, alias routing, and failover behavior.
          </p>
          <div className="mt-8 flex flex-wrap gap-3">
            <Link
              className="inline-flex items-center justify-center rounded-full bg-blue-600 px-5 py-3 text-sm font-semibold text-white no-underline transition hover:bg-blue-500 hover:text-white"
              to="/docs/quickstart"
            >
              Quickstart
            </Link>
            <Link
              className="inline-flex items-center justify-center rounded-full border border-slate-300 bg-slate-50 px-5 py-3 text-sm font-semibold text-slate-900 no-underline transition hover:border-slate-400 hover:bg-slate-100 hover:text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:hover:border-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-100"
              to="/docs/intro"
            >
              Browse Docs
            </Link>
          </div>
        </div>

        <div className="rounded-[2rem] border border-slate-200/80 bg-slate-950 p-6 text-slate-50 shadow-panel dark:border-slate-800 sm:p-8">
          <div className="rounded-2xl border border-white/10 bg-white/5 p-5">
            <p className="mb-3 text-xs font-semibold uppercase tracking-[0.2em] text-blue-200">Request Flow</p>
            <div className="space-y-3 font-mono text-sm text-slate-200">
              <div className="rounded-xl border border-white/10 bg-white/5 px-4 py-3">client request</div>
              <div className="pl-2 text-blue-200">authenticate and resolve model</div>
              <div className="rounded-xl border border-white/10 bg-white/5 px-4 py-3">
                select direct model or alias target
              </div>
              <div className="pl-2 text-blue-200">translate for provider-native upstreams</div>
              <div className="rounded-xl border border-white/10 bg-white/5 px-4 py-3">
                normalize response for clients
              </div>
            </div>
          </div>
          <div className="mt-5 grid grid-cols-2 gap-3">
            {operatingPoints.map((item) => (
              <div
                key={item}
                className="rounded-2xl border border-white/10 bg-white/5 px-4 py-4 text-sm font-medium text-slate-100"
              >
                {item}
              </div>
            ))}
          </div>
        </div>
      </div>
    </header>
  );
}

export default function Home() {
  const { siteConfig } = useDocusaurusContext();

  return (
    <Layout
      title={siteConfig.title}
      description="Documentation for aiproxy, an OpenAI-compatible proxy for multiple AI providers."
    >
      <HomepageHeader />
      <main className="px-4 pb-16 sm:px-6 lg:px-8">
        <section className="mx-auto max-w-6xl">
          <div className="grid gap-4 lg:grid-cols-3">
            {highlights.map((highlight) => (
              <article
                key={highlight.title}
                className="rounded-[1.5rem] border border-slate-200/80 bg-white/85 p-6 shadow-panel backdrop-blur dark:border-slate-800 dark:bg-slate-950/70"
              >
                <Heading as="h2" className="mb-3 text-xl font-bold text-slate-950 dark:text-slate-50">
                  {highlight.title}
                </Heading>
                <p className="m-0 leading-7 text-slate-600 dark:text-slate-400">{highlight.body}</p>
              </article>
            ))}
          </div>
        </section>

        <section className="mx-auto mt-5 max-w-6xl">
          <div className="overflow-hidden rounded-[2rem] border border-slate-200/80 bg-white/85 p-6 shadow-panel backdrop-blur dark:border-slate-800 dark:bg-slate-950/70 sm:p-8">
            <div className="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
              <div className="max-w-2xl">
                <Heading as="h2" className="mb-3 text-2xl font-bold text-slate-950 dark:text-slate-50">
                  Start with the essentials
                </Heading>
                <p className="m-0 leading-7 text-slate-600 dark:text-slate-400">
                  The docs are organized around how the proxy actually behaves: configuration first, then providers,
                  routing, request behavior, and deployment.
                </p>
              </div>
              <div className="rounded-2xl border border-blue-200 bg-blue-50 px-4 py-3 text-sm font-medium text-blue-900 dark:border-blue-900/60 dark:bg-blue-950/40 dark:text-blue-100">
                Built for OpenAI, Anthropic, Gemini, and OpenAI-compatible backends
              </div>
            </div>
            <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
              {quickLinks.map((item) => (
                <Link
                  key={item.href}
                  className="group rounded-[1.25rem] border border-slate-200 bg-slate-50 px-5 py-4 text-sm font-semibold text-slate-900 no-underline transition hover:border-blue-300 hover:bg-blue-50 hover:text-slate-950 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-100 dark:hover:border-blue-900 dark:hover:bg-slate-900 dark:hover:text-white"
                  to={item.href}
                >
                  <span className="flex items-center justify-between gap-4">
                    <span>{item.label}</span>
                    <span className="text-slate-400 transition group-hover:text-blue-600 dark:text-slate-500 dark:group-hover:text-blue-300">
                      view
                    </span>
                  </span>
                </Link>
              ))}
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}
