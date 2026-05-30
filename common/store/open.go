package store

import (
	"context"
	"fmt"

	"baza-skolkovo/src/common/config"
)

// Open создаёт реализацию Store согласно конфигурации (backend json|postgres).
func Open(ctx context.Context, cfg config.Config) (Store, error) {
	switch cfg.StoreBackend {
	case "", "json":
		return NewJSONStore(cfg.RegistryPath)
	case "postgres":
		return NewPostgresStore(ctx, cfg.PostgresDSN)
	default:
		return nil, fmt.Errorf("неизвестный STORE_BACKEND: %q", cfg.StoreBackend)
	}
}
