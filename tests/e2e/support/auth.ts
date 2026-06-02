// Помощники аутентификации для E2E. Куки кладём в request-контекст браузера,
// поэтому последующие page.goto() идут уже авторизованными.
import { Browser, BrowserContext } from '@playwright/test';
import { PORTS, CREDS, url } from './config';
import type { AuthKind } from './pages';

// Админка :8090 — форма логина → cookie admin_session.
export async function loginAdmin(context: BrowserContext): Promise<void> {
  const resp = await context.request.post(url(PORTS.admin, '/login'), {
    form: { username: CREDS.admin.user, password: CREDS.admin.pass },
    maxRedirects: 0,
  });
  if (![303, 302, 200].includes(resp.status())) {
    throw new Error(`admin login: неожиданный статус ${resp.status()}`);
  }
}

// Портал :8092 — dev magic-link. Нужен заранее заведённый клиент с этим email.
export async function loginPortal(context: BrowserContext, email: string): Promise<void> {
  const r1 = await context.request.post(url(PORTS.portal, '/login'), {
    form: { email },
    maxRedirects: 0,
  });
  const loc = r1.headers()['location'] || '';
  const m = decodeURIComponent(loc).match(/token=([a-f0-9]+)/i);
  if (!m) throw new Error(`portal login: не нашли magic-token в Location: ${loc}`);
  const verify = await context.request.get(url(PORTS.portal, `/login/verify?token=${m[1]}`), {
    maxRedirects: 0,
  });
  if (![303, 302].includes(verify.status())) {
    throw new Error(`portal verify: неожиданный статус ${verify.status()}`);
  }
}

export const basicResidency = { username: CREDS.admin.user, password: CREDS.admin.pass };
export const basicConsultant = { username: CREDS.consultant.user, password: CREDS.consultant.pass };

export const PORTAL_EMAIL = process.env.SK_PORTAL_EMAIL || 'qa-resident@example.com';

// authedContext — создаёт контекст с нужной аутентификацией (cookie или BasicAuth)
// и доп. опциями (например viewport). Используется во всех спеках.
export async function authedContext(
  browser: Browser,
  auth: AuthKind,
  extra: Record<string, any> = {},
): Promise<BrowserContext> {
  const opts: Record<string, any> = { ...extra };
  if (auth === 'basic-residency') opts.httpCredentials = basicResidency;
  if (auth === 'basic-consultant') opts.httpCredentials = basicConsultant;
  const ctx = await browser.newContext(opts);
  if (auth === 'admin') await loginAdmin(ctx);
  if (auth === 'portal') await loginPortal(ctx, PORTAL_EMAIL);
  return ctx;
}
