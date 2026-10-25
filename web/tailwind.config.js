// The neutral scale is driven by CSS variables (src/assets/styles/index.css), so
// every `*-neutral-N` class follows the Appearance setting without a per-class
// `dark:` variant: the light theme mirrors the scale (50 <-> 950, 100 <-> 900 ...).
const neutral = Object.fromEntries(
  [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950].map((n) => [
    n,
    `rgb(var(--neutral-${n}) / <alpha-value>)`
  ])
);

/** @type {import('tailwindcss').Config} */
export default {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: { neutral }
    }
  },
  plugins: [],
  corePlugins: {
    preflight: false
  }
};
