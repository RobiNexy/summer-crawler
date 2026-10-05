package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanMemoriesSplitsAndRefines(t *testing.T) {
	memories := []json.RawMessage{
		json.RawMessage(`{"question":{"content":"问题"},"content":"回答","images":[],"created_at":"今天"}`),
		json.RawMessage(`{"title":"标题","content":"正文","images":["图"],"created_at":"昨天","outchain":"https://example.com","outchain_title":"链接标题"}`),
	}
	blackboard, normal, refinedBlackboard, refinedNormal, err := CleanMemories(memories)
	if err != nil {
		t.Fatal(err)
	}
	if len(blackboard) != 1 || len(normal) != 1 || len(refinedBlackboard) != 1 || len(refinedNormal) != 1 {
		t.Fatalf("拆分结果异常：黑板墙=%d 普通=%d 提炼黑板墙=%d 提炼普通=%d", len(blackboard), len(normal), len(refinedBlackboard), len(refinedNormal))
	}
	if string(refinedBlackboard[0]["question"]) != `"问题"` || string(refinedBlackboard[0]["my_answer"]) != `"回答"` {
		t.Fatalf("黑板墙提炼结果异常：%s", mustJSON(refinedBlackboard[0]))
	}
	if string(refinedNormal[0]["outlink"]) != `{"author":null,"image":null,"title":"链接标题","url":"https://example.com"}` {
		t.Fatalf("外链提炼结果异常：%s", mustJSON(refinedNormal[0]))
	}
}

func TestWriteMemoriesMarkdown(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "memories_for_llm.md")
	memories := []map[string]json.RawMessage{{
		"time":    json.RawMessage(`"2025-01-01"`),
		"content": json.RawMessage(`"正文"`),
	}}
	if err := WriteMemoriesMarkdown(path, memories); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "## 我在某社交软件上发过的所有动态\n\n### 时间:2025-01-01\n\n正文\n\n" {
		t.Fatalf("Markdown 内容异常：%s", data)
	}
}

func TestWriteUserMemoriesText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user_memories_for_llm.txt")
	memories := []json.RawMessage{
		json.RawMessage(`{"title":"周末","content":"去公园散步","created_at":"2025-01-01","images":[{"url":"https://example.com/image.jpg"}]}`),
		json.RawMessage(`{"content":"补充动态","time":"2025-01-02"}`),
	}
	if err := WriteUserMemoriesText(path, "user-42", memories); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"用户 user-42 的动态", "共 2 条", "标题：周末", "去公园散步", "2025-01-02", "补充动态"} {
		if !strings.Contains(text, want) {
			t.Errorf("输出文本缺少 %q：%s", want, text)
		}
	}
	if strings.Contains(text, "example.com") || strings.Contains(text, "图片") {
		t.Fatalf("纯文本不应包含媒体链接：%s", text)
	}
}
