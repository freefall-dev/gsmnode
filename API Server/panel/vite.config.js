import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";

// Builds into internal/api/dist, which the Go server embeds via go:embed
// and serves at the server root.
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: {
    outDir: "../internal/api/dist",
    emptyOutDir: true,
  },
  server: {
    port: 5174,
    proxy: {
      // Dev-mode proxy to a locally running API Server.
      "/api": "http://localhost:8080",
    },
  },
});
