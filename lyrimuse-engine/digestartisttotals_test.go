package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// 第一页就报总页数超过上限的账号:只请求这一页就放弃,之后一段时间内不再请求。
func TestLastfmArtistTotalsStopsAtFirstPageWhenTooMany(t *testing.T) {
	useUnthrottledGuard(t)
	savedClient, savedPath, savedTooMany := lastfmReadClient, artistTotalsPath, lastfmArtistTotalsTooMany
	t.Cleanup(func() {
		lastfmReadClient, artistTotalsPath, lastfmArtistTotalsTooMany = savedClient, savedPath, savedTooMany
	})
	artistTotalsPath = ""
	lastfmArtistTotalsTooMany.user, lastfmArtistTotalsTooMany.until = "", time.Time{}
	requests := 0
	lastfmReadClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		body := `{"topartists":{"artist":[{"name":"A","playcount":"3"}],"@attr":{"totalPages":"25"}}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	now := time.Now()
	if _, err := lastfmArtistTotals(t.Context(), "big", "k", now.Add(-24*time.Hour), now); err == nil {
		t.Fatal("超过上限应报错")
	}
	if requests != 1 {
		t.Errorf("第一页就该放弃,实际请求 %d 页", requests)
	}
	if _, err := lastfmArtistTotals(t.Context(), "big", "k", now.Add(-24*time.Hour), now); err == nil || requests != 1 {
		t.Errorf("记住之后不该再请求,请求 %d 次 err=%v", requests, err)
	}
	if _, err := lastfmArtistTotals(t.Context(), "other", "k", now.Add(-24*time.Hour), now); err == nil || requests != 2 {
		t.Errorf("换了账号要重新请求,请求 %d 次 err=%v", requests, err)
	}
}
