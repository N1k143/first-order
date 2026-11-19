import { defineConfig } from '@playwright/test';

// This config never starts a server and never runs in ordinary browser tests.
const target = process.env.E2E_BASE_URL;
if (!target || process.env.E2E_ALLOW_WRITES !== '1') {
  throw new Error('Production E2E requires E2E_BASE_URL and E2E_ALLOW_WRITES=1.');
}
const url = new URL(target);
if (url.username || url.password || url.search || url.hash || url.pathname !== '/') {
  throw new Error('E2E_BASE_URL must be an origin without credentials, path or query.');
}
const localHTTP =
  url.protocol === 'http:' &&
  ['localhost', '127.0.0.1', '[::1]'].includes(url.hostname) &&
  process.env.E2E_ALLOW_HTTP === '1';
if (url.protocol !== 'https:' && !localHTTP) throw new Error('HTTPS is required.');
if (process.env.DEBUG || process.env.PWDEBUG)
  throw new Error('Disable DEBUG/PWDEBUG: browser debugging can expose credentials.');

export default defineConfig({
  testDir: './tests/production',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 120_000,
  expect: { timeout: 20_000 },
  reporter: [['./tests/production/safe-reporter.ts']],
  outputDir: '/tmp/firstorder-production-browser-artifacts',
  preserveOutput: 'never',
  use: {
    baseURL: url.origin,
    headless: true,
    trace: 'off',
    video: 'off',
    screenshot: 'off',
    launchOptions: process.env.CHROME_PATH ? { executablePath: process.env.CHROME_PATH } : {},
  },
});
