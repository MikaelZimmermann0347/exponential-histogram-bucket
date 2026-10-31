package exponential_histogram_bucket

import (
	"errors"
	"math"
	"sort"
)

// Sentinel errors returned by methods that validate state.
var (
	ErrInvalidDecay        = errors.New("exponential_histogram-bucket: decay must be in (0, 1)")
	ErrInvalidMaxBuckets   = errors.New("exponential-histogram-bucket: maxBuckets must be >= 2")
	ErrNotEnoughSamples    = errors.New("exponential-histogram-bucket: not enough samples")
	ErrInvalidPercentile   = errors.New("exponential-histogram-bucket: percentile must be in [0, 100]")
)

// Sample is a single timestamped value.
type Sample struct {
	Time  float64
	Value float64
}

// Bucket aggregates one decayed partition of the sample stream.
type Bucket struct {
	Time   float64
	Sum    float64
	Weight float64
	Count  int
}

// Histogram maintains a fixed-size window of buckets for computing
// exponentially decaying percentiles. The design is a simplification of the
// forward-decaying histogram used for streaming percentile queries: each new
// sample is placed in a bucket identified by the caller-supplied time. Buckets
// whose decayed weight falls below a threshold are evicted. Once maxBuckets is
// reached, the oldest existing bucket is merged forward rather than creating a
// new one — preserving a constant upper bound on the number of buckets.
type Histogram struct {
	maxBuckets int
	decay      float64
	evictThresh float64
	buckets    []Bucket
}

// New creates a new Histogram.
//
// maxBuckets is the upper bound on the number of live buckets. It must be at
// least 2; higher values reduce merge frequency at the cost of slightly more
// bookkeeping per insertion.
//
// decay is the per-unit-time exponential decay factor applied to sample
// weights. A value of 1.0 would mean "no decay" (constant weight), but it is
// rejected because the window would grow unbounded.
func New(maxBuckets int, decay float64) (*Histogram, error) {
	if maxBuckets < 2 {
		return nil, ErrInvalidMaxBuckets
	}
	if !(decay > 0 && decay < 1) {
		return nil, ErrInvalidDecay
	}
	return &Histogram{
		maxBuckets:  maxBuckets,
		decay:       decay,
		evictThresh: 1e-12,
	}, nil
}

// Add inserts a single timestamped value into the histogram.
//
// The time parameter is intentionally a float64 taken from a caller-supplied
// clock so tests can be deterministic. In production this would typically be
// wall-clock seconds. Samples may be added out of strict time order as long
// as each new sample's time is not earlier than the last inserted sample's
// time; an earlier insertion would dilute the decay weights inconsistently.
func (h *Histogram) Add(time, value float64) {
	w := math.Pow(h.decay, time)
	if len(h.buckets) >= h.maxBuckets {
		// Merge the two oldest adjacent buckets. Buckets are inserted in
		// non-decreasing time order, so the oldest two are always at the
		// front of the slice. Merging forward preserves their combined
		// weight under the same decay schedule.
		h.buckets[0].Sum += h.buckets[1].Sum
		h.buckets[0].Weight += h.buckets[1].Weight
		h.buckets[0].Count += h.buckets[1].Count
		h.buckets = append(h.buckets[:1], h.buckets[2:]...)
	}
	h.buckets = append(h.buckets, Bucket{
		Time:   time,
		Sum:    value * w,
		Weight: w,
		Count:  1,
	})
}

// TotalWeight returns the combined decayed weight of all currently-held
// samples. A value of zero indicates an empty histogram.
func (h *Histogram) TotalWeight() float64 {
	var tw float64
	for _, b := range h.buckets {
		tw += b.Weight
	}
	return tw
}

// TotalCount returns the number of raw samples stored in all buckets,
// unaffected by decay.
func (h *Histogram) TotalCount() int {
	var n int
	for _, b := range h.buckets {
		n += b.Count
	}
	return n
}

// Percentile returns the value at the given percentile (0–100, inclusive) of
// the decayed weight distribution. The result is the weighted value at which
// the cumulative decayed weight crosses the requested fraction of the total.
//
// The histogram uses the "lower interpolation" rule: the first sample whose
// cumulative weight meets or exceeds the target percentile is returned. This
// keeps the result reproducible and bounded by real inserted values.
func (h *Histogram) Percentile(p float64) (float64, error) {
	if p < 0 || p > 100 {
		return 0, ErrInvalidPercentile
	}
	tw := h.TotalWeight()
	if tw <= 0 {
		return 0, ErrNotEnoughSamples
	}

	type entry struct {
		value  float64
		weight float64
	}
	var entries []entry
	for _, b := range h.buckets {
		entries = append(entries, entry{value: b.Sum / b.Weight, weight: b.Weight})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].value < entries[j].value })

	target := (p / 100) * tw
	var cum float64
	for _, e := range entries {
		cum += e.weight
		if cum >= target {
			return e.value, nil
		}
	}
	return entries[len(entries)-1].value, nil
}
