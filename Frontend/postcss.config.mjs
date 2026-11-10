/**
 * Tailwind CSS v4 is configured through PostCSS only: there is no
 * tailwind.config file, theme tokens live in globals.css under @theme.
 */
const config = {
  plugins: {
    "@tailwindcss/postcss": {},
  },
};

export default config;
