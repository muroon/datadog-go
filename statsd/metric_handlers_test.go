package statsd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldSample(t *testing.T) {
	rates := []float64{0.01, 0.05, 0.1, 0.25, 0.5, 0.75, 0.9, 0.99, 1.0}
	iterations := 50_000

	for _, rate := range rates {
		rate := rate // Capture range variable.
		t.Run(fmt.Sprintf("Rate %0.2f", rate), func(t *testing.T) {
			t.Parallel()

			mh := newMetricHandler(newBufferPool(1, 1, 1), nil)
			count := 0
			for i := 0; i < iterations; i++ {
				if shouldSample(rate, mh.random, &mh.randomLock) {
					count++
				}
			}
			assert.InDelta(t, rate, float64(count)/float64(iterations), 0.01)
		})
	}
}

func BenchmarkShouldSample(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		mh := newMetricHandler(newBufferPool(1, 1, 1), nil)
		for pb.Next() {
			shouldSample(0.1, mh.random, &mh.randomLock)
		}
	})
}

func initMetricHandler(bufferSize int) (*bufferPool, *sender, *metricHandler) {
	pool := newBufferPool(10, bufferSize, 5)
	// manually create the sender so the sender loop is not started. All we
	// need is the queue
	s := &sender{
		queue: make(chan *statsdBuffer, 10),
		pool:  pool,
	}

	w := newMetricHandler(pool, s)
	return pool, s, w
}

func testMetricHandler(t *testing.T, m metric, expectedBuffer string) {
	_, s, w := initMetricHandler(100)

	err := w.processMetric(m)
	assert.Nil(t, err)

	w.flush()
	data := <-s.queue
	assert.Equal(t, expectedBuffer, string(data.buffer))

}

func TestMetricHandlerGauge(t *testing.T) {
	testMetricHandler(
		t,
		metric{
			metricType: gauge,
			namespace:  "namespace.",
			globalTags: []string{"globalTags", "globalTags2"},
			name:       "test_gauge",
			fvalue:     21,
			tags:       []string{"tag1", "tag2"},
			rate:       1,
		},
		"namespace.test_gauge:21|g|#globalTags,globalTags2,tag1,tag2\n",
	)
}

func TestMetricHandlerCount(t *testing.T) {
	testMetricHandler(
		t,
		metric{
			metricType: count,
			namespace:  "namespace.",
			globalTags: []string{"globalTags", "globalTags2"},
			name:       "test_count",
			ivalue:     21,
			tags:       []string{"tag1", "tag2"},
			rate:       1,
		},
		"namespace.test_count:21|c|#globalTags,globalTags2,tag1,tag2\n",
	)
}

func TestMetricHandlerHistogram(t *testing.T) {
	testMetricHandler(
		t,
		metric{
			metricType: histogram,
			namespace:  "namespace.",
			globalTags: []string{"globalTags", "globalTags2"},
			name:       "test_histogram",
			fvalue:     21,
			tags:       []string{"tag1", "tag2"},
			rate:       1,
		},
		"namespace.test_histogram:21|h|#globalTags,globalTags2,tag1,tag2\n",
	)
}

func TestMetricHandlerDistribution(t *testing.T) {
	testMetricHandler(
		t,
		metric{
			metricType: distribution,
			namespace:  "namespace.",
			globalTags: []string{"globalTags", "globalTags2"},
			name:       "test_distribution",
			fvalue:     21,
			tags:       []string{"tag1", "tag2"},
			rate:       1,
		},
		"namespace.test_distribution:21|d|#globalTags,globalTags2,tag1,tag2\n",
	)
}

func TestMetricHandlerSet(t *testing.T) {
	testMetricHandler(
		t,
		metric{
			metricType: set,
			namespace:  "namespace.",
			globalTags: []string{"globalTags", "globalTags2"},
			name:       "test_set",
			svalue:     "value:1",
			tags:       []string{"tag1", "tag2"},
			rate:       1,
		},
		"namespace.test_set:value:1|s|#globalTags,globalTags2,tag1,tag2\n",
	)
}

func TestMetricHandlerTiming(t *testing.T) {
	testMetricHandler(
		t,
		metric{
			metricType: timing,
			namespace:  "namespace.",
			globalTags: []string{"globalTags", "globalTags2"},
			name:       "test_timing",
			fvalue:     1.2,
			tags:       []string{"tag1", "tag2"},
			rate:       1,
		},
		"namespace.test_timing:1.200000|ms|#globalTags,globalTags2,tag1,tag2\n",
	)
}

