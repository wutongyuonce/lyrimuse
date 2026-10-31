package main

import (
	"context"
	"errors"
	"testing"
)

// 连着 ListenBrainz、令牌在钥匙串里读不出时,完成的收听要报瞬时错误交给重试,不能当成没连静默丢掉。
func TestSubmitKeepsListenWhenKeychainUnreadable(t *testing.T) {
	c := &credentialVaultCache
	c.Lock()
	oldFailed := c.failed
	c.failed = true
	c.Unlock()
	t.Cleanup(func() {
		c.Lock()
		c.failed = oldFailed
		c.Unlock()
	})
	ctx := context.Background()
	meta := lbTrackMeta{TrackName: "t", ArtistName: "a"}
	lb := &lbClient{user: "alice"}
	if err := lb.submit(ctx, "single", 1700000000, meta); !errors.Is(err, errListenCredentialsUnavailable) {
		t.Errorf("连着、钥匙串读不出时完成收听应报瞬时错误,得到 %v", err)
	}
	if err := lb.submit(ctx, "playing_now", 0, meta); err != nil {
		t.Errorf("正在播放照旧静默跳过,得到 %v", err)
	}
	lb.user = ""
	if err := lb.submit(ctx, "single", 1700000000, meta); err != nil {
		t.Errorf("没连 ListenBrainz 时照旧静默跳过,得到 %v", err)
	}
	c.Lock()
	c.failed = false
	c.Unlock()
	lb.user = "alice"
	if err := lb.submit(ctx, "single", 1700000000, meta); err != nil {
		t.Errorf("钥匙串读得出、只是没令牌时照旧静默跳过,得到 %v", err)
	}
}
