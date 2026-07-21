/**
 * E2E: Smoke Tests — Every route renders without crashing.
 * Fast, broad coverage. If this fails, something is fundamentally broken.
 */
import { test, expect } from '@playwright/test';
import { loginAs, loginAsAdmin, goto, expectNoError } from './setup.js';
import { API_URL } from './ports.js';

test.describe('Smoke — Public Routes', () => {
  const publicRoutes = [
    '/',
    '/login',
    '/patches',
    '/events',
  ];

  for (const route of publicRoutes) {
    test(`${route} renders`, async ({ page }) => {
      await goto(page, route);
      await expectNoError(page);
    });
  }
});

test.describe('Smoke — Authenticated Routes (Admin)', () => {
  const routes = [
    '/dashboard',
    '/settings',
    '/settings/notifications',
    '/settings/security',
    '/settings/patches',
    '/settings/quilts',
    '/notifications',
    '/activity',
    '/patches/new',
    '/events/new',
    '/admin',
    '/admin/users',
    '/admin/reports',
    '/admin/audit',
    '/admin/submissions',
    '/admin/claims',
  ];

  for (const route of routes) {
    test(`${route} renders for admin`, async ({ page }) => {
      await loginAsAdmin(page);
      await goto(page, route);
      await expectNoError(page);
    });
  }
});

test.describe('Smoke — Patch Routes', () => {
  const SLUG = 'lancaster-arts-district';
  const patchRoutes = [
    `/patches/${SLUG}`,
    `/patches/${SLUG}/members`,
    `/patches/${SLUG}/events`,
    `/patches/${SLUG}/governance`,
    `/patches/${SLUG}/governance/docs`,
    `/patches/${SLUG}/governance/proposals`,
    `/patches/${SLUG}/governance/new`,
    `/patches/${SLUG}/settings/info`,
    `/patches/${SLUG}/settings/members`,
    `/patches/${SLUG}/settings/notifications`,
    `/patches/${SLUG}/settings/danger`,
  ];

  for (const route of patchRoutes) {
    test(`${route} renders`, async ({ page }) => {
      await loginAsAdmin(page);
      await goto(page, route);
      await expectNoError(page);
    });
  }
});

test.describe('Smoke — Different User Roles', () => {
  const roles = ['admin', 'organizer', 'active', 'lurker', 'new'];

  for (const role of roles) {
    test(`dashboard loads for ${role}`, async ({ page }) => {
      await loginAs(page, role);
      await goto(page, '/dashboard');
      await expectNoError(page);
    });

    test(`home page loads for ${role}`, async ({ page }) => {
      await loginAs(page, role);
      await goto(page, '/');
      await expectNoError(page);
    });
  }
});

test.describe('Smoke — API Health', () => {
  test('health endpoint returns ok', async ({ request }) => {
    const response = await request.get(`${API_URL}/api/v1/health`);
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    expect(body.status).toBe('ok');
  });

  test('instance endpoint returns config', async ({ request }) => {
    const response = await request.get(`${API_URL}/api/v1/instance`);
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    expect(body.name).toBeTruthy();
  });
});
