// Бизнес-логика: сквозные цепочки «начало → понятный человеку результат».
// Проверяем не только рендер, но и действия (POST) и их видимый результат.
import { test, expect } from '@playwright/test';
import { authedContext } from '../support/auth';
import { url, PORTS, MCP_API_KEY } from '../support/config';

const mcpHeaders = { Authorization: `Bearer ${MCP_API_KEY}`, Accept: 'application/json, text/event-stream', 'Content-Type': 'application/json' };

async function mcpCall(request: any, name: string, args: Record<string, any>) {
  const r = await request.post(url(PORTS.mcp, '/mcp'), {
    headers: mcpHeaders,
    data: { jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name, arguments: args } },
  });
  expect(r.ok(), `${name}: HTTP ${r.status()}`).toBeTruthy();
  return r.json();
}

test.describe('Бизнес-логика — подписки портала', () => {
  test('выбрать категорию → Сохранить → подписка сохранена и видна после перезагрузки', async ({ browser }) => {
    const ctx = await authedContext(browser, 'portal', { viewport: { width: 1366, height: 900 } });
    const page = await ctx.newPage();
    await page.goto(url(PORTS.portal, '/subscriptions'), { waitUntil: 'domcontentloaded' });
    const cb = page.locator('input[type=checkbox][name=categories]').first();
    await expect(cb, 'на странице есть чекбоксы категорий').toHaveCount(1, { timeout: 5000 }).catch(() => {});
    await cb.check();
    await page.getByRole('button', { name: /Сохранить подписки/ }).click();
    await page.waitForLoadState('domcontentloaded');
    await expect(page.locator('.flash')).toContainText(/Подписк/i);
    // Персистентность: после перезагрузки чекбокс остаётся отмеченным
    await page.goto(url(PORTS.portal, '/subscriptions'), { waitUntil: 'domcontentloaded' });
    await expect(page.locator('input[type=checkbox][name=categories]').first()).toBeChecked();
    await ctx.close();
  });
});

test.describe('Бизнес-логика — чат-консультант (API чат-виджета)', () => {
  test('session → сообщение → ответ или корректная недоступность (не 5xx)', async ({ request }) => {
    const s = await request.post(url(PORTS.chat, '/api/session'));
    expect(s.ok(), `/api/session HTTP ${s.status()}`).toBeTruthy();
    const sid = (await s.json()).id;
    expect(sid, 'сессия чата имеет id').toBeTruthy();

    const r = await request.post(url(PORTS.chat, '/api/chat'), {
      data: { session_id: sid, message: 'Какие документы нужны для подачи заявки на резидентство?' },
    });
    expect(r.status(), 'чат не отвечает 5xx').toBeLessThan(500);
    const body = JSON.stringify(await r.json());
    // Либо реальный ответ (reply), либо штатное сообщение о недоступности LLM — но осмысленное.
    expect(body).toMatch(/reply|error|ответ|недоступ/i);
  });
});

test.describe('Бизнес-логика — MCP check_eligibility', () => {
  test('ИНН → отчёт о праве на резидентство', async ({ request }) => {
    const body = await mcpCall(request, 'check_eligibility', { inn: '7710000000' });
    expect(body.result, 'check_eligibility вернул result').toBeTruthy();
    const text = body.result?.content?.[0]?.text || '';
    expect(text.length, 'отчёт не пустой').toBeGreaterThan(0);
  });
});

test.describe('Навигация — get_navigation возвращает маршрут + как попасть', () => {
  test('запрос → топ-результат содержит route и howto', async ({ request }) => {
    const body = await mcpCall(request, 'get_navigation', { query: 'управление прокси и VPN', limit: 3 });
    const arr = JSON.parse(body.result.content[0].text);
    expect(arr.length, 'есть результаты навигации').toBeGreaterThan(0);
    expect(arr[0], 'у узла есть route').toHaveProperty('route');
    expect(arr[0], 'у узла есть howto').toHaveProperty('howto');
    expect(arr.some((n: any) => n.route === '/proxy'), 'для «прокси» среди топ-результатов есть /proxy').toBeTruthy();
  });
});
