import { test, expect, type Page } from '@playwright/test';

const ADMIN_USERNAME = process.env.ADMIN_USERNAME || 'admin';
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD || 'admin123';
const TARGET_URL = 'https://example.com/e2e-target';

function uniqueSuffix(): string {
  return `${Date.now()}_${Math.floor(Math.random() * 1e6)}`;
}

async function registerUser(page: Page): Promise<{ username: string; password: string }> {
  const suffix = uniqueSuffix();
  const username = `e2e_${suffix}`;
  const password = `e2e-password-${suffix}`;

  await page.goto('/');
  await page.locator('.toggle-link', { hasText: 'Register here' }).click();
  await page.fill('#regUsername', username);
  await page.fill('#regEmail', `${username}@example.test`);
  await page.fill('#regPassword', password);
  await page.click('#registerBtn');

  await expect(page.locator('#dashboardSection.active')).toBeVisible();
  return { username, password };
}

async function loginAsUser(page: Page, username: string, password: string): Promise<void> {
  await page.goto('/');
  await page.fill('#loginUsername', username);
  await page.fill('#loginPassword', password);
  await page.click('#loginBtn');
  await expect(page.locator('#dashboardSection.active')).toBeVisible();
}

test('register, create short URL, redirect, logout and login again', async ({ page }) => {
  const { username, password } = await registerUser(page);

  // Create a short URL through the UI.
  await page.fill('#originalUrl', TARGET_URL);
  await page.click('#createUrlBtn');
  await expect(page.locator('#resultSection')).toBeVisible();

  const code = (await page.locator('#resultCode').textContent())?.trim();
  expect(code).toBeTruthy();
  const shortURL = (await page.locator('#resultUrl').textContent())?.trim() ?? '';
  expect(shortURL).toContain(`/r/${code}`);

  // The dashboard "My URLs" list shows the newly created row.
  await page.locator('.tab-button[data-tab="myUrlsTab"]').click();
  await expect(page.locator('#myUrlsList .url-item')).toBeVisible();
  await expect(page.locator('#myTotalUrls')).toHaveText('1');
  await expect(page.locator('#myTotalVisits')).toHaveText('0');

  // The canonical public redirect /r/:code navigates to the target.
  await page.goto(`/r/${code}`);
  await expect(page).toHaveURL(TARGET_URL);

  // Logout returns to the auth view; the stored session is gone.
  await page.goto('/');
  page.on('dialog', (dialog) => dialog.accept());
  await page.click('#logoutBtn');
  await expect(page.locator('#authSection')).toBeVisible();
  await expect(page.locator('#dashboardSection')).not.toHaveClass(/active/);

  // The same credentials still log in afterwards.
  await loginAsUser(page, username, password);
  await expect(page.locator('#currentUser')).toHaveText(username);
});

test('a stale token in localStorage is rejected on reload (session restore)', async ({ page }) => {
  await registerUser(page);

  // Tamper with the stored token, then reload: the SPA must validate the
  // session against /api/v1/auth/me instead of trusting localStorage.
  await page.evaluate(() => localStorage.setItem('token', 'stale-invalid-token'));
  await page.reload();

  await expect(page.locator('#authSection')).toBeVisible();
  await expect(page.locator('#dashboardSection')).not.toHaveClass(/active/);
  await expect(page.locator('#authErrorMsg')).toHaveText('Session expired, please login again');

  const token = await page.evaluate(() => localStorage.getItem('token'));
  expect(token).toBeNull();
});

test('an admin sees the admin-only tabs', async ({ page }) => {
  await loginAsUser(page, ADMIN_USERNAME, ADMIN_PASSWORD);

  await expect(page.locator('.tab-button[data-tab="allUrlsTab"]')).toBeVisible();
  await expect(page.locator('.tab-button[data-tab="usersTab"]')).toBeVisible();

  // Admin tabs load their content; the admin row is always present exactly
  // once (by username cell), regardless of how many other users earlier tests
  // registered in the shared store. Note: hasText is case-insensitive, so it
  // must be scoped to the username cell — otherwise every row matches via the
  // "Change to Admin" action button.
  await page.locator('.tab-button[data-tab="usersTab"]').click();
  await expect(page.locator('#usersTableBody td:first-child', { hasText: 'admin' })).toHaveCount(1);
});

test('a regular user does not see the admin-only tabs', async ({ page }) => {
  await registerUser(page);

  await expect(page.locator('.tab-button[data-tab="allUrlsTab"]')).toBeHidden();
  await expect(page.locator('.tab-button[data-tab="usersTab"]')).toBeHidden();
});