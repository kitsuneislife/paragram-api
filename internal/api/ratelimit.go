package api

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"time"
)

// rateLimit lets each client IP make limit requests per window to every
// handler sharing bucket, counted in Redis so all instances agree.
// shortcut: keys on RemoteAddr, so behind a proxy all clients share one IP; read X-Forwarded-For from a trusted proxy once one is deployed.
func (s *server) rateLimit(bucket string, limit int64, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		key := "ratelimit:" + bucket + ":" + ip

		pipe := s.Redis.TxPipeline()
		incr := pipe.Incr(r.Context(), key)
		pipe.ExpireNX(r.Context(), key, window)
		ttl := pipe.PTTL(r.Context(), key)
		if _, err := pipe.Exec(r.Context()); err != nil {
			writeInternal(w, r, s.Log, err)
			return
		}

		reset := strconv.Itoa(int(math.Ceil(ttl.Val().Seconds())))
		h := w.Header()
		h.Set("RateLimit-Limit", strconv.FormatInt(limit, 10))
		h.Set("RateLimit-Remaining", strconv.FormatInt(max(0, limit-incr.Val()), 10))
		h.Set("RateLimit-Reset", reset)
		if incr.Val() > limit {
			h.Set("Retry-After", reset)
			writeProblem(w, http.StatusTooManyRequests, "rate-limited", "too many requests, retry later")
			return
		}
		next(w, r)
	}
}
