package store

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kabradshaw1/portfolio/go/pkg/tracing"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"
)

const (
	revenueTTL     = 48 * time.Hour
	trendingTTL    = 2 * time.Hour
	abandonmentTTL = 24 * time.Hour

	revenuePrefix     = "analytics:revenue:"
	trendingPrefix    = "analytics:trending:"
	abandonmentPrefix = "analytics:abandonment:"
	abandonUserPrefix = "analytics:abandonment:users:"

	// windowKeyLayout is the canonical time layout used for flushed window keys.
	windowKeyLayout = time.RFC3339
)

// RedisStore implements Store backed by Redis.
type RedisStore struct {
	client  *redis.Client
	breaker *gobreaker.CircuitBreaker[any]
}

// NewRedisStore creates a RedisStore with circuit breaker protection.
func NewRedisStore(client *redis.Client, breaker *gobreaker.CircuitBreaker[any]) *RedisStore {
	return &RedisStore{
		client:  client,
		breaker: breaker,
	}
}

// FlushRevenue writes revenue counters for the given window.
func (s *RedisStore) FlushRevenue(ctx context.Context, windowKey string, totalCents, orderCount int64) error {
	key := revenuePrefix + windowKey

	_, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "FlushRevenue", key)
		defer span.End()

		pipe := s.client.Pipeline()
		avg := int64(0)
		if orderCount > 0 {
			avg = totalCents / orderCount
		}
		pipe.HSet(ctx2, key,
			"total_cents", totalCents,
			"order_count", orderCount,
			"avg_cents", avg,
		)
		pipe.Expire(ctx2, key, revenueTTL)
		_, err := pipe.Exec(ctx2)
		if err != nil {
			return nil, fmt.Errorf("flush revenue pipeline: %w", err)
		}
		return nil, nil
	})
	return err
}

// GetRevenue returns revenue windows for the last N hours, sorted chronologically.
func (s *RedisStore) GetRevenue(ctx context.Context, hours int) ([]RevenueWindow, error) {
	result, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "GetRevenue", revenuePrefix+"*")
		defer span.End()

		cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
		var windows []RevenueWindow

		iter := s.client.Scan(ctx2, 0, revenuePrefix+"*", 0).Iterator()
		for iter.Next(ctx2) {
			key := iter.Val()
			windowKey := strings.TrimPrefix(key, revenuePrefix)
			ws, err := parseWindowKey(windowKey)
			if err != nil || ws.Before(cutoff) {
				continue
			}

			vals, err := s.client.HGetAll(ctx2, key).Result()
			if err != nil {
				return nil, fmt.Errorf("get revenue hgetall %s: %w", key, err)
			}
			if len(vals) == 0 {
				continue
			}

			tc, _ := strconv.ParseInt(vals["total_cents"], 10, 64)
			oc, _ := strconv.ParseInt(vals["order_count"], 10, 64)
			ac, _ := strconv.ParseInt(vals["avg_cents"], 10, 64)

			windows = append(windows, RevenueWindow{
				WindowStart: ws,
				WindowEnd:   ws.Add(time.Hour),
				TotalCents:  tc,
				OrderCount:  oc,
				AvgCents:    ac,
			})
		}
		if err := iter.Err(); err != nil {
			return nil, fmt.Errorf("scan revenue keys: %w", err)
		}

		sort.Slice(windows, func(i, j int) bool {
			return windows[i].WindowStart.Before(windows[j].WindowStart)
		})
		return windows, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]RevenueWindow), nil
}

// FlushTrending writes product scores to a sorted set and product names to a hash for the given window.
func (s *RedisStore) FlushTrending(ctx context.Context, windowKey string, scores map[string]float64, names map[string]string) error {
	if len(scores) == 0 {
		return nil
	}

	key := trendingPrefix + windowKey
	namesKey := trendingPrefix + "names:" + windowKey

	_, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "FlushTrending", key)
		defer span.End()

		members := make([]redis.Z, 0, len(scores))
		for productID, score := range scores {
			members = append(members, redis.Z{Score: score, Member: productID})
		}

		pipe := s.client.Pipeline()
		pipe.ZAdd(ctx2, key, members...)
		pipe.Expire(ctx2, key, trendingTTL)

		if len(names) > 0 {
			fields := make([]string, 0, len(names)*2)
			for pid, name := range names {
				fields = append(fields, pid, name)
			}
			pipe.HSet(ctx2, namesKey, fields)
			pipe.Expire(ctx2, namesKey, trendingTTL)
		}

		_, err := pipe.Exec(ctx2)
		if err != nil {
			return nil, fmt.Errorf("flush trending pipeline: %w", err)
		}
		return nil, nil
	})
	return err
}

