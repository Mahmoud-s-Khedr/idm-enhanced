import { defineConfig } from 'vitest/config';
export default defineConfig({
  test: {
    environment: 'jsdom',
    setupFiles: ['frontend/src/test-setup.ts'],
    include: [
      'frontend/src/**/*.test.tsx',
      'extensions/shared/src/**/*.test.ts',
    ],
    coverage: {
      provider: 'v8',
      include: [
        'frontend/src/App.tsx',
        'extensions/shared/src/connection.ts',
        'extensions/shared/src/popup.ts',
      ],
      thresholds: {
        perFile: true,
        lines: 80,
        statements: 80,
        functions: 80,
        branches: 75,
      },
    },
  },
});
