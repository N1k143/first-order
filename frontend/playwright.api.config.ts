import { defineConfig } from '@playwright/test';

/** Local UI contract tests: all API traffic is mocked, no production writes. */
export default defineConfig({
  testDir: './tests/api-browser',
  fullyParallel: true,
  use: {
    baseURL: 'http://127.0.0.1:3122',
    launchOptions: process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : {},
  },
  webServer: {
    command:
      'NEXT_PUBLIC_DATA_MODE=api npm run build && npm run start -- --hostname 127.0.0.1 --port 3122',
    url: 'http://127.0.0.1:3122',
    reuseExistingServer: false,
    timeout: 120000,
  },
});
