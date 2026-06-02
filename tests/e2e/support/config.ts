// Общие константы и помощники для E2E-тестов «База Сколково».
// Хост/креды берутся из окружения с дев-значениями по умолчанию (локальный docker-compose).

export const HOST = process.env.SK_HOST || 'http://localhost';

export const PORTS = {
  mcp: 8080,
  admin: 8090,
  residency: 8091,
  portal: 8092,
  chat: 8093,
  consultant: 8094,
  metrics: 9090,
} as const;

export const CREDS = {
  admin: { user: process.env.SK_ADMIN_USER || 'admin', pass: process.env.SK_ADMIN_PASS || 'change-me-please' },
  consultant: { user: process.env.SK_CONSULTANT_USER || 'consultant', pass: process.env.SK_CONSULTANT_PASS || 'change-me-please' },
};

export const MCP_API_KEY = process.env.SK_MCP_API_KEY || '517a4b18d8701532ce5e9d50671395b8602a9f9e68691f1d';

export const url = (port: number, path = '') => `${HOST}:${port}${path}`;
