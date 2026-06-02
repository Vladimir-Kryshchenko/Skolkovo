// Вёрстка: «нет кривых страниц, текст помещается в блоки, отображается целиком».
// Сигнал — ВИДИМЫЙ, потоковый элемент, который реально вылезает за правый край
// вьюпорта. Исключаем артефакты, которые НЕ являются кривой вёрсткой:
//  - position fixed/absolute/sticky (тултипы [data-tooltip]::after, липкая шапка);
//  - элементы внутри контейнеров с overflow-x: auto/scroll/hidden (таблицы .table-wrap,
//    горизонтальное меню nav — это намеренная внутренняя прокрутка);
//  - невидимые (opacity:0 / нулевой размер).
// Дополнительно ловим обрезанный текст: блочный элемент с overflow:hidden без
// text-overflow:ellipsis, чей контент шире контейнера.
import { test, expect, Browser, BrowserContext } from '@playwright/test';
import { GROUPS } from '../support/pages';
import { authedContext } from '../support/auth';
import { url } from '../support/config';

const VIEWPORTS = [
  { name: 'desktop', width: 1366, height: 900 },
  { name: 'mobile', width: 390, height: 844 },
];

for (const grp of GROUPS) {
  test.describe(`Вёрстка — ${grp.iface}`, () => {
    for (const vp of VIEWPORTS) {
      test.describe(vp.name, () => {
        let ctx: BrowserContext;
        test.beforeAll(async ({ browser }) => {
          ctx = await authedContext(browser as Browser, grp.auth, { viewport: { width: vp.width, height: vp.height } });
        });
        test.afterAll(async () => { await ctx?.close(); });

        for (const p of grp.pages) {
          test(`${p.route} — нет реального переполнения (${vp.name})`, async () => {
            const page = await ctx.newPage();
            try {
              await page.goto(url(grp.port, p.route), { waitUntil: 'networkidle' }).catch(async () => {
                await page.goto(url(grp.port, p.route), { waitUntil: 'domcontentloaded' });
              });

              const report = await page.evaluate(() => {
                const clientW = document.documentElement.clientWidth;
                const inScrollContainer = (el: Element) => {
                  let p = el.parentElement;
                  while (p && p !== document.body) {
                    const ox = getComputedStyle(p).overflowX;
                    if (ox === 'auto' || ox === 'scroll' || ox === 'hidden') return true;
                    p = p.parentElement;
                  }
                  return false;
                };
                const overflowRight: string[] = [];
                const clipped: string[] = [];
                document.querySelectorAll('body *').forEach((el) => {
                  const he = el as HTMLElement;
                  const r = he.getBoundingClientRect();
                  if (r.width === 0 || r.height === 0) return;
                  const cs = getComputedStyle(he);
                  if (cs.opacity === '0' || cs.visibility === 'hidden') return;
                  const pos = cs.position;
                  const label = `${el.tagName.toLowerCase()}.${(el.getAttribute('class') || '').slice(0, 36)}`;
                  // 1) элемент вылезает за правый край вьюпорта
                  if (pos !== 'fixed' && pos !== 'absolute' && pos !== 'sticky' && !inScrollContainer(el)) {
                    if (r.right > clientW + 6 && r.left < clientW) {
                      overflowRight.push(`${label} right=${Math.round(r.right)}/${clientW}`);
                    }
                  }
                  // 2) обрезанный текст (контент шире блока, overflow скрыт, без ellipsis)
                  if (
                    (cs.overflowX === 'hidden' || cs.overflow === 'hidden') &&
                    cs.textOverflow !== 'ellipsis' &&
                    he.scrollWidth > he.clientWidth + 2 &&
                    he.childElementCount === 0 &&
                    (he.textContent || '').trim().length > 0
                  ) {
                    clipped.push(`${label} scrollW=${he.scrollWidth}>clientW=${he.clientWidth}`);
                  }
                });
                return { overflowRight: overflowRight.slice(0, 10), clipped: clipped.slice(0, 10) };
              });

              expect(report.overflowRight, `${p.route} (${vp.name}) — элементы вылезают за вьюпорт: ${report.overflowRight.join(' | ')}`).toEqual([]);
              expect(report.clipped, `${p.route} (${vp.name}) — обрезан текст: ${report.clipped.join(' | ')}`).toEqual([]);
            } finally {
              await page.close();
            }
          });
        }
      });
    }
  });
}
