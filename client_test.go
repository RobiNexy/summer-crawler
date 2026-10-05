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

func TestHTTPClientRetryDelayDoubles(t *testing.T) {
	client := &HTTPClient{Delay: 5 * time.Second}
	for attempt, want := range []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second} {
		if got := client.retryDelay(attempt + 1); got != want {
			t.Errorf("第 %d 次失败后的等待时间 = %s，期望 %s", attempt+1, got, want)
		}
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

func TestCrawlerFetchUserMemoriesPaginatesOnlyUserActivities(t *testing.T) {
	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/user-42/activities" {
			t.Errorf("请求了意料之外的接口：%s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		offset := r.URL.Query().Get("offset")
		offsets = append(offsets, offset)
		switch offset {
		case "0":
			_, _ = w.Write([]byte(`[{"id":"1"},{"id":"2"}]`))
		case "2":
			_, _ = w.Write([]byte(`[{"id":"3"}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	defer server.Close()
	client, err := NewHTTPClient(time.Second, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	crawler := &Crawler{Client: client, BaseURL: server.URL, ActivityLimit: 2}
	memories, err := crawler.FetchUserMemories(context.Background(), "user-42")
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 3 || strings.Join(offsets, ",") != "0,2,3" {
		t.Fatalf("动态条数=%d offsets=%v", len(memories), offsets)
	}
}

func TestParseCLIOptionsUserMode(t *testing.T) {
	outputDir, sample, concurrency, userID, err := parseCLIOptions([]string{"-user", "user-42", "-output-dir", "out"})
	if err != nil {
		t.Fatal(err)
	}
	if outputDir != "out" || sample != 0 || concurrency != 4 || userID != "user-42" {
		t.Fatalf("解析结果：output=%q sample=%d concurrency=%d user=%q", outputDir, sample, concurrency, userID)
	}
	if _, _, _, _, err := parseCLIOptions([]string{"-user", "user-42", "-sample", "1"}); err == nil {
		t.Fatal("-user 和 -sample 同时使用时应报错")
	}
}

func TestCrawlerFetchFriendsAddsProfiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token" {
			t.Errorf("授权请求头 = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/user/friends":
			if r.URL.Query().Get("offset") != "0" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
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
	if err := crawler.EnrichFriendProfiles(context.Background(), friends, 2); err != nil {
		t.Fatal(err)
	}
	if len(friends) != 1 || string(friends[0]["nickname"]) != `"Ada"` {
		t.Fatalf("好友=%s", mustJSON(friends))
	}
	if !strings.Contains(string(friends[0]["info"]), `"status":"normal"`) {
		t.Fatalf("详细资料缺失：%s", friends[0]["info"])
	}
}

func TestCrawlerSkipsUnavailableFriendProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "临时故障", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client, err := NewHTTPClient(time.Second, 1, 0, log.New(&strings.Builder{}, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	crawler := &Crawler{Client: client, BaseURL: server.URL}
	friends := []map[string]json.RawMessage{{
		"id":       json.RawMessage(`"42"`),
		"nickname": json.RawMessage(`"Ada"`),
		"gender":   json.RawMessage(`2`),
	}}
	if err := crawler.EnrichFriendProfiles(context.Background(), friends, 1); err != nil {
		t.Fatalf("资料请求失败不应中断好友抓取：%v", err)
	}
	if string(friends[0]["nickname"]) != `"Ada"` {
		t.Fatalf("好友列表数据被丢弃：%s", mustJSON(friends))
	}
	if info := rawObject(friends[0]["info"]); len(info) != 0 {
		t.Fatalf("失败资料应使用空对象默认值：%s", friends[0]["info"])
	}
}

func mustJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
