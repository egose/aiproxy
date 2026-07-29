/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    './src/**/*.{js,jsx,ts,tsx}',
    './docs/**/*.{md,mdx}',
    './blog/**/*.{md,mdx}',
    './pages/**/*.{js,jsx,ts,tsx,md,mdx}',
    './docusaurus.config.{js,ts}',
  ],
  theme: {
    extend: {
      colors: {
        steel: {
          950: '#09111f',
        },
      },
      boxShadow: {
        panel: '0 20px 60px rgba(15, 23, 42, 0.12)',
      },
      typography: {
        DEFAULT: {
          css: {
            maxWidth: '72ch',
            color: '#334155',
            h1: {
              color: '#0f172a',
              fontWeight: '800',
              letterSpacing: '-0.03em',
            },
            h2: {
              color: '#0f172a',
              fontWeight: '750',
              letterSpacing: '-0.025em',
            },
            h3: {
              color: '#0f172a',
              fontWeight: '700',
            },
            code: {
              color: '#0f172a',
              fontWeight: '600',
            },
            'code::before': {
              content: '"`"',
            },
            'code::after': {
              content: '"`"',
            },
            a: {
              color: '#1d4ed8',
              textDecoration: 'none',
              fontWeight: '600',
            },
            'a:hover': {
              color: '#2563eb',
            },
            strong: {
              color: '#0f172a',
            },
            blockquote: {
              color: '#334155',
              borderLeftColor: '#93c5fd',
            },
            thead: {
              color: '#0f172a',
            },
          },
        },
        invert: {
          css: {
            color: '#cbd5e1',
            h1: {
              color: '#f8fafc',
            },
            h2: {
              color: '#f8fafc',
            },
            h3: {
              color: '#f8fafc',
            },
            code: {
              color: '#f8fafc',
            },
            a: {
              color: '#93c5fd',
            },
            'a:hover': {
              color: '#bfdbfe',
            },
            strong: {
              color: '#f8fafc',
            },
            blockquote: {
              color: '#cbd5e1',
              borderLeftColor: '#3b82f6',
            },
            thead: {
              color: '#f8fafc',
            },
          },
        },
      },
    },
  },
  plugins: [require('@tailwindcss/typography')],
};
