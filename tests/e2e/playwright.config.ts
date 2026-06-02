import { defineConfig } from '@playwright/test';

// E2E-конфигурация для «База Сколково». Интерфейсы живут на разных портах,
// поэтому baseURL не задаём — спеки используют абсолютные URL из support/config.ts.
export default defineConfig({
  testDir: './specs',
  timeout: 45_000,
  expect: { timeout: 8_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [
    ['list'],
    ['json', { outputFile: 'results/results.json' }],
    ['html', { outputFolder: 'results/html', open: 'never' }],
  ],
  use: {
    actionTimeout: 12_000,
    navigationTimeout: 20_000,
    ignoreHTTPSErrors: true,
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    video: 'off',
    viewport: { width: 1366, height: 900 },
    locale: 'ru-RU',
  },
});
