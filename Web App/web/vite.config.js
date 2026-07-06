import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import tailwindcss from "@tailwindcss/vite";

// The production build is written into ../server/dist so the Go BFF can embed it.
// In dev, /api is proxied to the API Server directly.
export default defineConfig({
  plugins: [vue(), tailwindcss()],
  build: {
    outDir: "../server/dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.API_BASE || "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
