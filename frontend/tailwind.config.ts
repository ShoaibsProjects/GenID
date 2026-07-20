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
          DEFAULT: "#F59E0B",
          50: "#FFFBEB",
          100: "#FEF3C7",
          200: "#FDE68A",
          300: "#FCD34D",
          400: "#FBBF24",
          500: "#F59E0B",
          600: "#D97706",
          700: "#B45309",
          800: "#92400E",
          900: "#78350F",
          dim: "rgba(245, 158, 11, 0.12)",
          glow: "rgba(245, 158, 11, 0.25)",
        },
        surface: {
          DEFAULT: "#0A0A0C",
          50: "#121214",
          100: "#1A1A1F",
          200: "#1E1E24",
          300: "#26292E",
          400: "#3B3D45",
          500: "#555862",
        },
        border: {
          DEFAULT: "#1E1E24",
          light: "#2A2A32",
        },
        muted: {
          DEFAULT: "#52525B",
        },
        primary: {
          DEFAULT: "#F5F5F0",
        },
        secondary: {
          DEFAULT: "#A1A1AA",
        },
      },
      fontFamily: {
        sans: ["Plus Jakarta Sans", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "Fira Code", "monospace"],
      },
      animation: {
        "page-in": "fadeInUp 0.5s cubic-bezier(0.16, 1, 0.3, 1) both",
        "card-in": "fadeInUp 0.4s cubic-bezier(0.16, 1, 0.3, 1) both",
        "slide-down": "slideDown 0.2s cubic-bezier(0.16, 1, 0.3, 1) both",
        "shimmer": "shimmer 1.5s ease-in-out infinite",
        "glow": "glowPulse 3s ease-in-out infinite",
        "status": "statusPulse 2s ease-in-out infinite",
        "fade-in": "fadeIn 0.2s ease both",
        "count-up": "countUp 0.4s cubic-bezier(0.16, 1, 0.3, 1) both",
      },
      keyframes: {
        fadeInUp: {
          "from": { opacity: "0", transform: "translateY(16px)" },
          "to":   { opacity: "1", transform: "translateY(0)" },
        },
        fadeIn: {
          "from": { opacity: "0" },
          "to":   { opacity: "1" },
        },
        slideDown: {
          "from": { opacity: "0", transform: "translateY(-8px)" },
          "to":   { opacity: "1", transform: "translateY(0)" },
        },
        shimmer: {
          "0%":   { backgroundPosition: "-200% 0" },
          "100%": { backgroundPosition: "200% 0" },
        },
        glowPulse: {
          "0%, 100%": { boxShadow: "0 0 8px rgba(245, 158, 11, 0.08)" },
          "50%":      { boxShadow: "0 0 20px rgba(245, 158, 11, 0.2)" },
        },
        statusPulse: {
          "0%, 100%": { opacity: "1", transform: "scale(1)" },
          "50%":      { opacity: "0.7", transform: "scale(1.15)" },
        },
        countUp: {
          "from": { opacity: "0", transform: "translateY(8px)" },
          "to":   { opacity: "1", transform: "translateY(0)" },
        },
      },
    },
  },
  plugins: [],
}

export default config
