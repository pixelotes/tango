package trackerlist

import (
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// List fetches and caches a public tracker list, refreshing it periodically.
type List struct {
	url      string
	interval time.Duration

	mu       sync.RWMutex
	trackers []string

	stopCh chan struct{}
	done   chan struct{}
}

func New(url string, interval time.Duration) *List {
	return &List{
		url:      url,
		interval: interval,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start performs an initial fetch and then refreshes on the configured interval.
func (l *List) Start() {
	l.fetch()

	go func() {
		defer close(l.done)
		ticker := time.NewTicker(l.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				l.fetch()
			case <-l.stopCh:
				return
			}
		}
	}()
}

func (l *List) Stop() {
	close(l.stopCh)
	<-l.done
}

// Trackers returns the current cached tracker URLs.
func (l *List) Trackers() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]string, len(l.trackers))
	copy(out, l.trackers)
	return out
}

func (l *List) fetch() {
	resp, err := http.Get(l.url)
	if err != nil {
		log.Printf("trackerlist: fetch failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("trackerlist: fetch returned %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("trackerlist: read body failed: %v", err)
		return
	}

	trackers := parseTrackerList(string(body))

	l.mu.Lock()
	l.trackers = trackers
	l.mu.Unlock()

	log.Printf("trackerlist: loaded %d trackers from %s", len(trackers), l.url)
}

// parseTrackerList splits the ngosang-format text (one URL per line, blank lines as separators).
func parseTrackerList(data string) []string {
	var result []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
