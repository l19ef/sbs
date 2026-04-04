package builder

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type SubscriptionContentLoader interface {
	Load(ctx context.Context, source subscriptionSource) ([]byte, error)
}

type URLLoader interface {
	LoadURL(ctx context.Context, source subscriptionSource) ([]byte, error)
}

func DefaultLoader() SubscriptionContentLoader {
	return URLLoaderAdapter{
		Loader: HTTPURLLoader{
			Client: &http.Client{Timeout: 30 * time.Second},
		},
	}
}

type URLLoaderAdapter struct {
	Loader URLLoader
}

func (l URLLoaderAdapter) Load(ctx context.Context, source subscriptionSource) ([]byte, error) {
	if l.Loader == nil {
		return nil, fmt.Errorf("url loader is not configured")
	}
	return l.Loader.LoadURL(ctx, source)
}

type HTTPURLLoader struct {
	Client *http.Client
}

func (l HTTPURLLoader) LoadURL(ctx context.Context, source subscriptionSource) ([]byte, error) {
	if source.URL == "" {
		return nil, fmt.Errorf("url is empty")
	}

	client := l.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "sb-config-manager/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch subscription: unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read subscription body: %w", err)
	}
	return data, nil
}
