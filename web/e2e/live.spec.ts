import { expect, test } from '@playwright/test'

test('renders the onboarding UI from the real daemon without MSW', async ({ page, request }) => {
  const apiResponse = await request.get('/api/v1/onboarding/state')
  expect(apiResponse.ok()).toBeTruthy()
  expect(await apiResponse.json()).toEqual(expect.objectContaining({ completed: false }))

  await page.goto('/')
  await expect(page.getByRole('heading', { name: /first-time setup/i })).toBeVisible()
  await expect(page.getByText('Welcome to your NAS')).toBeVisible()
  expect(await page.evaluate(() => navigator.serviceWorker.controller)).toBeNull()
})