// GetTrending returns the top trending products from the most recent window.
func (s *RedisStore) GetTrending(ctx context.Context, limit int) (*TrendingResult, error) {
	result, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "GetTrending", trendingPrefix+"*")
		defer span.End()

		var latestKey string
		var latestTime time.Time

		iter := s.client.Scan(ctx2, 0, trendingPrefix+"*", 0).Iterator()
		for iter.Next(ctx2) {
			key := iter.Val()
			windowKey := strings.TrimPrefix(key, trendingPrefix)
			if strings.HasPrefix(windowKey, "names:") {
				continue
			}
			t, err := parseWindowKey(windowKey)
			if err != nil {
				continue
			}
			if latestKey == "" || t.After(latestTime) {
				latestKey = key
				latestTime = t
			}
		}
		if err := iter.Err(); err != nil {
			return nil, fmt.Errorf("scan trending keys: %w", err)
		}

		if latestKey == "" {
			return (*TrendingResult)(nil), nil
		}

		// Extract the windowKey by stripping the prefix from the latest key.
		windowKey := latestKey[len(trendingPrefix):]
		namesKey := trendingPrefix + "names:" + windowKey

		members, err := s.client.ZRevRangeWithScores(ctx2, latestKey, 0, int64(limit-1)).Result()
		if err != nil {
			return nil, fmt.Errorf("get trending zrevrange: %w", err)
		}

		// Fetch product names from the names hash.
		nameMap, err := s.client.HGetAll(ctx2, namesKey).Result()
		if err != nil {
			return nil, fmt.Errorf("get trending names: %w", err)
		}

		products := make([]TrendingProduct, len(members))
		for i, m := range members {
			pid := m.Member.(string)
			products[i] = TrendingProduct{
				ProductID:   pid,
				ProductName: nameMap[pid],
				Score:       m.Score,
			}
		}

		return &TrendingResult{
			WindowEnd: latestTime.Add(time.Hour),
			Products:  products,
		}, nil
	})
	if err != nil {
		return nil, err
	}
	r, _ := result.(*TrendingResult)
	return r, nil
}

// FlushAbandonment writes cart abandonment metrics for the given window.
func (s *RedisStore) FlushAbandonment(ctx context.Context, windowKey string, started, converted int64) error {
	key := abandonmentPrefix + windowKey

	_, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "FlushAbandonment", key)
		defer span.End()

		abandoned := started - converted
		if abandoned < 0 {
			abandoned = 0
		}
		var rate float64
		if started > 0 {
			rate = float64(abandoned) / float64(started)
		}

		pipe := s.client.Pipeline()
		pipe.HSet(ctx2, key,
			"started", started,
			"converted", converted,
			"abandoned", abandoned,
			"rate", strconv.FormatFloat(rate, 'f', 4, 64),
		)
		pipe.Expire(ctx2, key, abandonmentTTL)
		_, err := pipe.Exec(ctx2)
		if err != nil {
			return nil, fmt.Errorf("flush abandonment pipeline: %w", err)
		}
		return nil, nil
	})
	return err
}

// GetAbandonment returns abandonment windows for the last N hours, sorted chronologically.
func (s *RedisStore) GetAbandonment(ctx context.Context, hours int) ([]AbandonmentWindow, error) {
	result, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "GetAbandonment", abandonmentPrefix+"*")
		defer span.End()

		cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)
		var windows []AbandonmentWindow

		iter := s.client.Scan(ctx2, 0, abandonmentPrefix+"*", 0).Iterator()
		for iter.Next(ctx2) {
			key := iter.Val()
			windowKey := strings.TrimPrefix(key, abandonmentPrefix)
			if strings.HasPrefix(windowKey, "users:") {
				continue
			}
			ws, err := parseWindowKey(windowKey)
			if err != nil || ws.Before(cutoff) {
				continue
			}

			vals, err := s.client.HGetAll(ctx2, key).Result()
			if err != nil {
				return nil, fmt.Errorf("get abandonment hgetall %s: %w", key, err)
			}
			if len(vals) == 0 {
				continue
			}

			started, _ := strconv.ParseInt(vals["started"], 10, 64)
			converted, _ := strconv.ParseInt(vals["converted"], 10, 64)
			abandoned, _ := strconv.ParseInt(vals["abandoned"], 10, 64)
			rate, _ := strconv.ParseFloat(vals["rate"], 64)

			windows = append(windows, AbandonmentWindow{
				WindowStart:     ws,
				WindowEnd:       ws.Add(30 * time.Minute),
				CartsStarted:    started,
				CartsConverted:  converted,
				CartsAbandoned:  abandoned,
				AbandonmentRate: rate,
			})
		}
		if err := iter.Err(); err != nil {
			return nil, fmt.Errorf("scan abandonment keys: %w", err)
		}

		sort.Slice(windows, func(i, j int) bool {
			return windows[i].WindowStart.Before(windows[j].WindowStart)
		})
		return windows, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]AbandonmentWindow), nil
}

// TrackAbandonmentUser adds a user to the set of users in a given abandonment bucket.
func (s *RedisStore) TrackAbandonmentUser(ctx context.Context, windowKey, userID, bucket string) error {
	key := abandonUserPrefix + windowKey + ":" + bucket

	_, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "TrackAbandonmentUser", key)
		defer span.End()

		pipe := s.client.Pipeline()
		pipe.SAdd(ctx2, key, userID)
		pipe.Expire(ctx2, key, abandonmentTTL)
		_, err := pipe.Exec(ctx2)
		if err != nil {
			return nil, fmt.Errorf("track abandonment user: %w", err)
		}
		return nil, nil
	})
	return err
}

// CountAbandonmentUsers returns the number of unique users in a given abandonment bucket.
func (s *RedisStore) CountAbandonmentUsers(ctx context.Context, windowKey, bucket string) (int64, error) {
	key := abandonUserPrefix + windowKey + ":" + bucket

	result, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "CountAbandonmentUsers", key)
		defer span.End()

		count, err := s.client.SCard(ctx2, key).Result()
		if err != nil {
			return int64(0), fmt.Errorf("count abandonment users: %w", err)
		}
		return count, nil
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}
