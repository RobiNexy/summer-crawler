package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const defaultBaseURL = "https://imsummer.cn/api/v9"

// activeBaseURL 是 runCLI 实际使用的 API 根地址，测试时可替换。
var activeBaseURL = defaultBaseURL

// Crawler 抓取已授权用户的动态和好友资料。
// BaseURL 必须指向 API 根地址；Authorization 会作为请求头发送。
// Progress 为可选回调，每完成一个子步骤调用一次，用于终端进度展示。
type Crawler struct {
	Client        *HTTPClient
	BaseURL       string
	Headers       http.Header
	ActivityLimit int
	MaxActivities int
	Progress      func(format string, args ...any)
}

func (c *Crawler) progressf(format string, args ...any) {
	if c.Progress != nil {
		c.Progress(format, args...)
	}
}

// FetchMemories 返回所有动态分页结果，并在遇到首个空分页时停止。
// API 或解码出错时会返回错误，不会返回不完整结果。
func (c *Crawler) FetchMemories(ctx context.Context) ([]json.RawMessage, error) {
	if c.ActivityLimit < 1 {
		return nil, fmt.Errorf("每页动态数量必须大于 0")
	}
	var all []json.RawMessage
	for offset := 0; ; offset += c.ActivityLimit {
		endpoint, err := c.endpoint("user/activities")
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		query.Set("limit", strconv.Itoa(c.ActivityLimit))
		query.Set("offset", strconv.Itoa(offset))
		endpoint.RawQuery = query.Encode()

		var page []json.RawMessage
		if err := c.Client.GetJSON(ctx, endpoint.String(), c.Headers, &page); err != nil {
			return nil, fmt.Errorf("获取偏移量为 %d 的动态失败：%w", offset, err)
		}
		if len(page) == 0 {
			return all, nil
		}
		all = append(all, page...)
		c.progressf("已获取 %d 条动态", len(all))
		if c.MaxActivities > 0 && len(all) >= c.MaxActivities {
			return all[:c.MaxActivities], nil
		}
	}
}

// FetchProfile 获取当前用户的完整资料。
func (c *Crawler) FetchProfile(ctx context.Context) (map[string]json.RawMessage, error) {
	endpoint, err := c.endpoint("user")
	if err != nil {
		return nil, err
	}
	var profile map[string]json.RawMessage
	if err := c.Client.GetJSON(ctx, endpoint.String(), c.Headers, &profile); err != nil {
		return nil, fmt.Errorf("获取个人资料失败：%w", err)
	}
	return profile, nil
}

// FetchQuestionBoards 获取当前用户发布过的黑板墙问题。
func (c *Crawler) FetchQuestionBoards(ctx context.Context) ([]json.RawMessage, error) {
	return c.fetchPaged(ctx, "user/question_boards", "黑板墙问题")
}

// FetchQuestionAnswers 获取指定黑板墙问题收到的回答。
func (c *Crawler) FetchQuestionAnswers(ctx context.Context, questionID string) ([]json.RawMessage, error) {
	return c.fetchPaged(ctx, "questions/"+url.PathEscape(questionID)+"/answers?sort=top&comment_filter=all", "问题回答")
}

// FetchPaper 获取个人交友问卷。
func (c *Crawler) FetchPaper(ctx context.Context, paperID string) (map[string]json.RawMessage, error) {
	endpoint, err := c.endpoint("papers/" + url.PathEscape(paperID))
	if err != nil {
		return nil, err
	}
	var paper map[string]json.RawMessage
	if err := c.Client.GetJSON(ctx, endpoint.String(), c.Headers, &paper); err != nil {
		return nil, fmt.Errorf("获取交友问卷失败：%w", err)
	}
	return paper, nil
}

// EnrichQuestionBoards 将每个黑板墙问题的回答写入 answers 字段。
func (c *Crawler) EnrichQuestionBoards(ctx context.Context, questions []json.RawMessage) ([]json.RawMessage, error) {
	result := make([]json.RawMessage, 0, len(questions))
	for index, rawQuestion := range questions {
		c.progressf("抓取问题回答 %d/%d", index+1, len(questions))
		var question map[string]json.RawMessage
		if err := json.Unmarshal(rawQuestion, &question); err != nil {
			return nil, fmt.Errorf("解析第 %d 个黑板墙问题失败：%w", index, err)
		}
		id, err := rawID(question["id"])
		if err != nil {
			return nil, fmt.Errorf("第 %d 个黑板墙问题缺少 ID：%w", index, err)
		}
		answers, err := c.FetchQuestionAnswers(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("获取问题 %s 的回答失败：%w", id, err)
		}
		question["answers"], err = json.Marshal(answers)
		if err != nil {
			return nil, fmt.Errorf("编码问题 %s 的回答失败：%w", id, err)
		}
		encoded, err := json.Marshal(question)
		if err != nil {
			return nil, fmt.Errorf("编码第 %d 个黑板墙问题失败：%w", index, err)
		}
		result = append(result, encoded)
	}
	return result, nil
}

// FetchActivityComments 获取指定动态的评论。
func (c *Crawler) FetchActivityComments(ctx context.Context, activityID string) ([]json.RawMessage, error) {
	return c.fetchPaged(ctx, "activities/"+url.PathEscape(activityID)+"/comments?sort=normal", "动态评论")
}

