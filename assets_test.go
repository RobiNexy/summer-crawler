package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDownloadAlbumAssetsSkipsUnavailableResource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "临时故障", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := NewHTTPClient(time.Second, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := map[string]json.RawMessage{"avatar": strconvQuote(server.URL + "/avatar.png")}

	if err := DownloadAlbumAssets(context.Background(), client, nil, t.TempDir(), nil, 1, profile, nil, nil, nil); err != nil {
		t.Fatalf("资源不可下载不应中断归档：%v", err)
	}
	if got := rawString(profile, "avatar"); got != server.URL+"/avatar.png" {
		t.Fatalf("不可下载资源应保留原始地址，得到 %q", got)
	}
}
