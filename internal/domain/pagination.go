package domain

import (
	"encoding/base64"
	"encoding/json"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

type Page struct {
	Limit  int
	Cursor string
}

func (p Page) Size() int {
	if p.Limit <= 0 {
		return DefaultPageSize
	}
	return min(p.Limit, MaxPageSize)
}

type PageResult[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func EncodeCursor(values ...string) string {
	raw, _ := json.Marshal(values)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func DecodeCursor(token string, parts int) ([]string, error) {
	if token == "" {
		return nil, nil
	}
	if len(token) > 2048 {
		return nil, Invalid("invalid cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, Invalid("invalid cursor")
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil || len(values) != parts {
		return nil, Invalid("invalid cursor")
	}
	return values, nil
}
