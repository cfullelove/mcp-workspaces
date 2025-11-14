import path from "path";
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, ".", "");
  return {
    // Serve SPA at root by default; override with VITE_BASE_PATH if needed
    base: env.VITE_BASE_PATH || "/",
    plugins: [react()],
    resolve: {
      alias: {
        "@": path.resolve(__dirname, "./src"),
      },
    },
    server: {
      proxy: {
        "/api": {
          target: "http://backend:8080",
          changeOrigin: true,
        },
        "/events": {
          target: "http://backend:8080",
          changeOrigin: true,
        },
      },
    },
  };
});
