import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";

export default defineConfig([
  ...nextVitals,
  globalIgnores([".next/**", "out/**", "node_modules/**"]),
  {
    rules: {
      "react/no-unescaped-entities": "off",
      // Next 16 enables React-Compiler-stage rules by default. This app is a
      // static export and does not use the React Compiler; the patterns these
      // flag (e.g. the standard hydration guard `setMounted(true)` in an
      // effect) are intentional. Keep them off to preserve the Next 14 lint bar.
      "react-hooks/set-state-in-effect": "off",
      "react-hooks/purity": "off",
      "react-hooks/immutability": "off",
    },
  },
]);