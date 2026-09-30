/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./app/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        background: "#08090d",
        surface: "#0e1017",
        "surface-hover": "#141724",
        "surface-active": "#1a1e2e",
        border: "rgba(255, 255, 255, 0.08)",
        "border-subtle": "rgba(255, 255, 255, 0.05)",
        accent: "#06b6d4",
        "accent-muted": "rgba(6, 182, 212, 0.15)",
        "accent-violet": "#8b5cf6",
        "accent-amber": "#f59e0b",
        "accent-red": "#ef4444",
      },
      fontFamily: {
        mono: [
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Monaco",
          "Consolas",
          "Liberation Mono",
          "Courier New",
          "monospace",
        ],
      },
    },
  },
  plugins: [],
};
