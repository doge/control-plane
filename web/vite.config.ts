import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    host: "127.0.0.1",
    strictPort: true,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        // Keep the browser's Host header so the panel's same-origin check
        // accepts local WebSocket upgrades forwarded by Vite.
        changeOrigin: false,
        ws: true,
      },
    },
  },
});
