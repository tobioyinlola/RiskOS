import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import prettier from "eslint-config-prettier/flat";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  prettier,
  {
    rules: {
      // Generated API types are consumed only through the typed client in lib/api.
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["**/schema.gen", "**/schema.gen.ts"],
              message: "Import API types and the client from @/lib/api/client.",
            },
          ],
        },
      ],
      "no-console": ["error", { allow: ["warn", "error"] }],
      // Bare TODOs are rejected by scripts/check-todos.sh; these markers are never allowed.
      "no-warning-comments": [
        "error",
        { terms: ["fixme", "xxx", "hack"], location: "anywhere" },
      ],
    },
  },
  {
    files: ["lib/api/**"],
    rules: { "no-restricted-imports": "off" },
  },
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "coverage/**",
    "playwright-report/**",
    "test-results/**",
    "next-env.d.ts",
    "lib/api/schema.gen.ts",
    "components/ui/**",
  ]),
]);

export default eslintConfig;
