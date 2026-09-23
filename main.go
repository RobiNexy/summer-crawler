package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	baseURL := flag.String("base-url", defaultBaseURL, "API root URL")
	authorization := flag.String("authorization", os.Getenv("SUMMER_AUTHORIZATION"), "authorization token; defaults to SUMMER_AUTHORIZATION")
	outputDir := flag.String("output-dir", ".", "directory for JSON output")
	friends := flag.Bool("friends", false, "also fetch friends and profiles")
	limit := flag.Int("limit", 200, "activities per page")
	flag.Parse()
	if *authorization == "" {
		log.Fatal("authorization is required: set SUMMER_AUTHORIZATION or pass -authorization")
	}

	client, err := NewHTTPClient(20*time.Second, 5, 3*time.Second, log.Default())
	if err != nil {
		log.Fatal(err)
	}
	crawler := &Crawler{
		Client:        client,
		BaseURL:       *baseURL,
		ActivityLimit: *limit,
		Headers: http.Header{
			"Authorization":   []string{*authorization},
			"User-Agent":      []string{"okhttp/4.12.0"},
			"Accept-Encoding": []string{"gzip"},
		},
	}

	ctx := context.Background()
	if err := os.MkdirAll(*outputDir, 0o700); err != nil {
		log.Fatalf("create output directory: %v", err)
	}
	memories, err := crawler.FetchMemories(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := writeJSON(filepath.Join(*outputDir, "memories.json"), memories); err != nil {
		log.Fatal(err)
	}
	log.Printf("saved %d activities", len(memories))

	if *friends {
		allFriends, err := crawler.FetchFriends(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if err := writeJSON(filepath.Join(*outputDir, "friends.json"), allFriends); err != nil {
			log.Fatal(err)
		}
		log.Printf("saved %d friends", len(allFriends))
	}
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
