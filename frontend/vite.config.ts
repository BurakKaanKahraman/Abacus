import react from '@vitejs/plugin-react';
// vitest/config re-exports Vite's defineConfig with the `test` block typed.
import { defineConfig } from 'vitest/config';

export default defineConfig(({ mode }) => ({
  plugins: [react()],
  server: {
    port: 5173,
    // Fail loudly instead of silently moving to another port, so the backend
    // CORS allowlist always matches the URL the app is actually served from.
    strictPort: true,
  },
  build: {
    outDir: 'dist',
    // A production source map is the whole TypeScript source, several times
    // the size of the bundle, served to anyone who asks. Kept for every other
    // mode, where it costs nothing and makes a stack trace readable.
    sourcemap: mode !== 'production',
  },
  test: {
    environment: 'jsdom',
    // A concrete origin: with an opaque one, jsdom cannot provide a real
    // Storage implementation and localStorage degrades to an empty object.
    environmentOptions: {
      jsdom: { url: 'http://localhost:5173' },
    },
    globals: true,
    setupFiles: ['./tests/setup.ts'],
    // Tests live outside src/ so that production sources stay free of test
    // files, mirroring the backend's tests/ layout.
    include: ['tests/**/*.test.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}'],
      // main.tsx is the bootstrap and types/ is erased at compile time, so
      // neither has runtime statements worth reporting on.
      exclude: ['src/main.tsx', 'src/types/**', 'src/**/*.d.ts'],
      reporter: ['text', 'html'],
    },
  },
}));
