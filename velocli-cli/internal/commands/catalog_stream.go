package commands

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

type catalogStreamEvent struct {
	kind string
	data string
}

type catalogStream struct {
	mu     sync.Mutex
	events chan catalogStreamEvent
	start  sync.Once
}

func newCatalogStream() *catalogStream {
	return &catalogStream{events: make(chan catalogStreamEvent, 16)}
}

func (s *catalogStream) startIfNeeded(apiURL string) {
	s.start.Do(func() {
		go s.run(apiURL)
	})
}

func (s *catalogStream) run(apiURL string) {
	url := strings.TrimRight(apiURL, "/") + "/api/v1/catalog/stream"

	backoff := 400 * time.Millisecond
	for {
		ctx := context.Background()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			time.Sleep(backoff)
			backoff = minDuration(backoff*2, 6*time.Second)
			continue
		}
		req.Header.Set("Accept", "text/event-stream")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(backoff)
			backoff = minDuration(backoff*2, 6*time.Second)
			continue
		}

		if res.StatusCode < 200 || res.StatusCode >= 300 {
			_ = res.Body.Close()
			time.Sleep(backoff)
			backoff = minDuration(backoff*2, 6*time.Second)
			continue
		}

		backoff = 400 * time.Millisecond
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 0, 64<<10), 2<<20)

		var eventName string
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				eventName = ""
				continue
			}
			if strings.HasPrefix(line, "event:") {
				eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
				continue
			}
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if eventName == "catalog" {
					select {
					case s.events <- catalogStreamEvent{kind: "catalog", data: data}:
					default:
					}
				}
			}
		}
		_ = res.Body.Close()
		time.Sleep(backoff)
		backoff = minDuration(backoff*2, 6*time.Second)
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
