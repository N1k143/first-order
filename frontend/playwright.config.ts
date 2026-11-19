import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './tests/browser',
  fullyParallel: true,
  use: {
    baseURL: 'http://127.0.0.1:3121',
    headless: true,
    launchOptions: process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : {},
  },
  webServer: {
    command: 'npm run start -- --hostname 127.0.0.1 --port 3121',
    url: 'http://127.0.0.1:3121',
    reuseExistingServer: false,
    timeout: 60000,
  },
});
