package pinqloq

import (
	"log"
	"sync"
	"time"
)

var throttledWarnMu sync.Mutex
var throttledWarnNextAt = map[string]time.Time{}

func warnThrottled(key string, interval time.Duration, format string, args ...any) {
	throttledWarnMu.Lock()
	now := time.Now()
	if now.Before(throttledWarnNextAt[key]) {
		throttledWarnMu.Unlock()
		return
	}
	throttledWarnNextAt[key] = now.Add(interval)
	throttledWarnMu.Unlock()

	log.Printf(format, args...)
}
