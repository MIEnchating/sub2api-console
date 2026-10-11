import { availableParallelism } from "node:os";
import { fileURLToPath } from "node:url";

import { defineConfig } from "vitest/config";

// Keep jsdom workers below the measured contention limit and leave smaller hosts room to run.
const testWorkers = Math.max(1, Math.min(8, availableParallelism() - 1));

export default defineConfig({
  test: {
    environment: "jsdom",
    // DOM tests use .test.tsx; a .test.ts can opt in with @vitest-environment jsdom.
    environmentMatchGlobs: [["**/*.test.ts", "node"]],
    setupFiles: ["./src/test/setup.ts"],
    maxWorkers: testWorkers,
    minWorkers: testWorkers,
  },
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
});
