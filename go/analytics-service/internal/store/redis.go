package store

import (
	"context"
	"fmt"
	"sort"
	"strconv"
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

	// windowKeyLayout is the time layout used for window keys (hourly granularity).
	windowKeyLayout = "2006-01-02T15"
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

// FlushRevenue atomically increments revenue counters for the given window.
func (s *RedisStore) FlushRevenue(ctx context.Context, windowKey string, totalCents, orderCount int64) error {
	key := revenuePrefix + windowKey

	_, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "FlushRevenue", key)
		defer span.End()

		pipe := s.client.Pipeline()
		pipe.HIncrBy(ctx2, key, "total_cents", totalCents)
		pipe.HIncrBy(ctx2, key, "order_count", orderCount)
		pipe.Expire(ctx2, key, revenueTTL)
		_, err := pipe.Exec(ctx2)
		if err != nil {
			return nil, fmt.Errorf("flush revenue pipeline: %w", err)
		}

		// Recompute average after increment.
		vals, err := s.client.HMGet(ctx2, key, "total_cents", "order_count").Result()
		if err != nil {
			return nil, fmt.Errorf("flush revenue hmget: %w", err)
		}
		tc, _ := strconv.ParseInt(fmt.Sprint(vals[0]), 10, 64)
		oc, _ := strconv.ParseInt(fmt.Sprint(vals[1]), 10, 64)
		var avg int64
		if oc > 0 {
			avg = tc / oc
		}
		return nil, s.client.HSet(ctx2, key, "avg_cents", avg).Err()
	})
	return err
}

// GetRevenue returns revenue windows for the last N hours, sorted chronologically.
func (s *RedisStore) GetRevenue(ctx context.Context, hours int) ([]RevenueWindow, error) {
	result, err := s.breaker.Execute(func() (any, error) {
		ctx2, span := tracing.RedisSpan(ctx, "GetRevenue", revenuePrefix+"*")
		defer span.End()

		now := time.Now().UTC()
		var windows []RevenueWindow

		for i := 0; i < hours; i++ {
			t := now.Add(-time.Duration(i) * time.Hour)
			windowKey := t.Format(windowKeyLayout)
			key := revenuePrefix + windowKey

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

			ws := t.Truncate(time.Hour)
			windows = append(windows, RevenueWindow{
				WindowStart: ws,
				WindowEnd:   ws.Add(time.Hour),
				TotalCents:  tc,
				OrderCount:  oc,
				AvgCents:    ac,
			})
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

// FlushTrending writes product scores to a sorted set for the given window.
func (s *RedisStore) FlushTrending(ctx context.Context, windowKey string, scores map[string]float64) error {
	key := trendingPrefix + windowKey

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

		// Check the last 2 hours for the most recent trending window.
		now := time.Now().UTC()
		var latestKey string
		var latestTime time.Time

		for i := 0; i < 2; i++ {
			t := now.Add(-time.Duration(i) * time.Hour)
			windowKey := t.Format(windowKeyLayout)
			key := trendingPrefix + windowKey

			exists, err := s.client.Exists(ctx2, key).Result()
			if err != nil {
				return nil, fmt.Errorf("get trending exists %s: %w", key, err)
			}
			if exists > 0 {
				latestKey = key
				latestTime = t.Truncate(time.Hour)
				break
			}
		}

		if latestKey == "" {
			return (*TrendingResult)(nil), nil
		}

		members, err := s.client.ZRevRangeWithScores(ctx2, latestKey, 0, int64(limit-1)).Result()
		if err != nil {
			return nil, fmt.Errorf("get trending zrevrange: %w", err)
		}

		products := make([]TrendingProduct, len(members))
		for i, m := range members {
			products[i] = TrendingProduct{
				ProductID: m.Member.(string),
				Score:     m.Score,
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

		now := time.Now().UTC()
		var windows []AbandonmentWindow

		for i := 0; i < hours; i++ {
			t := now.Add(-time.Duration(i) * time.Hour)
			windowKey := t.Format(windowKeyLayout)
			key := abandonmentPrefix + windowKey

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

			ws := t.Truncate(time.Hour)
			windows = append(windows, AbandonmentWindow{
				WindowStart:     ws,
				WindowEnd:       ws.Add(time.Hour),
				CartsStarted:    started,
				CartsConverted:  converted,
				CartsAbandoned:  abandoned,
				AbandonmentRate: rate,
			})
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
