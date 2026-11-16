import { defineConfig } from 'vite';
import { readFileSync, cpSync, mkdirSync, rmSync } from 'node:fs';
export default defineConfig({
  root: 'extensions/shared',
  plugins: [
    {
      name: 'browser-manifests',
      closeBundle() {
        for (const name of ['firefox', 'chromium']) {
          rmSync(`extensions/${name}/dist`, { recursive: true, force: true });
          mkdirSync(`extensions/${name}/dist`, { recursive: true });
          cpSync('extensions/shared/dist', `extensions/${name}/dist`, {
            recursive: true,
          });
          cpSync(
            `extensions/${name}/manifest.json`,
            `extensions/${name}/dist/manifest.json`,
          );
          JSON.parse(
            readFileSync(`extensions/${name}/dist/manifest.json`, 'utf8'),
          );
        }
      },
    },
  ],
  build: {
    emptyOutDir: true,
    rollupOptions: { input: 'extensions/shared/popup.html' },
  },
});
