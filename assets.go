package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// assetJob 描述一个待下载的远程资源；相同 URL 只下载一次。
type assetJob struct {
	remoteURL string
	directory string
	prefix    string
	relative  string
}

// pendingApply 在下载完成后把本地路径写回数据结构。
type pendingApply struct {
	job   *assetJob
	apply func(relative string)
}

type assetStore struct {
	client     *HTTPClient
	headers    http.Header
	root       string
	progress   func(format string, args ...any)
	jobs       []*assetJob
	index      map[string]*assetJob
	applies    []pendingApply
	finalizers []func()
	downloaded atomic.Int64
}

func (s *assetStore) progressf(format string, args ...any) {
	if s.progress != nil {
		s.progress(format, args...)
	}
}

// DownloadAlbumAssets 并行下载相册依赖的远程资源，并把 JSON 中的地址替换为相对路径。
// concurrency 限制同时下载的数量；progress 为可选进度回调。
func DownloadAlbumAssets(ctx context.Context, client *HTTPClient, headers http.Header, root string, progress func(string, ...any), concurrency int, profile map[string]json.RawMessage, questions []json.RawMessage, memories []json.RawMessage, friends []map[string]json.RawMessage) error {
	store := &assetStore{
		client:   client,
		headers:  headers,
		root:     root,
		progress: progress,
		index:    make(map[string]*assetJob),
	}
	store.collectProfile(profile)
	for index, rawQuestion := range questions {
		var question map[string]json.RawMessage
		if err := json.Unmarshal(rawQuestion, &question); err != nil {
			return fmt.Errorf("解析第 %d 个黑板墙问题资源失败：%w", index, err)
		}
		var savers []func()
		if save := store.collectMedia(question["images"], filepath.Join("assets", "images", "questions"), fmt.Sprintf("question-%d", index)); save != nil {
			savers = append(savers, func() { question["images"] = save() })
		}
		var answers []map[string]json.RawMessage
		if json.Unmarshal(question["answers"], &answers) == nil {
			var answerSavers []func()
			for answerIndex, answer := range answers {
				prefix := fmt.Sprintf("question-%d-answer-%d", index, answerIndex)
				if save := store.collectMedia(answer["images"], filepath.Join("assets", "images", "questions"), prefix); save != nil {
					target := answer
					answerSavers = append(answerSavers, func() { target["images"] = save() })
				}
			}
			if len(answerSavers) > 0 {
				savers = append(savers, func() {
					for _, save := range answerSavers {
						save()
					}
					question["answers"], _ = json.Marshal(answers)
				})
			}
		}
		if len(savers) > 0 {
			captured := question
			store.finalizers = append(store.finalizers, func() {
				for _, save := range savers {
					save()
				}
				questions[index], _ = json.Marshal(captured)
			})
		}
	}
	for index, rawMemory := range memories {
		var memory map[string]json.RawMessage
		if err := json.Unmarshal(rawMemory, &memory); err != nil {
			return fmt.Errorf("解析第 %d 条动态资源失败：%w", index, err)
		}
		var savers []func()
		if save := store.collectMedia(memory["images"], filepath.Join("assets", "images", "memories"), fmt.Sprintf("memory-%d", index)); save != nil {
			savers = append(savers, func() { memory["images"] = save() })
		}
		if save := store.collectComments(memory["comments"]); save != nil {
			savers = append(savers, func() { memory["comments"] = save() })
		}
		if len(savers) > 0 {
			captured := memory
			store.finalizers = append(store.finalizers, func() {
				for _, save := range savers {
					save()
				}
				memories[index], _ = json.Marshal(captured)
			})
		}
	}
	for index, friend := range friends {
		info := rawObject(friend["info"])
		directProfile := len(info) == 0
		if directProfile {
			info = friend
		}
		if !store.collectProfileFields(info) {
			continue
		}
		friendMap := friend
		store.finalizers = append(store.finalizers, func() {
			if directProfile {
				friends[index] = info
				return
			}
			friendMap["info"], _ = json.Marshal(info)
			encoded, err := json.Marshal(friendMap)
			if err == nil {
				friends[index] = mapRaw(encoded)
			}
		})
	}

	if err := store.downloadAll(ctx, concurrency); err != nil {
		return err
	}
	for _, pending := range store.applies {
		if pending.job.relative != "" {
			pending.apply(pending.job.relative)
		}
	}
	for _, finalize := range store.finalizers {
		finalize()
	}
	return nil
}

func (s *assetStore) addJob(remoteURL, directory, prefix string) *assetJob {
	if job, ok := s.index[remoteURL]; ok {
		return job
	}
	job := &assetJob{remoteURL: remoteURL, directory: directory, prefix: prefix}
	s.jobs = append(s.jobs, job)
	s.index[remoteURL] = job
	return job
}

func (s *assetStore) schedule(job *assetJob, apply func(relative string)) {
	s.applies = append(s.applies, pendingApply{job: job, apply: apply})
}

