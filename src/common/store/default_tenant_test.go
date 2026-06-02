package store

import (
	"context"
	"testing"

	"baza-skolkovo/src/common/model"
)

// fakeTenantEnsurer — in-memory реализация DefaultTenantEnsurer для unit-тестов
// (без PostgreSQL). Считает число вставок, чтобы проверить идемпотентность.
type fakeTenantEnsurer struct {
	tenants   []*model.Tenant
	inserts   int
	listErr   error
	createErr error
}

func (f *fakeTenantEnsurer) ListTenants(ctx context.Context) ([]*model.Tenant, error) {
	return f.tenants, f.listErr
}

func (f *fakeTenantEnsurer) CreateTenant(ctx context.Context, t *model.Tenant) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.inserts++
	f.tenants = append(f.tenants, t)
	return nil
}

func TestGetOrCreateDefaultTenant_Idempotent(t *testing.T) {
	ctx := context.Background()
	fake := &fakeTenantEnsurer{}

	first, err := GetOrCreateDefaultTenant(ctx, fake)
	if err != nil {
		t.Fatalf("первый вызов: %v", err)
	}
	if first == nil || first.ID == "" {
		t.Fatal("ожидали непустой тенант с ID")
	}
	if first.Name != DefaultTenantName {
		t.Errorf("имя тенанта = %q, ожидали %q", first.Name, DefaultTenantName)
	}

	second, err := GetOrCreateDefaultTenant(ctx, fake)
	if err != nil {
		t.Fatalf("второй вызов: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("второй вызов вернул другой ID: %q != %q", second.ID, first.ID)
	}
	if fake.inserts != 1 {
		t.Errorf("вставок тенанта = %d, ожидали ровно 1 (нет дублей «Default»)", fake.inserts)
	}
}

func TestGetOrCreateDefaultTenant_ReusesExisting(t *testing.T) {
	ctx := context.Background()
	fake := &fakeTenantEnsurer{
		tenants: []*model.Tenant{
			{ID: "tenant-a", Name: "Acme"},
			{ID: "tenant-default", Name: " default "}, // регистр/пробелы игнорируются
		},
	}

	got, err := GetOrCreateDefaultTenant(ctx, fake)
	if err != nil {
		t.Fatalf("GetOrCreateDefaultTenant: %v", err)
	}
	if got.ID != "tenant-default" {
		t.Errorf("переиспользовали не тот тенант: %q, ожидали tenant-default", got.ID)
	}
	if fake.inserts != 0 {
		t.Errorf("вставок = %d, ожидали 0 (существующий «Default» переиспользован)", fake.inserts)
	}
}
