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

// Crawler fetches the authenticated user's activities and friend profiles.
// BaseURL must point at an API root; Authorization is sent as a request header.
type Crawler struct {
	Client        *HTTPClient
	BaseURL       string
	Headers       http.Header
	ActivityLimit int
}

// FetchMemories returns all activity pages and stops at the first empty page.
// An API or decoding error is returned instead of producing a partial result.
func (c *Crawler) FetchMemories(ctx context.Context) ([]json.RawMessage, error) {
	if c.ActivityLimit < 1 {
		return nil, fmt.Errorf("activity limit must be positive")
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
			return nil, fmt.Errorf("fetch activities at offset %d: %w", offset, err)
		}
		if len(page) == 0 {
			return all, nil
		}
		all = append(all, page...)
	}
}

// FetchFriends returns the relationship entries with each entry's profile in
// the "info" field, matching the Python output shape.
func (c *Crawler) FetchFriends(ctx context.Context) ([]map[string]json.RawMessage, error) {
	endpoint, err := c.endpoint("user/relationships")
	if err != nil {
		return nil, err
	}
	var relationships struct {
		Friends []map[string]json.RawMessage `json:"friends"`
	}
	if err := c.Client.GetJSON(ctx, endpoint.String(), c.Headers, &relationships); err != nil {
		return nil, fmt.Errorf("fetch relationships: %w", err)
	}

	friends := make([]map[string]json.RawMessage, 0, len(relationships.Friends))
	for index, friend := range relationships.Friends {
		id, err := rawID(friend["id"])
		if err != nil {
			return nil, fmt.Errorf("friend %d: %w", index, err)
		}
		profileURL, err := c.endpoint("users/" + url.PathEscape(id))
		if err != nil {
			return nil, err
		}
		var profile map[string]json.RawMessage
		if err := c.Client.GetJSON(ctx, profileURL.String(), c.Headers, &profile); err != nil {
			return nil, fmt.Errorf("fetch profile for friend %s: %w", id, err)
		}
		friend["info"], err = json.Marshal(profile)
		if err != nil {
			return nil, fmt.Errorf("encode profile for friend %s: %w", id, err)
		}
		friends = append(friends, friend)
	}
	return friends, nil
}

func (c *Crawler) endpoint(path string) (*url.URL, error) {
	base := strings.TrimRight(c.BaseURL, "/") + "/"
	endpoint, err := url.Parse(base + strings.TrimLeft(path, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse API URL: %w", err)
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
	return "", fmt.Errorf("missing or invalid friend id")
}
