import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// /api/* is proxied to the Go backend so the browser never deals with CORS.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: process.env.VITE_API_PROXY_TARGET ?? "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
});
