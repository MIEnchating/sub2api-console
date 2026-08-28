import { defineConfig } from "@rsbuild/core";
import { pluginReact } from "@rsbuild/plugin-react";
import { pluginTailwindcss } from "@rsbuild/plugin-tailwindcss";

export default defineConfig({
  plugins: [pluginReact(), pluginTailwindcss({ optimize: false })],
  source: { entry: { index: "./src/main.tsx" } },
  resolve: { alias: { "@": "./src" } },
  html: { template: "./index.html" },
  server: {
    host: "0.0.0.0",
    port: 3004,
    proxy: {
      "/api": {
        target: process.env.VITE_API_BASE_URL || "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  output: { distPath: { root: "dist" } },
});
