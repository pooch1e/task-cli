// Package ratelimit tracks LLM call frequency and warns on excessive usage.
// State is persisted in a JSON file so it survives process restarts.
package ratelimit

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	maxCalls   = 5
	windowSecs = 60
	stateFile  = "ratelimit.json"
)

// CheckAndRecord records the current call timestamp and reports whether the
// rate limit (maxCalls within windowSecs) has been exceeded.
// A non-nil error means the state file could not be read or written; the
// caller should treat this as a non-fatal warning and proceed.
func CheckAndRecord(dir string) (exceeded bool, err error) {
	path := filepath.Join(dir, stateFile)

	calls, err := readTimestamps(path)
	if err != nil {
		return false, err
	}

	cutoff := time.Now().Add(-windowSecs * time.Second)
	recent := pruneOlderThan(calls, cutoff)

	exceeded = len(recent) >= maxCalls

	recent = append(recent, time.Now())
	return exceeded, writeTimestamps(path, recent)
}

// readTimestamps loads the persisted call log, returning an empty slice if
// the file does not exist.
func readTimestamps(path string) ([]time.Time, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stamps []time.Time
	if err := json.Unmarshal(data, &stamps); err != nil {
		// Corrupt file — reset rather than blocking the user, but log it.
		log.Printf("warning: ratelimit state corrupt, resetting (%v)", err)
		return nil, nil
	}
	return stamps, nil
}

// writeTimestamps persists the call log atomically via a temp file + rename.
func writeTimestamps(path string, stamps []time.Time) error {
	data, err := json.Marshal(stamps)
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// pruneOlderThan returns only those timestamps that are after cutoff.
func pruneOlderThan(stamps []time.Time, cutoff time.Time) []time.Time {
	out := stamps[:0] // reuse backing array, no allocation if all are recent
	for _, t := range stamps {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}
