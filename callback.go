package pinqloq

import "log"

func raiseSent(onSent OnSent, entry LogEntry) {
	if onSent == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("pinqloq: the OnSent callback panicked; recovered. %v", r)
		}
	}()

	onSent(entry)
}

func raiseFailed(onFailed OnFailed, entry LogEntry, err *LogError) {
	if onFailed == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			log.Printf("pinqloq: the OnFailed callback panicked; recovered. %v", r)
		}
	}()

	onFailed(entry, err)
}
