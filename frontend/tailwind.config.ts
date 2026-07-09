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
        // ObserveID brand palette — copper accent on dark industrial
        brand: {
          50: "#fdf3ed",
          100: "#f9e4d4",
          200: "#f2c9a8",
          300: "#e9a874",
          400: "#D4854A",
          500: "#c0753f",
          600: "#a36335",
          700: "#864f2b",
          800: "#6e3f23",
          900: "#5a341c",
          950: "#331b0d",
        },
        surface: {
          DEFAULT: "#0C0E13",
          50: "#12151C",
          100: "#1a1d27",
          200: "#1E2230",
          300: "#262938",
          400: "#3b3e4d",
          500: "#555A68",
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
