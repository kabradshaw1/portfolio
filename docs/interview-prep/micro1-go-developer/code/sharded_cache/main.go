package main

import (
	"hash/fnv"
	"sync"
	"time"
)

type entry struct {
	value     string
	expiresAt time.Time
}

type shard struct {
	mu sync.RWMutex
	m  map[string]entry
}

type Cache struct {
	shards []shard
}

func (c *Cache) shardFor(key string) *shard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &c.shards[int(h.Sum32())%len(c.shards)]
}
