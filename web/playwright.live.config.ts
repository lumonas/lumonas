import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  testMatch: 'live.spec.ts',
  timeout: 60_000,
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: 'http://localhost:5173',
    headless: true,
    trace: 'retain-on-failure',
  },
  webServer: {
    command:
      'GOCACHE=${GOCACHE:-/tmp/lumonas-go-build} GOPATH=$(go env GOPATH) LUMONAS_DEV_DIR=$(mktemp -d /tmp/lumonas-live.XXXXXX) LUMONAS_AUTH_REQUIRED=false bash ../scripts/dev.sh full',
    url: 'http://localhost:5173',
    reuseExistingServer: false,
    timeout: 120_000,
    env: {
      VITE_USE_MOCKS: 'false',
    },
  },
  projects: [
    {
      name: 'chromium-live',
      use: { browserName: 'chromium' },
    },
  ],
})
