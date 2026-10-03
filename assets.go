package main

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type assetStore struct {
	client    *HTTPClient
	headers   http.Header
	root      string
	cache     map[string]string
	downloads int
	progress  func(format string, args ...any)
}

func (s *assetStore) progressf(format string, args ...any) {
	if s.progress != nil {
		s.progress(format, args...)
	}
}

// DownloadAlbumAssets 下载相册依赖的远程资源，并把 JSON 中的地址替换为相对路径。
// progress 为可选回调，用于进度展示。
func DownloadAlbumAssets(ctx context.Context, client *HTTPClient, headers http.Header, root string, progress func(string, ...any), profile map[string]json.RawMessage, questions []json.RawMessage, memories []json.RawMessage, friends []map[string]json.RawMessage) error {
	store := &assetStore{client: client, headers: headers, root: root, cache: make(map[string]string), progress: progress}
	if err := store.profile(ctx, profile); err != nil {
		return err
	}
	for index, rawQuestion := range questions {
		var question map[string]json.RawMessage
		if err := json.Unmarshal(rawQuestion, &question); err != nil {
			return fmt.Errorf("解析第 %d 个黑板墙问题资源失败：%w", index, err)
		}
		if err := store.questionMedia(ctx, question, index); err != nil {
			return err
		}
		encoded, err := json.Marshal(question)
		if err != nil {
			return fmt.Errorf("编码第 %d 个黑板墙问题资源失败：%w", index, err)
		}
		questions[index] = encoded
	}
	for index, rawMemory := range memories {
		var memory map[string]json.RawMessage
		if err := json.Unmarshal(rawMemory, &memory); err != nil {
			return fmt.Errorf("解析第 %d 条动态资源失败：%w", index, err)
		}
		media, err := store.mediaList(ctx, memory["images"], filepath.Join("assets", "images", "memories"), fmt.Sprintf("memory-%d", index))
		if err != nil {
			return err
		}
		if media != nil {
			memory["images"] = media
		}
		comments, err := store.comments(ctx, memory["comments"], index)
		if err != nil {
			return err
		}
		if comments != nil {
			memory["comments"] = comments
		}
		encoded, err := json.Marshal(memory)
		if err != nil {
			return fmt.Errorf("编码第 %d 条动态资源失败：%w", index, err)
		}
		memories[index] = encoded
	}
	for index, friend := range friends {
		info := rawObject(friend["info"])
		directProfile := len(info) == 0
		if directProfile {
			info = friend
		}
		if err := store.profile(ctx, info); err != nil {
			return fmt.Errorf("下载第 %d 位好友头像失败：%w", index, err)
		}
		if directProfile {
			friends[index] = info
		} else {
			friend["info"], _ = json.Marshal(info)
		}
		encoded, err := json.Marshal(friend)
		if err != nil {
			return fmt.Errorf("编码第 %d 位好友资源失败：%w", index, err)
		}
		friends[index] = mapRaw(encoded)
	}
	return nil
}

// questionMedia 下载黑板墙问题及其回答的配图。
func (s *assetStore) questionMedia(ctx context.Context, question map[string]json.RawMessage, index int) error {
	media, err := s.mediaList(ctx, question["images"], filepath.Join("assets", "images", "questions"), fmt.Sprintf("question-%d", index))
	if err != nil {
		return fmt.Errorf("下载第 %d 个黑板墙问题配图失败：%w", index, err)
	}
	if media != nil {
		question["images"] = media
	}
	var answers []map[string]json.RawMessage
	if json.Unmarshal(question["answers"], &answers) != nil {
		return nil
	}
	for answerIndex, answer := range answers {
		media, err := s.mediaList(ctx, answer["images"], filepath.Join("assets", "images", "questions"), fmt.Sprintf("question-%d-answer-%d", index, answerIndex))
		if err != nil {
			return fmt.Errorf("下载第 %d 个黑板墙问题的回答配图失败：%w", index, err)
		}
		if media != nil {
			answer["images"] = media
		}
	}
	question["answers"], err = json.Marshal(answers)
	if err != nil {
		return fmt.Errorf("编码第 %d 个黑板墙问题的回答失败：%w", index, err)
	}
	return nil
}

