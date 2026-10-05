package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultOutputDir = "summer_crawl_output"

const (
	requestDelayMin = 100 * time.Millisecond
	requestDelayMax = 300 * time.Millisecond
)

func main() {
	if err := runCLI(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func runCLI(args []string, input io.Reader, output io.Writer) error {
	if len(args) < 1 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("用法：./summer_crawl apikey [-user 用户ID] [-sample 数量] [-concurrency 并行数] [-output-dir 输出目录]")
	}
	authorization := args[0]
	outputDir, sample, concurrency, userID, err := parseCLIOptions(args[1:])
	if err != nil {
		return err
	}
	var keepBlackboard, keepFriends bool
	if userID == "" {
		reader := bufio.NewReader(input)
		keepBlackboard, err = askYesNo(reader, output, "是否保留黑板墙动态？", true)
		if err != nil {
			return err
		}
		keepFriends, err = askYesNo(reader, output, "是否保留好友信息？", true)
		if err != nil {
			return err
		}
	}

	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return fmt.Errorf("创建输出目录失败：%w", err)
	}
	capturedAt := time.Now().UTC()
	client, err := NewHTTPClient(20*time.Second, 5, 5*time.Second, log.New(output, "", log.LstdFlags))
	if err != nil {
		return err
	}
	client.SetRandomDelay(requestDelayMin, requestDelayMax)
	printer := newProgressPrinter(output)
	crawler := &Crawler{
		Client:        client,
		BaseURL:       activeBaseURL,
		ActivityLimit: 200,
		MaxActivities: sample,
		Progress:      printer.step,
		Headers: http.Header{
			"Authorization":   []string{authorization},
			"User-Agent":      []string{"okhttp/4.12.0"},
			"Accept-Encoding": []string{"gzip"},
		},
	}
	if userID != "" {
		fmt.Fprintf(output, "抓取用户 %s 的动态……\n", userID)
		memories, err := crawler.FetchUserMemories(context.Background(), userID)
		printer.end()
		if err != nil {
			return err
		}
		const outputFilename = "user_memories_for_llm.txt"
		if err := WriteUserMemoriesText(filepath.Join(outputDir, outputFilename), userID, memories); err != nil {
			return err
		}
		fmt.Fprintf(output, "完成：%d 条动态，文本文件：%s\n", len(memories), filepath.Join(outputDir, outputFilename))
		return nil
	}

	fmt.Fprintln(output, "抓取个人资料……")
	profile, err := crawler.FetchProfile(context.Background())
	if err != nil {
		return err
	}
	fmt.Fprintln(output, "抓取黑板墙提问……")
	questions, err := crawler.FetchQuestionBoards(context.Background())
	if err != nil {
		return err
	}
	if sample > 0 && len(questions) > sample {
		questions = questions[:sample]
	}
	questions, err = crawler.EnrichQuestionBoards(context.Background(), questions)
	printer.end()
	if err != nil {
		return err
	}
	var paper map[string]json.RawMessage
	if paperID := rawString(profile, "paper_id"); paperID != "" {
		fmt.Fprintln(output, "抓取交友问卷……")
		paper, err = crawler.FetchPaper(context.Background(), paperID)
		if err != nil {
			return err
		}
	}
	fmt.Fprintln(output, "抓取动态……")
	memories, err := crawler.FetchMemories(context.Background())
	printer.end()
	if err != nil {
		return err
	}
	memories, err = crawler.EnrichMemories(context.Background(), memories)
	printer.end()
	if err != nil {
		return err
	}
	friends := []map[string]json.RawMessage(nil)
	if keepFriends {
		fmt.Fprintln(output, "抓取好友列表……")
		friends, err = crawler.FetchFriends(context.Background())
		printer.end()
		if err != nil {
			return err
		}
		fmt.Fprintln(output, "抓取好友详细资料……")
		if err := crawler.EnrichFriendProfiles(context.Background(), friends, concurrency); err != nil {
			return err
		}
		printer.end()
	}
	fmt.Fprintln(output, "下载头像、图片和录音……")
	if err := DownloadAlbumAssets(context.Background(), client, crawler.Headers, outputDir, printer.step, concurrency, profile, questions, memories, friends); err != nil {
		return err
	}
	printer.end()
	if err := writeJSON(filepath.Join(outputDir, "profile.json"), profile); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outputDir, "capture.json"), map[string]string{"captured_at": capturedAt.Format(time.RFC3339)}); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outputDir, "question_boards.json"), questions); err != nil {
		return err
	}
	if paper != nil {
		if err := writeJSON(filepath.Join(outputDir, "paper.json"), paper); err != nil {
			return err
		}
	}
	if err := writeJSON(filepath.Join(outputDir, "memories.json"), memories); err != nil {
		return err
	}
	if keepFriends {
		if err := writeJSON(filepath.Join(outputDir, "friends.json"), friends); err != nil {
			return err
		}
	}
	blackboard, normal, refinedBlackboard, refinedNormal, err := CleanMemories(memories)
	if err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outputDir, "normal_memories.json"), normal); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outputDir, "refined_normal_memories.json"), refinedNormal); err != nil {
		return err
	}
	if err := WriteMemoriesMarkdown(filepath.Join(outputDir, "memories_for_llm.md"), refinedNormal); err != nil {
		return err
	}
	if !keepBlackboard {
		blackboard = nil
		refinedBlackboard = nil
	} else {
		if err := writeJSON(filepath.Join(outputDir, "blackboard_memories.json"), blackboard); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(outputDir, "refined_blackboard_memories.json"), refinedBlackboard); err != nil {
			return err
		}
	}

	album, err := renderAlbum(profile, paper, questions, capturedAt.Format(time.RFC3339), refinedNormal, refinedBlackboard, friends, keepBlackboard, keepFriends)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "album.html"), album, 0o600); err != nil {
		return fmt.Errorf("写入 HTML 相册失败：%w", err)
	}
	fmt.Fprintf(output, "完成：%d 条普通动态，%d 条黑板墙动态，输出目录：%s\n", len(normal), len(blackboard), outputDir)
	return nil
}

