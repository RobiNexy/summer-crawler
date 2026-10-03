package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"
)

// HTTPClient 用有限次数的重试发送 JSON 请求。
// 只有服务器返回 2xx 状态码且响应内容为有效 JSON 时，请求才算成功；
// 传输、HTTP 状态或解码错误会在全部尝试结束后返回。
type HTTPClient struct {
	Client         *http.Client
	Retries        int
	Delay          time.Duration
	Logger         *log.Logger
	RandomDelayMin time.Duration
	RandomDelayMax time.Duration
}

// NewHTTPClient 创建 HTTP 客户端。
// Retries 表示尝试总次数，而非首次请求之外的额外重试次数。
func NewHTTPClient(timeout time.Duration, retries int, delay time.Duration, logger *log.Logger) (*HTTPClient, error) {
	if retries < 1 {
		return nil, fmt.Errorf("重试次数至少为 1")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("超时时间必须大于 0")
	}
	if delay < 0 {
		return nil, fmt.Errorf("重试间隔不能为负数")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &HTTPClient{
		Client:  &http.Client{Timeout: timeout},
		Retries: retries,
		Delay:   delay,
		Logger:  logger,
	}, nil
}

// SetRandomDelay 设置每次网络请求前的随机等待区间，避免连续冲击服务端。
func (c *HTTPClient) SetRandomDelay(minimum, maximum time.Duration) {
	if minimum < 0 {
		minimum = 0
	}
	if maximum < minimum {
		maximum = minimum
	}
	c.RandomDelayMin = minimum
	c.RandomDelayMax = maximum
}

func (c *HTTPClient) wait(ctx context.Context) error {
	delay := c.RandomDelayMin
	if c.RandomDelayMax > c.RandomDelayMin {
		delay += time.Duration(rand.Int63n(int64(c.RandomDelayMax - c.RandomDelayMin + 1)))
	}
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// GetJSON 发送 GET 请求，并将响应解码到 out。
// out 由调用方持有；发生错误时可能包含部分解码结果，不应继续使用。
// 上下文取消后会停止后续尝试及等待。
func (c *HTTPClient) GetJSON(ctx context.Context, url string, headers http.Header, out any) error {
	var lastErr error
	for attempt := 1; attempt <= c.Retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.wait(ctx); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("创建 GET 请求失败：%w", err)
		}
		req.Header = headers.Clone()
		response, err := c.Client.Do(req)
		if err == nil {
			err = decodeResponse(response, out)
		}
		if err == nil {
			return nil
		}
		lastErr = err
		c.Logger.Printf("GET 请求 %s 第 %d/%d 次尝试失败：%v", url, attempt, c.Retries, err)
		if attempt < c.Retries {
			timer := time.NewTimer(c.Delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return fmt.Errorf("GET 请求 %s 在尝试 %d 次后仍失败：%w", url, c.Retries, lastErr)
}

// Download 获取二进制资源，用于保存头像、图片和录音。
func (c *HTTPClient) Download(ctx context.Context, url string, headers http.Header) ([]byte, string, error) {
	var lastErr error
	for attempt := 1; attempt <= c.Retries; attempt++ {
		if err := c.wait(ctx); err != nil {
			return nil, "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, "", fmt.Errorf("创建资源请求失败：%w", err)
		}
		req.Header = headers.Clone()
		response, err := c.Client.Do(req)
		if err == nil && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			data, readErr := io.ReadAll(response.Body)
			contentType := response.Header.Get("Content-Type")
			response.Body.Close()
			if readErr == nil {
				return data, contentType, nil
			}
			err = readErr
		} else if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			err = fmt.Errorf("HTTP 状态异常：%s", response.Status)
		}
		lastErr = err
		c.Logger.Printf("下载资源 %s 第 %d/%d 次尝试失败：%v", url, attempt, c.Retries, err)
		if attempt < c.Retries {
			time.Sleep(c.Delay)
		}
	}
	return nil, "", fmt.Errorf("下载资源 %s 在尝试 %d 次后仍失败：%w", url, c.Retries, lastErr)
}

func decodeResponse(response *http.Response, out any) error {
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, response.Body)
		return fmt.Errorf("HTTP 状态异常：%s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("解码 JSON 响应失败：%w", err)
	}
	return nil
}
