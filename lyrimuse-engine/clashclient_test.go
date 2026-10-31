package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// clashClient 每次新建,不能留空闲连接。
func TestClashClientDoesNotKeepIdleConnections(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "mihomo.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	saved := clashControllerSockets
	t.Cleanup(func() { clashControllerSockets = saved })
	clashControllerSockets = []string{sock}
	c := clashClient()
	if c == nil {
		t.Fatal("控制口存在时应拿到客户端")
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok || !tr.DisableKeepAlives {
		t.Errorf("Clash 客户端应关掉连接复用,得到 %#v", c.Transport)
	}
}