func parseCLIOptions(args []string) (string, int, int, string, error) {
	outputDir := defaultOutputDir
	sample := 0
	concurrency := 4
	userID := ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "-output-dir":
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
				return "", 0, 0, "", fmt.Errorf("-output-dir 后必须提供目录")
			}
			outputDir = args[index+1]
			index++
		case "-sample":
			if index+1 >= len(args) {
				return "", 0, 0, "", fmt.Errorf("-sample 后必须提供正整数")
			}
			if _, err := fmt.Sscanf(args[index+1], "%d", &sample); err != nil || sample < 1 {
				return "", 0, 0, "", fmt.Errorf("-sample 后必须提供正整数")
			}
			index++
		case "-concurrency":
			if index+1 >= len(args) {
				return "", 0, 0, "", fmt.Errorf("-concurrency 后必须提供正整数")
			}
			if _, err := fmt.Sscanf(args[index+1], "%d", &concurrency); err != nil || concurrency < 1 || concurrency > 64 {
				return "", 0, 0, "", fmt.Errorf("-concurrency 后必须提供 1 到 64 的整数")
			}
			index++
		case "-user":
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
				return "", 0, 0, "", fmt.Errorf("-user 后必须提供用户 ID")
			}
			userID = strings.TrimSpace(args[index+1])
			index++
		default:
			return "", 0, 0, "", fmt.Errorf("未知参数 %s；用法：./summer_crawl apikey [-user 用户ID] [-sample 数量] [-concurrency 并行数] [-output-dir 输出目录]", args[index])
		}
	}
	if userID != "" && sample > 0 {
		return "", 0, 0, "", fmt.Errorf("-user 会抓取全部动态，不能与 -sample 同时使用")
	}
	return outputDir, sample, concurrency, userID, nil
}

func askYesNo(input *bufio.Reader, output io.Writer, question string, defaultValue bool) (bool, error) {
	choice := "Y/n"
	if !defaultValue {
		choice = "y/N"
	}
	fmt.Fprintf(output, "%s [%s] ", question, choice)
	line, err := input.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("读取交互配置失败：%w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "是":
		return true, nil
	case "n", "no", "否":
		return false, nil
	default:
		return defaultValue, nil
	}
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("编码文件 %s 失败：%w", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("写入文件 %s 失败：%w", path, err)
	}
	return nil
}

// progressPrinter 在终端中用单行滚动刷新进度；非终端输出时退化为逐行打印。
// 并行任务会并发调用，内部加锁保证输出不串行交错。
type progressPrinter struct {
	w     io.Writer
	mu    sync.Mutex
	isTTY bool
}

func newProgressPrinter(w io.Writer) *progressPrinter {
	printer := &progressPrinter{w: w}
	if file, ok := w.(*os.File); ok {
		if info, err := file.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			printer.isTTY = true
		}
	}
	return printer
}

func (p *progressPrinter) step(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isTTY {
		fmt.Fprintf(p.w, "\r\033[K"+format, args...)
		return
	}
	fmt.Fprintf(p.w, format+"\n", args...)
}

func (p *progressPrinter) end() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.isTTY {
		fmt.Fprint(p.w, "\r\033[K")
	}
	fmt.Fprintln(p.w)
}
