import type { Config } from "tailwindcss"

const config: Config = {
  content: [
    "./src/pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
    "../../node_modules/@tremor/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        accent: {
          DEFAULT: "#3B82F6",
          400: "#60A5FA",
          500: "#3B82F6",
          600: "#2563EB",
          dim: "rgba(59, 130, 246, 0.12)",
        },
        surface: {
          DEFAULT: "#09090B",
          50: "#111116",
          100: "#1a1d27",
          200: "#1E2230",
          300: "#262938",
          400: "#3b3e4d",
          500: "#555A68",
        },
        border: {
          DEFAULT: "#1E1E24",
        },
        muted: {
          DEFAULT: "#52525B",
        },
        primary: {
          DEFAULT: "#FAFAFA",
        },
        secondary: {
          DEFAULT: "#A1A1AA",
        },
      },
      fontFamily: {
        sans: ["Plus Jakarta Sans", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "Fira Code", "monospace"],
      },
    },
  },
  plugins: [],
}

export default config
