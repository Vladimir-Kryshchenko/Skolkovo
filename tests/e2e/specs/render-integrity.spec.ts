// Render-integrity: логинимся в каждый интерфейс, открываем каждую страницу,
// проверяем полный рендер (статус < 400, есть </html>, нет маркеров ошибок,
// есть навигация где положено). Этот guard ловит класс ошибок шаблонов/handlers,
// из-за которых ЛК отдавал пустые/обрезанные страницы.
import { test, expect, Browser, BrowserContext } from '@playwright/test';
import { GROUPS, ERROR_MARKERS, AuthKind } from '../support/pages';
import { loginAdmin, loginPortal, basicResidency, basicConsultant } from '../support/auth';
import { url } from '../support/config';

const PORTAL_EMAIL = process.env.SK_PORTAL_EMAIL || 'qa-resident@example.com';

async function makeContext(browser: Browser, auth: AuthKind): Promise<BrowserContext> {
  if (auth === 'basic-residency') return browser.newContext({ httpCredentials: basicResidency });
  if (auth === 'basic-consultant') return browser.newContext({ httpCredentials: basicConsultant });
  const ctx = await browser.newContext();
  if (auth === 'admin') await loginAdmin(ctx);
  if (auth === 'portal') await loginPortal(ctx, PORTAL_EMAIL);
  return ctx;
}

for (const grp of GROUPS) {
  test.describe(grp.iface, () => {
    let ctx: BrowserContext;
    test.beforeAll(async ({ browser }) => { ctx = await makeContext(browser, grp.auth); });
    test.afterAll(async () => { await ctx?.close(); });

    for (const p of grp.pages) {
      test(`${p.route} — полный рендер без ошибок`, async () => {
        const page = await ctx.newPage();
        try {
          const resp = await page.goto(url(grp.port, p.route), { waitUntil: 'domcontentloaded' });
          expect(resp, `нет ответа на ${p.route}`).toBeTruthy();
          expect(resp!.status(), `HTTP-статус ${p.route}`).toBeLessThan(400);

          const html = await page.content();
          expect(html.length, `${p.route} подозрительно мал (${html.length}b)`).toBeGreaterThan(500);
          expect(html, `${p.route} — не полный документ (нет </html>)`).toContain('</html>');

          for (const re of ERROR_MARKERS) {
            expect(html, `${p.route} содержит маркер ошибки ${re}`).not.toMatch(re);
          }
          if (p.expectNav) {
            const navCount = await page.locator('nav, .sidebar, aside').count();
            expect(navCount, `${p.route} — ожидалась навигация (nav/sidebar)`).toBeGreaterThan(0);
          }
        } finally {
          await page.close();
        }
      });
    }
  });
}
