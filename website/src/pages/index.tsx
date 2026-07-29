import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';

import styles from './index.module.css';

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

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();

  return (
    <header className={styles.heroBanner}>
      <div className={styles.heroInner}>
        <p className={styles.eyebrow}>Documentation</p>
        <Heading as="h1" className={styles.heroTitle}>
          {siteConfig.title}
        </Heading>
        <p className={styles.heroSubtitle}>{siteConfig.tagline}</p>
        <p className={styles.heroDescription}>
          Route multiple AI providers through one service, keep the public API consistent, and let the proxy own model
          naming, auth, and failover behavior.
        </p>
        <div className={styles.buttons}>
          <Link className="button button--primary button--lg" to="/docs/quickstart">
            Quickstart
          </Link>
          <Link className="button button--secondary button--lg" to="/docs/intro">
            Browse Docs
          </Link>
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
      <main className={styles.main}>
        <section className={styles.highlightsSection}>
          <div className={styles.highlightsGrid}>
            {highlights.map((highlight) => (
              <article key={highlight.title} className={styles.card}>
                <Heading as="h2" className={styles.cardTitle}>
                  {highlight.title}
                </Heading>
                <p className={styles.cardBody}>{highlight.body}</p>
              </article>
            ))}
          </div>
        </section>

        <section className={styles.quickLinksSection}>
          <div className={styles.quickLinksPanel}>
            <Heading as="h2" className={styles.quickLinksTitle}>
              Start with the essentials
            </Heading>
            <div className={styles.quickLinksRow}>
              <Link className={styles.quickLink} to="/docs/configuration">
                Configuration
              </Link>
              <Link className={styles.quickLink} to="/docs/config-examples">
                Config Examples
              </Link>
              <Link className={styles.quickLink} to="/docs/providers-and-routing">
                Providers and Routing
              </Link>
              <Link className={styles.quickLink} to="/docs/request-examples">
                Request Examples
              </Link>
              <Link className={styles.quickLink} to="/docs/api-reference">
                API Reference
              </Link>
              <Link className={styles.quickLink} to="/docs/deployment">
                Deployment
              </Link>
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}
