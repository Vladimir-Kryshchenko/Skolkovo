// default_tenant.go — единая идемпотентная логика «получить-или-создать» тенант
// по умолчанию. Один источник истины для всех путей, где требуется tenant_id, но
// он не задан: MCP create_client, HTTP/JSON-API Резидентство-Админ, форма /tenants.
//
// Раньше тенант «Default» создавался вслепую (форма + тестовый помощник INSERT-или),
// из-за чего в БД накапливались дубли строк «Default». Здесь — get-or-create:
// существующий тенант «Default» переиспользуется, новый создаётся только при его
// отсутствии.
package store

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"baza-skolkovo/src/common/model"
)

// DefaultTenantName — имя тенанта по умолчанию (единый литерал для всех вызовов).
const DefaultTenantName = "Default"

// DefaultTenantEnsurer — минимальное подмножество TenantStore, нужное для
// идемпотентного получения тенанта по умолчанию. *PostgresClientStore и интерфейс
// TenantStore ему удовлетворяют (как DeadlineEnsurer для дедлайнов).
type DefaultTenantEnsurer interface {
	ListTenants(ctx context.Context) ([]*model.Tenant, error)
	CreateTenant(ctx context.Context, tenant *model.Tenant) error
}

// GetOrCreateDefaultTenant возвращает существующий тенант с именем «Default»
// (без учёта регистра/пробелов), а если такого нет — создаёт новый и возвращает его.
// Операция идемпотентна: повторные вызовы отдают один и тот же тенант, дубли не плодятся.
func GetOrCreateDefaultTenant(ctx context.Context, ts DefaultTenantEnsurer) (*model.Tenant, error) {
	if ts == nil {
		return nil, ErrEmptyField
	}

	tenants, err := ts.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	for _, t := range tenants {
		if strings.EqualFold(strings.TrimSpace(t.Name), DefaultTenantName) {
			return t, nil
		}
	}

	// Тенанта «Default» ещё нет — создаём. APIKey обязателен (validateTenant),
	// поэтому генерируем уникальный ключ.
	tenant := &model.Tenant{
		ID:     uuid.New().String(),
		Name:   DefaultTenantName,
		APIKey: "default-" + uuid.New().String(),
		Active: true,
	}
	if err := ts.CreateTenant(ctx, tenant); err != nil {
		return nil, err
	}
	return tenant, nil
}
