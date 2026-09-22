import { defineConfig } from '@playwright/test';

const baseURL = process.env.BASE_URL || 'http://127.0.0.1:8080';

// CI runs the bundled chromium project after `npx playwright install chromium`.
// For local runs, `npm run test:local` uses the system Chrome binary
// (`channel: 'chrome'`) so no browser download is required.
export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  retries: 1,
  reporter: [
    ['list'],
    ['html', { open: 'never' }],
  ],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
    { name: 'local-chrome', use: { browserName: 'chromium', channel: 'chrome' } },
  ],
});