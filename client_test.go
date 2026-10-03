package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPClientRetriesHTTPFailures(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			http.Error(w, "临时故障", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client, err := NewHTTPClient(time.Second, 3, 0, log.New(&strings.Builder{}, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]bool
	if err := client.GetJSON(context.Background(), server.URL, nil, &result); err != nil {
		t.Fatal(err)
	}
	if !result["ok"] || attempts.Load() != 3 {
		t.Fatalf("结果=%v 尝试次数=%d", result, attempts.Load())
	}
}

func TestCrawlerFetchMemoriesStopsAtEmptyPage(t *testing.T) {
	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offsets = append(offsets, r.URL.Query().Get("offset"))
		if r.URL.Query().Get("offset") == "0" {
			_, _ = w.Write([]byte(`[{"id":1}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	client, err := NewHTTPClient(time.Second, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	crawler := &Crawler{Client: client, BaseURL: server.URL, ActivityLimit: 1}
	memories, err := crawler.FetchMemories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 || len(offsets) != 2 || offsets[1] != "1" {
		t.Fatalf("动态=%s 偏移量=%v", mustJSON(memories), offsets)
	}
}

func TestCrawlerFetchFriendsAddsProfiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token" {
			t.Errorf("授权请求头 = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/user/friends":
			_, _ = w.Write([]byte(`[{"id":"42","nickname":"Ada","gender":2,"friend_days":10}]`))
		case "/users/42":
			_, _ = w.Write([]byte(`{"id":"42","nickname":"Ada","status":"normal"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewHTTPClient(time.Second, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	crawler := &Crawler{
		Client:  client,
		BaseURL: server.URL,
		Headers: http.Header{"Authorization": []string{"token"}},
	}
	friends, err := crawler.FetchFriends(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := crawler.EnrichFriendProfiles(context.Background(), friends); err != nil {
		t.Fatal(err)
	}
	if len(friends) != 1 || string(friends[0]["nickname"]) != `"Ada"` {
		t.Fatalf("好友=%s", mustJSON(friends))
	}
	if !strings.Contains(string(friends[0]["info"]), `"status":"normal"`) {
		t.Fatalf("详细资料缺失：%s", friends[0]["info"])
	}
}

func mustJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
