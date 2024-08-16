/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./**/*.html", "./**/*.templ", "./**/*.go"],
  theme: {
    extend: {},
  },
  plugins: [],
  safelist: [
    {
      pattern: /text-.*-800/,
    },
    {
      pattern: /bg-.*-100/,
    },
  ],
};
