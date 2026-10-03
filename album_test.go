package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderAlbumUsesCleanedData(t *testing.T) {
	normal := []map[string]json.RawMessage{{
		"title":   json.RawMessage(`"标题"`),
		"content": json.RawMessage(`"普通动态"`),
		"images":  json.RawMessage(`[]`),
		"time":    json.RawMessage(`"2024-06-18T00:00:00+08:00"`),
	}}
	blackboard := []map[string]json.RawMessage{{
		"question":  json.RawMessage(`"问题"`),
		"my_answer": json.RawMessage(`"回答"`),
		"images":    json.RawMessage(`[]`),
		"time":      json.RawMessage(`"2023-01-01"`),
	}}
	friends := []map[string]json.RawMessage{{
		"id":   json.RawMessage(`"1"`),
		"name": json.RawMessage(`"朋友"`),
		"info": json.RawMessage(`{"status":"OK"}`),
	}}
	data, err := renderAlbum(nil, nil, nil, "2026-10-03T00:00:00Z", normal, blackboard, friends, true, true)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	for _, value := range []string{"普通动态", "黑板墙问题：问题", "朋友", "2 0 2 4", "共 1 位 · 注销/异常 0 位 · 男 0 位 · 女 0 位"} {
		if !strings.Contains(page, value) {
			t.Fatalf("相册缺少 %q", value)
		}
	}
	if strings.Contains(page, "FEED_START") || strings.Contains(page, "FRIENDS_START") || strings.Contains(page, "cdn.jsdelivr") {
		t.Fatal("相册仍包含未替换标记或外部 Alpine 依赖")
	}
}