func (s *assetStore) profile(ctx context.Context, profile map[string]json.RawMessage) error {
	if profile == nil {
		return nil
	}
	for _, key := range []string{"avatar", "accessory"} {
		url := rawString(profile, key)
		if url == "" || !strings.HasPrefix(url, "http") {
			continue
		}
		prefix := key
		if id := rawString(profile, "id"); id != "" {
			prefix = id + "-" + key
		}
		relative, err := s.download(ctx, url, filepath.Join("assets", "images", "avatar"), prefix)
		if err != nil {
			return err
		}
		profile[key] = json.RawMessage(strconvQuote(relative))
	}
	return nil
}

func (s *assetStore) mediaList(ctx context.Context, raw json.RawMessage, directory, prefix string) (json.RawMessage, error) {
	var media []map[string]json.RawMessage
	if json.Unmarshal(raw, &media) != nil {
		return nil, nil
	}
	for index, item := range media {
		url := rawString(item, "url")
		if url == "" || !strings.HasPrefix(url, "http") {
			continue
		}
		kind := rawString(item, "type")
		itemDirectory := directory
		if kind == "audio" {
			itemDirectory = filepath.Join("assets", "audio")
		}
		relative, err := s.download(ctx, url, itemDirectory, fmt.Sprintf("%s-%d", prefix, index))
		if err != nil {
			return nil, err
		}
		item["url"] = json.RawMessage(strconvQuote(relative))
		media[index] = item
	}
	encoded, err := json.Marshal(media)
	if err != nil {
		return nil, fmt.Errorf("编码媒体资源失败：%w", err)
	}
	return encoded, nil
}

func (s *assetStore) comments(ctx context.Context, raw json.RawMessage, index int) (json.RawMessage, error) {
	var comments []map[string]json.RawMessage
	if json.Unmarshal(raw, &comments) != nil {
		return nil, nil
	}
	for commentIndex, comment := range comments {
		for _, userKey := range []string{"user", "to_user"} {
			user := rawObject(comment[userKey])
			if err := s.profile(ctx, user); err != nil {
				return nil, fmt.Errorf("下载第 %d 条动态评论用户资源失败：%w", index, err)
			}
			encoded, _ := json.Marshal(user)
			comment[userKey] = encoded
		}
		toComment := rawObject(comment["to_comment"])
		toCommentUser := rawObject(toComment["user"])
		if err := s.profile(ctx, toCommentUser); err != nil {
			return nil, fmt.Errorf("下载第 %d 条动态评论回复用户资源失败：%w", index, err)
		}
		if len(toCommentUser) > 0 {
			toComment["user"], _ = json.Marshal(toCommentUser)
			comment["to_comment"], _ = json.Marshal(toComment)
		}
		comments[commentIndex] = comment
	}
	encoded, err := json.Marshal(comments)
	if err != nil {
		return nil, fmt.Errorf("编码第 %d 条动态评论资源失败：%w", index, err)
	}
	return encoded, nil
}

func (s *assetStore) download(ctx context.Context, remoteURL, directory, prefix string) (string, error) {
	if relative, ok := s.cache[remoteURL]; ok {
		return relative, nil
	}
	data, contentType, err := s.client.Download(ctx, remoteURL, s.headers)
	if err != nil {
		return "", err
	}
	extension := resourceExtension(remoteURL, contentType)
	filename := filepath.Join(directory, prefix+extension)
	absolute := filepath.Join(s.root, filename)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", fmt.Errorf("创建资源目录失败：%w", err)
	}
	if err := os.WriteFile(absolute, data, 0o600); err != nil {
		return "", fmt.Errorf("保存资源 %s 失败：%w", remoteURL, err)
	}
	relative := filepath.ToSlash(filename)
	s.cache[remoteURL] = relative
	s.downloads++
	s.progressf("已下载 %d 个资源：%s", s.downloads, relative)
	return relative, nil
}

func resourceExtension(remoteURL, contentType string) string {
	if extension := path.Ext(strings.Split(remoteURL, "?")[0]); extension != "" && len(extension) <= 5 {
		return extension
	}
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		switch mediaType {
		case "audio/mpeg":
			return ".mp3"
		case "audio/mp4":
			return ".m4a"
		case "image/jpeg":
			return ".jpg"
		case "image/png":
			return ".png"
		}
	}
	return ".bin"
}

func strconvQuote(value string) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func mapRaw(data []byte) map[string]json.RawMessage {
	var result map[string]json.RawMessage
	_ = json.Unmarshal(data, &result)
	return result
}
