/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./app/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        background: "#0d0f17",
        surface: "#161926",
        border: "#262b40",
        accent: "#3b82f6",
      },
    },
  },
  plugins: [],
};
