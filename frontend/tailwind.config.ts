import type { Config } from "tailwindcss"

const config: Config = {
  content: [
    "./src/pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        accent: {
          DEFAULT: "#F59E0B",
          400: "#FBBF24",
          500: "#F59E0B",
          600: "#D97706",
          dim: "rgba(245, 158, 11, 0.10)",
          glow: "rgba(245, 158, 11, 0.20)",
          strong: "rgba(245, 158, 11, 0.35)",
        },
        obsidian: {
          DEFAULT: "#050508",
          raised: "#0C0C10",
          elevated: "#14141A",
          floating: "#1C1C24",
          border: "rgba(255, 255, 255, 0.06)",
        },
        glass: {
          1: "rgba(255, 255, 255, 0.02)",
          2: "rgba(255, 255, 255, 0.04)",
          3: "rgba(255, 255, 255, 0.06)",
          4: "rgba(255, 255, 255, 0.08)",
        },
      },
      fontFamily: {
        sans: ["Plus Jakarta Sans", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "Fira Code", "monospace"],
      },
      animation: {
        "float": "float 6s ease-in-out infinite",
        "float-slow": "floatSlow 8s ease-in-out infinite",
        "glow-pulse": "glowPulse 2s ease-in-out infinite",
        "breathe": "breathe 3s ease-in-out infinite",
        "fade-in": "fadeSlideUp 0.5s cubic-bezier(0.16, 1, 0.3, 1) both",
        "shimmer": "shimmer 1.5s ease-in-out infinite",
        "border-pulse": "borderPulse 3s ease-in-out infinite",
        "data-flow": "dataFlow 4s ease-in-out infinite",
        "slide-in": "slideIn 0.3s cubic-bezier(0.16, 1, 0.3, 1) both",
      },
      keyframes: {
        float: {
          "0%, 100%": { transform: "translateY(0)" },
          "50%": { transform: "translateY(-6px)" },
        },
        floatSlow: {
          "0%, 100%": { transform: "translateY(0) scale(1)" },
          "33%": { transform: "translateY(-8px) scale(1.02)" },
          "66%": { transform: "translateY(-3px) scale(0.98)" },
        },
        glowPulse: {
          "0%, 100%": { opacity: "0.6" },
          "50%": { opacity: "1" },
        },
        breathe: {
          "0%, 100%": { transform: "scale(1)", opacity: "0.8" },
          "50%": { transform: "scale(1.05)", opacity: "1" },
        },
        fadeSlideUp: {
          "from": { opacity: "0", transform: "translateY(20px)" },
          "to": { opacity: "1", transform: "translateY(0)" },
        },
        shimmer: {
          "0%": { backgroundPosition: "-200% 0" },
          "100%": { backgroundPosition: "200% 0" },
        },
        borderPulse: {
          "0%, 100%": { borderColor: "rgba(255, 255, 255, 0.06)" },
          "50%": { borderColor: "rgba(245, 158, 11, 0.15)" },
        },
        dataFlow: {
          "0%": { backgroundPosition: "0% 50%" },
          "50%": { backgroundPosition: "100% 50%" },
          "100%": { backgroundPosition: "0% 50%" },
        },
        slideIn: {
          "from": { opacity: "0", transform: "translateX(16px)" },
          "to": { opacity: "1", transform: "translateX(0)" },
        },
      },
    },
  },
  plugins: [],
}

export default config
