package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// CleanMemories 将原始动态按类型拆分，并提炼出适合后续使用的字段。
// question 不为空的动态归入黑板墙，其余动态归入普通动态。
func CleanMemories(memories []json.RawMessage) (blackboard, normal, refinedBlackboard, refinedNormal []map[string]json.RawMessage, err error) {
	for index, rawMemory := range memories {
		var memory map[string]json.RawMessage
		if err := json.Unmarshal(rawMemory, &memory); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("解析第 %d 条动态失败：%w", index, err)
		}

		if question, ok := memory["question"]; ok && !isJSONNullOrFalse(question) {
			blackboard = append(blackboard, memory)
			refined, err := refineBlackboardMemory(memory, index)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			refinedBlackboard = append(refinedBlackboard, refined)
			continue
		}

		normal = append(normal, memory)
		refined, err := refineNormalMemory(memory, index)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		refinedNormal = append(refinedNormal, refined)
	}
	return blackboard, normal, refinedBlackboard, refinedNormal, nil
}

func refineBlackboardMemory(memory map[string]json.RawMessage, index int) (map[string]json.RawMessage, error) {
	question, err := requiredObjectField(memory, "question", index)
	if err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{}
	questionContent, ok := question["content"]
	if !ok {
		return nil, fmt.Errorf("第 %d 条黑板墙动态缺少 question.content", index)
	}
	result["question"] = questionContent
	for _, field := range []string{"images", "created_at"} {
		value, ok := memory[field]
		if !ok {
			value = json.RawMessage("null")
		}
		result[map[string]string{"images": "images", "created_at": "time"}[field]] = value
	}
	content, ok := memory["content"]
	if !ok {
		return nil, fmt.Errorf("第 %d 条黑板墙动态缺少 content", index)
	}
	result["my_answer"] = content
	if comments, ok := memory["comments"]; ok {
		result["comments"] = comments
	}
	return result, nil
}

func refineNormalMemory(memory map[string]json.RawMessage, index int) (map[string]json.RawMessage, error) {
	result := map[string]json.RawMessage{"title": json.RawMessage("null")}
	if title, ok := memory["title"]; ok {
		result["title"] = title
	}
	for _, field := range []string{"content", "images", "created_at"} {
		value, ok := memory[field]
		if !ok {
			return nil, fmt.Errorf("第 %d 条普通动态缺少 %s", index, field)
		}
		result[map[string]string{"content": "content", "images": "images", "created_at": "time"}[field]] = value
	}

	if rawURL, ok := memory["outchain"]; ok && !isJSONNullOrFalse(rawURL) {
		result["outlink"] = objectWithOptionalFields(memory, "outchain", "outchain_title", "outchain_img", "outchain_author")
	}
	if comments, ok := memory["comments"]; ok {
		result["comments"] = comments
	}
	return result, nil
}

func requiredObjectField(memory map[string]json.RawMessage, field string, index int) (map[string]json.RawMessage, error) {
	raw, ok := memory[field]
	if !ok {
		return nil, fmt.Errorf("第 %d 条动态缺少 %s", index, field)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("第 %d 条动态的 %s 格式无效：%w", index, field, err)
	}
	return value, nil
}

func objectWithOptionalFields(memory map[string]json.RawMessage, fields ...string) json.RawMessage {
	result := make(map[string]json.RawMessage, len(fields))
	keys := []string{"url", "title", "image", "author"}
	for index, field := range fields {
		value, ok := memory[field]
		if !ok {
			value = json.RawMessage("null")
		}
		result[keys[index]] = value
	}
	encoded, _ := json.Marshal(result)
	return encoded
}

func isJSONNullOrFalse(raw json.RawMessage) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return true
	}
	switch value := value.(type) {
	case nil:
		return true
	case bool:
		return !value
	case string:
		return value == ""
	case float64:
		return value == 0
	case []any:
		return len(value) == 0
	case map[string]any:
		return len(value) == 0
	default:
		return false
	}
}

// WriteMemoriesMarkdown 将普通动态正文导出为便于 LLM 阅读的 Markdown。
func WriteMemoriesMarkdown(path string, memories []map[string]json.RawMessage) error {
	data := []byte("## 我在某社交软件上发过的所有动态\n\n")
	for index, memory := range memories {
		time, err := requiredString(memory, "time", index)
		if err != nil {
			return err
		}
		content, err := requiredString(memory, "content", index)
		if err != nil {
			return err
		}
		data = append(data, []byte(fmt.Sprintf("### 时间:%s\n\n%s\n\n", time, content))...)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("写入 Markdown 文件 %s 失败：%w", path, err)
	}
	return nil
}

// FriendStatusCounts 统计好友资料中的 status；缺少 status 时按 OK 计数。
func FriendStatusCounts(friends []map[string]json.RawMessage) map[string]int {
	counts := make(map[string]int)
	for _, friend := range friends {
		status := "OK"
		var info map[string]json.RawMessage
		if rawInfo, ok := friend["info"]; ok && !isJSONNullOrFalse(rawInfo) && json.Unmarshal(rawInfo, &info) == nil {
			if rawStatus, ok := info["status"]; ok {
				var value string
				if json.Unmarshal(rawStatus, &value) == nil && value != "" {
					status = value
				}
			}
		}
		counts[status]++
	}
	return counts
}

func requiredString(memory map[string]json.RawMessage, field string, index int) (string, error) {
	raw, ok := memory[field]
	if !ok {
		return "", fmt.Errorf("第 %d 条提炼动态缺少 %s", index, field)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("第 %d 条提炼动态的 %s 不是字符串：%w", index, field, err)
	}
	return value, nil
}
