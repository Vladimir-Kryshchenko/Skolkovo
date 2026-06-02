// Каталог всех HTML-страниц системы по интерфейсам — инвентарь для E2E.
// Источник: src/navindex/tree.go (структура сайта). JSON/API-эндпоинты и MCP
// сюда не входят (проверяются отдельно). {id}-страницы резолвятся в спеке динамически.
import { PORTS } from './config';

export type AuthKind = 'none' | 'admin' | 'portal' | 'basic-residency' | 'basic-consultant';

export interface PageDef {
  route: string;
  title: string; // ожидаемый заметный текст (для проверки, что страница про то)
  expectNav?: boolean; // есть общий sidebar/nav
}

export interface PageGroup {
  iface: string;
  port: number;
  auth: AuthKind;
  pages: PageDef[];
}

export const GROUPS: PageGroup[] = [
  {
    iface: 'Главная Админка (:8090)', port: PORTS.admin, auth: 'admin',
    pages: [
      { route: '/', title: 'Документы', expectNav: true },
      { route: '/sitepages', title: 'Страницы сайта', expectNav: true },
      { route: '/changes', title: 'Изменения', expectNav: true },
      { route: '/diff', title: 'версий', expectNav: true },
      { route: '/analytics', title: 'Аналитика', expectNav: true },
      { route: '/graph', title: 'связ', expectNav: true },
      { route: '/regulations', title: 'НПА', expectNav: true },
      { route: '/proxy', title: 'прокси', expectNav: true },
      { route: '/ai/models', title: 'модел', expectNav: true },
      { route: '/ai/agents', title: 'агент', expectNav: true },
    ],
  },
  {
    iface: 'Резидентство-Админ (:8091)', port: PORTS.residency, auth: 'basic-residency',
    pages: [
      { route: '/clients', title: 'Клиент', expectNav: true },
      { route: '/checklists', title: 'Чек-лист', expectNav: true },
      { route: '/deadlines', title: 'Дедлайн', expectNav: true },
      { route: '/templates', title: 'Шаблон', expectNav: true },
      { route: '/tenants', title: 'Тенант', expectNav: true },
      { route: '/events-admin', title: 'Мероприят', expectNav: true },
      { route: '/contests-admin', title: 'Конкурс', expectNav: true },
    ],
  },
  {
    iface: 'Дашборд консультанта (:8094)', port: PORTS.consultant, auth: 'basic-consultant',
    pages: [
      { route: '/consultant/dashboard', title: 'консультанта' },
    ],
  },
  {
    iface: 'Портал клиента (:8092)', port: PORTS.portal, auth: 'portal',
    pages: [
      { route: '/dashboard', title: 'стадия', expectNav: true },
      { route: '/checklists', title: 'чек-лист', expectNav: true },
      { route: '/deadlines', title: 'дедлайн', expectNav: true },
      { route: '/documents', title: 'документ', expectNav: true },
      { route: '/generate', title: 'Генерация', expectNav: true },
      { route: '/subscriptions', title: 'Подписк', expectNav: true },
      { route: '/notifications', title: 'Уведомлен' },
    ],
  },
  {
    iface: 'Чат-виджет (:8093)', port: PORTS.chat, auth: 'none',
    pages: [
      { route: '/chat', title: 'Консультант' },
    ],
  },
  {
    iface: 'Страницы входа (публичные)', port: PORTS.portal, auth: 'none',
    pages: [
      { route: '/login', title: 'кабинет' },
    ],
  },
];

// Маркеры серверных/шаблонных ошибок, которых не должно быть в отрендеренном HTML.
export const ERROR_MARKERS: RegExp[] = [
  /can't evaluate/i,
  /can&#39;t evaluate/i,
  /template:\s/i,
  /executing ".*" at <\./i,
  /runtime error/i,
  /panic:/i,
  /Internal Server Error/i,
  /SQLSTATE/i,
  /no such (table|column)/i,
  /nil pointer/i,
];
