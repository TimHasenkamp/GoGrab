/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/routes/admin/**/*.{svelte,ts}', './src/lib/**/*.{svelte,ts}', './src/app.html'],
  theme: {
    extend: {
      colors: {
        // Map CSS vars → semantic Tailwind colors. Use as `bg-background`,
        // `text-foreground`, `border-border`, `bg-accent`, etc.
        background: 'var(--background)',
        foreground: 'var(--foreground)',
        card: 'var(--card)',
        muted: {
          DEFAULT: 'var(--muted)',
          foreground: 'var(--muted-foreground)'
        },
        border: {
          DEFAULT: 'var(--border)',
          strong: 'var(--border-strong)'
        },
        accent: {
          DEFAULT: 'var(--accent)',
          hover: 'var(--accent-hover)',
          glow: 'var(--accent-glow)'
        },
        primary: 'var(--primary)',
        success: 'var(--success)',
        warning: 'var(--warning)',
        danger: 'var(--danger)'
      },
      fontFamily: {
        sans: ['Geist', 'ui-sans-serif', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['Geist Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace']
      },
      boxShadow: {
        'accent-glow': '0 0 0 1px var(--accent-glow), 0 8px 28px -8px var(--accent-glow)'
      }
    }
  },
  plugins: []
};
