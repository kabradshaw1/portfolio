package store

import "time"

func parseWindowKey(key string) (time.Time, error) {
	if t, err := time.Parse(windowKeyLayout, key); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15", key)
}
