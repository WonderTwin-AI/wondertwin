package replay

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// BenchmarkMiddlewareLatencyAdded measures the latency the replay
// middleware adds to a no-op handler under concurrent load. Reports
// p50/p99 added latency in nanoseconds.
//
// Target: p99 < 100µs at 1k concurrent requests with 4 KiB bodies.
// Future changes that regress this bound should be flagged.
func BenchmarkMiddlewareLatencyAdded(b *testing.B) {
	r := NewRecorder(Config{CaptureBodies: true, MaxBodyBytes: 64 * 1024})
	defer r.Close()

	body := strings.Repeat("x", 4*1024)
	plain := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	wrapped := r.Middleware()(plain)

	const workers = 8
	iterations := b.N
	if iterations < workers {
		iterations = workers
	}
	per := iterations / workers
	plainSamples := make([][]int64, workers)
	wrappedSamples := make([][]int64, workers)

	measure := func(handler http.Handler, samples []int64) {
		for i := range samples {
			req := httptest.NewRequest(http.MethodPost, "/v1/charges", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			start := time.Now()
			handler.ServeHTTP(w, req)
			samples[i] = time.Since(start).Nanoseconds()
		}
	}

	b.ResetTimer()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			plainSamples[w] = make([]int64, per)
			wrappedSamples[w] = make([]int64, per)
			measure(plain, plainSamples[w])
			measure(wrapped, wrappedSamples[w])
		}(w)
	}
	wg.Wait()

	all := func(parts [][]int64) []int64 {
		var out []int64
		for _, s := range parts {
			out = append(out, s...)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		return out
	}
	plainAll := all(plainSamples)
	wrappedAll := all(wrappedSamples)
	if len(plainAll) == 0 {
		return
	}
	plainP50 := plainAll[len(plainAll)/2]
	plainP99 := plainAll[(len(plainAll)*99)/100]
	wrappedP50 := wrappedAll[len(wrappedAll)/2]
	wrappedP99 := wrappedAll[(len(wrappedAll)*99)/100]

	addedP50 := wrappedP50 - plainP50
	addedP99 := wrappedP99 - plainP99
	if addedP50 < 0 {
		addedP50 = 0
	}
	if addedP99 < 0 {
		addedP99 = 0
	}
	b.ReportMetric(float64(addedP50), "addedP50-ns/op")
	b.ReportMetric(float64(addedP99), "addedP99-ns/op")
}
