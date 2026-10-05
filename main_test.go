package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCLIUserModeWritesOnlyPlainText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/user-42/activities" {
			t.Errorf("请求了意料之外的接口：%s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("offset") == "0" {
			_, _ = w.Write([]byte(`[{"content":"好友动态","created_at":"2025-01-01","images":[{"url":"https://example.com/image.jpg"}]}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	previousBaseURL := activeBaseURL
	activeBaseURL = server.URL
	defer func() { activeBaseURL = previousBaseURL }()

	outputDir := t.TempDir()
	var output bytes.Buffer
	err := runCLI([]string{"test-token", "-user", "user-42", "-output-dir", outputDir}, bytes.NewBuffer(nil), &output)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "user_memories_for_llm.txt" {
		t.Fatalf("输出目录内容不符合预期：%v", entries)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("好友动态")) || bytes.Contains(data, []byte("example.com")) {
		t.Fatalf("文本内容异常：%s", data)
	}
}
