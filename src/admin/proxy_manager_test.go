package admin

import (
	"path/filepath"
	"testing"
	"time"
)

// TestProxyManagerMutateNoDeadlock — регрессионный guard на самоблокировку:
// мутирующие методы внутри вызывают saveLocked, и раньше save() повторно брал
// уже удержанный pm.mu (sync.Mutex не рекурсивный) → горутина зависала навсегда,
// удерживая mu, из-за чего страница /proxy не отвечала (page.goto timeout).
// Операции не делают сетевых вызовов (TestProxy намеренно не вызываем).
func TestProxyManagerMutateNoDeadlock(t *testing.T) {
	pm := NewProxyManager(filepath.Join(t.TempDir(), "proxies.json"))

	done := make(chan struct{})
	go func() {
		id1 := pm.AddProxy("t1", "http", "http://127.0.0.1:1")
		pm.ActivateProxy(id1)
		id2 := pm.AddProxy("t2", "http", "http://127.0.0.1:2")
		pm.AutoSwitch()
		_ = pm.GetActiveURL()
		_ = pm.RemoveProxy(id2)
		_ = pm.RemoveProxy(id1)
		close(done)
	}()

	select {
	case <-done:
		// ok — все операции завершились без самоблокировки
	case <-time.After(3 * time.Second):
		t.Fatal("ProxyManager deadlock: мутирующие операции не завершились за 3с (saveLocked/mu)")
	}
}