// EnrichMemories 将评论写入对应动态的 comments 字段。
func (c *Crawler) EnrichMemories(ctx context.Context, memories []json.RawMessage) ([]json.RawMessage, error) {
	withComments := 0
	for _, rawMemory := range memories {
		var probe struct {
			CommentsCount int `json:"comments_count"`
		}
		if json.Unmarshal(rawMemory, &probe) == nil && probe.CommentsCount > 0 {
			withComments++
		}
	}
	result := make([]json.RawMessage, 0, len(memories))
	fetched := 0
	for index, rawMemory := range memories {
		var memory map[string]json.RawMessage
		if err := json.Unmarshal(rawMemory, &memory); err != nil {
			return nil, fmt.Errorf("解析第 %d 条动态失败：%w", index, err)
		}
		id, err := rawID(memory["id"])
		if err != nil {
			return nil, fmt.Errorf("第 %d 条动态缺少 ID：%w", index, err)
		}
		commentsCount := rawNumber(memory["comments_count"])
		if commentsCount > 0 {
			fetched++
			c.progressf("抓取评论 %d/%d", fetched, withComments)
			comments, err := c.FetchActivityComments(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("获取动态 %s 的评论失败：%w", id, err)
			}
			memory["comments"], err = json.Marshal(comments)
			if err != nil {
				return nil, fmt.Errorf("编码动态 %s 的评论失败：%w", id, err)
			}
		}
		encoded, err := json.Marshal(memory)
		if err != nil {
			return nil, fmt.Errorf("编码第 %d 条动态失败：%w", index, err)
		}
		result = append(result, encoded)
	}
	return result, nil
}

func (c *Crawler) fetchPaged(ctx context.Context, path, label string) ([]json.RawMessage, error) {
	const pageSize = 20
	var all []json.RawMessage
	for offset := 0; ; offset += pageSize {
		endpoint, err := c.endpoint(path)
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		query.Set("limit", strconv.Itoa(pageSize))
		query.Set("offset", strconv.Itoa(offset))
		endpoint.RawQuery = query.Encode()
		var page []json.RawMessage
		if err := c.Client.GetJSON(ctx, endpoint.String(), c.Headers, &page); err != nil {
			return nil, fmt.Errorf("获取%s（偏移量 %d）失败：%w", label, offset, err)
		}
		if len(page) == 0 {
			return all, nil
		}
		all = append(all, page...)
		c.progressf("已获取 %d 条%s", len(all), label)
		if c.MaxActivities > 0 && len(all) >= c.MaxActivities {
			return all[:c.MaxActivities], nil
		}
		if len(page) < pageSize {
			return all, nil
		}
	}
}

// FetchFriends 返回好友关系条目，并将每位好友的资料放入其 "info" 字段。
func (c *Crawler) FetchFriends(ctx context.Context) ([]map[string]json.RawMessage, error) {
	const pageSize = 20
	var friends []map[string]json.RawMessage
	for offset := 0; ; offset += pageSize {
		endpoint, err := c.endpoint("user/friends")
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		query.Set("q", "")
		query.Set("limit", strconv.Itoa(pageSize))
		query.Set("offset", strconv.Itoa(offset))
		endpoint.RawQuery = query.Encode()
		var page []map[string]json.RawMessage
		if err := c.Client.GetJSON(ctx, endpoint.String(), c.Headers, &page); err != nil {
			return nil, fmt.Errorf("获取好友（偏移量 %d）失败：%w", offset, err)
		}
		if len(page) == 0 {
			return friends, nil
		}
		friends = append(friends, page...)
		c.progressf("已获取 %d 位好友", len(friends))
		if c.MaxActivities > 0 && len(friends) >= c.MaxActivities {
			return friends[:c.MaxActivities], nil
		}
		if len(page) < pageSize {
			return friends, nil
		}
	}
}

// FetchFriendProfile 获取单个好友的详细资料（含 status 等列表接口没有的字段）。
func (c *Crawler) FetchFriendProfile(ctx context.Context, userID string) (map[string]json.RawMessage, error) {
	endpoint, err := c.endpoint("users/" + url.PathEscape(userID))
	if err != nil {
		return nil, err
	}
	var profile map[string]json.RawMessage
	if err := c.Client.GetJSON(ctx, endpoint.String(), c.Headers, &profile); err != nil {
		return nil, fmt.Errorf("获取好友 %s 的详细资料失败：%w", userID, err)
	}
	return profile, nil
}

// EnrichFriendProfiles 用详情接口补齐每位好友的资料，并写入 info 字段。
func (c *Crawler) EnrichFriendProfiles(ctx context.Context, friends []map[string]json.RawMessage) error {
	for index, friend := range friends {
		c.progressf("抓取好友资料 %d/%d", index+1, len(friends))
		id, err := rawID(friend["id"])
		if err != nil {
			return fmt.Errorf("第 %d 位好友：%w", index, err)
		}
		profile, err := c.FetchFriendProfile(ctx, id)
		if err != nil {
			return err
		}
		friend["info"], err = json.Marshal(profile)
		if err != nil {
			return fmt.Errorf("编码好友 %s 的资料失败：%w", id, err)
		}
	}
	return nil
}

func (c *Crawler) endpoint(path string) (*url.URL, error) {
	base := strings.TrimRight(c.BaseURL, "/") + "/"
	endpoint, err := url.Parse(base + strings.TrimLeft(path, "/"))
	if err != nil {
		return nil, fmt.Errorf("解析 API 地址失败：%w", err)
	}
	return endpoint, nil
}

func rawID(raw json.RawMessage) (string, error) {
	var stringID string
	if err := json.Unmarshal(raw, &stringID); err == nil && stringID != "" {
		return stringID, nil
	}
	var numberID json.Number
	if err := json.Unmarshal(raw, &numberID); err == nil && numberID.String() != "" {
		return numberID.String(), nil
	}
	return "", fmt.Errorf("缺少好友 ID 或 ID 格式无效")
}

func rawNumber(raw json.RawMessage) int {
	var value int
	_ = json.Unmarshal(raw, &value)
	return value
}