func TestMetricHandlerHistogramAggregated(t *testing.T) {
	testMetricHandler(
		t,
		metric{
			metricType: histogramAggregated,
			namespace:  "namespace.",
			globalTags: []string{"globalTags", "globalTags2"},
			name:       "test_histogram",
			fvalues:    []float64{1.2},
			stags:      "tag1,tag2",
			rate:       1,
		},
		"namespace.test_histogram:1.2|h|#globalTags,globalTags2,tag1,tag2\n",
	)
}

func TestMetricHandlerHistogramAggregatedMultiple(t *testing.T) {
	_, s, w := initMetricHandler(100)

	m := metric{
		metricType: histogramAggregated,
		namespace:  "namespace.",
		globalTags: []string{"globalTags", "globalTags2"},
		name:       "test_histogram",
		fvalues:    []float64{1.1, 2.2, 3.3, 4.4},
		stags:      "tag1,tag2",
		rate:       1,
	}
	err := w.processMetric(m)
	assert.Nil(t, err)

	w.flush()
	data := <-s.queue
	assert.Equal(t, "namespace.test_histogram:1.1:2.2:3.3:4.4|h|#globalTags,globalTags2,tag1,tag2\n", string(data.buffer))

	// reducing buffer size so not all values fit in one packet
	_, s, w = initMetricHandler(70)

	err = w.processMetric(m)
	assert.Nil(t, err)

	w.flush()
	data = <-s.queue
	assert.Equal(t, "namespace.test_histogram:1.1:2.2|h|#globalTags,globalTags2,tag1,tag2\n", string(data.buffer))
	data = <-s.queue
	assert.Equal(t, "namespace.test_histogram:3.3:4.4|h|#globalTags,globalTags2,tag1,tag2\n", string(data.buffer))
}

func TestMetricHandlerDistributionAggregated(t *testing.T) {
	testMetricHandler(
		t,
		metric{
			metricType: distributionAggregated,
			namespace:  "namespace.",
			globalTags: []string{"globalTags", "globalTags2"},
			name:       "test_distribution",
			fvalues:    []float64{1.2},
			stags:      "tag1,tag2",
			rate:       1,
		},
		"namespace.test_distribution:1.2|d|#globalTags,globalTags2,tag1,tag2\n",
	)
}

func TestMetricHandlerDistributionAggregatedMultiple(t *testing.T) {
	_, s, w := initMetricHandler(100)

	m := metric{
		metricType: distributionAggregated,
		namespace:  "namespace.",
		globalTags: []string{"globalTags", "globalTags2"},
		name:       "test_distribution",
		fvalues:    []float64{1.1, 2.2, 3.3, 4.4},
		stags:      "tag1,tag2",
		rate:       1,
	}
	err := w.processMetric(m)
	assert.Nil(t, err)

	w.flush()
	data := <-s.queue
	assert.Equal(t, "namespace.test_distribution:1.1:2.2:3.3:4.4|d|#globalTags,globalTags2,tag1,tag2\n", string(data.buffer))

	// reducing buffer size so not all values fit in one packet
	_, s, w = initMetricHandler(72)

	err = w.processMetric(m)
	assert.Nil(t, err)

	w.flush()
	data = <-s.queue
	assert.Equal(t, "namespace.test_distribution:1.1:2.2|d|#globalTags,globalTags2,tag1,tag2\n", string(data.buffer))
	data = <-s.queue
	assert.Equal(t, "namespace.test_distribution:3.3:4.4|d|#globalTags,globalTags2,tag1,tag2\n", string(data.buffer))
}

func TestMetricHandlerMultipleDifferentDistributionAggregated(t *testing.T) {
	// first metric will fit but not the second one
	_, s, w := initMetricHandler(160)

	m := metric{
		metricType: distributionAggregated,
		namespace:  "namespace.",
		globalTags: []string{"globalTags", "globalTags2"},
		name:       "test_distribution",
		fvalues:    []float64{1.1, 2.2, 3.3, 4.4},
		stags:      "tag1,tag2",
		rate:       1,
	}
	err := w.processMetric(m)
	assert.Nil(t, err)
	m = metric{
		metricType: distributionAggregated,
		namespace:  "namespace.",
		globalTags: []string{"globalTags", "globalTags2"},
		name:       "test_distribution_2",
		fvalues:    []float64{1.1, 2.2, 3.3, 4.4},
		stags:      "tag1,tag2",
		rate:       1,
	}
	err = w.processMetric(m)
	assert.Nil(t, err)

	w.flush()
	data := <-s.queue
	assert.Equal(t, "namespace.test_distribution:1.1:2.2:3.3:4.4|d|#globalTags,globalTags2,tag1,tag2\nnamespace.test_distribution_2:1.1:2.2:3.3|d|#globalTags,globalTags2,tag1,tag2\n", string(data.buffer))
	data = <-s.queue
	assert.Equal(t, "namespace.test_distribution_2:4.4|d|#globalTags,globalTags2,tag1,tag2\n", string(data.buffer))
}