// collectProfileFields 收集 profile 中头像和挂件的任务；返回是否有改动。
// 任务应用后直接原地写回 map。
func (s *assetStore) collectProfileFields(profile map[string]json.RawMessage) bool {
	if profile == nil {
		return false
	}
	changed := false
	for _, key := range []string{"avatar", "accessory"} {
		url := rawString(profile, key)
		if !isRemoteURL(url) {
			continue
		}
		prefix := key
		if id := rawString(profile, "id"); id != "" {
			prefix = id + "-" + key
		}
		job := s.addJob(url, filepath.Join("assets", "images", "avatar"), prefix)
		s.schedule(job, func(relative string) { profile[key] = strconvQuote(relative) })
		changed = true
	}
	return changed
}

func (s *assetStore) collectProfile(profile map[string]json.RawMessage) {
	s.collectProfileFields(profile)
}

// collectMedia 收集一个 images 数组中的远程资源任务，返回写回函数（未改动时返回 nil）。
func (s *assetStore) collectMedia(raw json.RawMessage, directory, prefix string) func() json.RawMessage {
	var media []map[string]json.RawMessage
	if json.Unmarshal(raw, &media) != nil {
		return nil
	}
	changed := false
	for index, item := range media {
		url := rawString(item, "url")
		if !isRemoteURL(url) {
			continue
		}
		itemDirectory := directory
		if rawString(item, "type") == "audio" {
			itemDirectory = filepath.Join("assets", "audio")
		}
		job := s.addJob(url, itemDirectory, fmt.Sprintf("%s-%d", prefix, index))
		target := item
		s.schedule(job, func(relative string) { target["url"] = strconvQuote(relative) })
		changed = true
	}
	if !changed {
		return nil
	}
	return func() json.RawMessage {
		encoded, err := json.Marshal(media)
		if err != nil {
			return raw
		}
		return encoded
	}
}

// collectComments 收集评论用户头像任务，返回写回函数（未改动时返回 nil）。
func (s *assetStore) collectComments(raw json.RawMessage) func() json.RawMessage {
	var comments []map[string]json.RawMessage
	if json.Unmarshal(raw, &comments) != nil {
		return nil
	}
	var commentSavers []func()
	collectUser := func(holder map[string]json.RawMessage, key string) {
		user := rawObject(holder[key])
		if len(user) == 0 {
			return
		}
		avatar := rawString(user, "avatar")
		if !isRemoteURL(avatar) {
			return
		}
		prefix := "avatar"
		if id := rawString(user, "id"); id != "" {
			prefix = id + "-avatar"
		}
		job := s.addJob(avatar, filepath.Join("assets", "images", "avatar"), prefix)
		s.schedule(job, func(relative string) { user["avatar"] = strconvQuote(relative) })
		commentSavers = append(commentSavers, func() { holder[key], _ = json.Marshal(user) })
	}
	for _, comment := range comments {
		collectUser(comment, "user")
		collectUser(comment, "to_user")
		toComment := rawObject(comment["to_comment"])
		if len(toComment) > 0 {
			collectUser(toComment, "user")
			commentSavers = append(commentSavers, func() { comment["to_comment"], _ = json.Marshal(toComment) })
		}
	}
	if len(commentSavers) == 0 {
		return nil
	}
	return func() json.RawMessage {
		for _, save := range commentSavers {
			save()
		}
		encoded, err := json.Marshal(comments)
		if err != nil {
			return raw
		}
		return encoded
	}
}

// downloadAll 并行下载全部任务；首个错误会取消其余任务。
func (s *assetStore) downloadAll(ctx context.Context, concurrency int) error {
	if len(s.jobs) == 0 {
		return nil
	}
	if concurrency < 1 {
		concurrency = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
		sem      = make(chan struct{}, concurrency)
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
		cancel()
	}
jobsLoop:
	for _, job := range s.jobs {
		select {
		case <-ctx.Done():
			break jobsLoop
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(job *assetJob) {
			defer wg.Done()
			defer func() { <-sem }()
			relative, err := s.download(ctx, job)
			if err != nil {
				if errors.Is(err, errRequestExhausted) {
					s.progressf("跳过下载失败的资源：%s", job.remoteURL)
					return
				}
				fail(err)
				return
			}
			job.relative = relative
			s.progressf("已下载 %d/%d 个资源：%s", s.downloaded.Add(1), len(s.jobs), relative)
		}(job)
	}
	wg.Wait()
	return firstErr
}

func (s *assetStore) download(ctx context.Context, job *assetJob) (string, error) {
	data, contentType, err := s.client.Download(ctx, job.remoteURL, s.headers)
	if err != nil {
		return "", err
	}
	extension := resourceExtension(job.remoteURL, contentType)
	filename := filepath.Join(job.directory, job.prefix+extension)
	absolute := filepath.Join(s.root, filename)
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", fmt.Errorf("创建资源目录失败：%w", err)
	}
	if err := os.WriteFile(absolute, data, 0o600); err != nil {
		return "", fmt.Errorf("保存资源 %s 失败：%w", job.remoteURL, err)
	}
	return filepath.ToSlash(filename), nil
}

func isRemoteURL(value string) bool {
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
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
