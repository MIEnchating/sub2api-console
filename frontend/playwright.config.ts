import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  testMatch: "**/*.e2e.ts",
  fullyParallel: true,
  workers: 2,
  use: {
    baseURL: "http://127.0.0.1:3013",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    launchOptions: {
      executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,
    },
  },
  projects: [
    {
      name: "desktop-light",
      use: { viewport: { width: 1440, height: 900 }, colorScheme: "light" },
    },
    { name: "mobile-dark", use: { viewport: { width: 390, height: 664 }, colorScheme: "dark" } },
  ],
  webServer: {
    command: "bun run dev --port 3013",
    url: "http://127.0.0.1:3013",
    reuseExistingServer: false,
    env: { VITE_API_BASE_URL: "http://127.0.0.1:19999" },
  },
});
