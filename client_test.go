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
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
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
		t.Fatalf("result=%v attempts=%d", result, attempts.Load())
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
		t.Fatalf("memories=%s offsets=%v", mustJSON(memories), offsets)
	}
}

func TestCrawlerFetchFriendsAddsProfiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token" {
			t.Errorf("authorization header = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/user/relationships":
			_, _ = w.Write([]byte(`{"friends":[{"id":"42","name":"Ada"}]}`))
		case "/users/42":
			_, _ = w.Write([]byte(`{"status":"OK"}`))
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
	if len(friends) != 1 || string(friends[0]["info"]) != `{"status":"OK"}` {
		t.Fatalf("friends=%s", mustJSON(friends))
	}
}

func mustJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
