package exponential_histogram_bucket

import (
	"math"
	"testing"
)

const eps = 1e-9

func floatEq(a, b float64) bool { return math.Abs(a-b) < eps }

func TestNewInvalidArgs(t *testing.T) {
	if _, err := New(0, 0.5); err != ErrInvalidMaxBuckets {
		t.Fatalf("New(0,...) err = %v, want %v", err, ErrInvalidMaxBuckets)
	}
	if _, err := New(1, 0.5); err != ErrInvalidMaxBuckets {
		t.Fatalf("New(1,...) err = %v, want %v", err, ErrInvalidMaxBuckets)
	}
	if _, err := New(4, 0); err != ErrInvalidDecay {
		t.Fatalf("New(4, 0) err = %v, want %v", err, ErrInvalidDecay)
	}
	if _, err := New(4, -0.5); err != ErrInvalidDecay {
		t.Fatalf("New(4, -0.5) err = %v, want %v", err, ErrInvalidDecay)
	}
	if _, err := New(4, 1.0); err != ErrInvalidDecay {
		t.Fatalf("New(4, 1.0) err = %v, want %v", err, ErrInvalidDecay)
	}
}

func TestNewValid(t *testing.T) {
	h, err := New(8, 0.9)
	if err != nil {
		t.Fatalf("New(8, 0.9) err = %v, want nil", err)
	}
	if h == nil {
		t.Fatal("histogram is nil")
	}
	if got := h.TotalWeight(); !floatEq(got, 0) {
		t.Fatalf("empty TotalWeight = %v, want 0", got)
	}
	if got := h.TotalCount(); got != 0 {
		t.Fatalf("empty TotalCount = %d, want 0", got)
	}
}

func TestEmptyPercentile(t *testing.T) {
	h, _ := New(4, 0.5)
	if _, err := h.Percentile(50); err != ErrNotEnoughSamples {
		t.Fatalf("empty Percentile err = %v, want %v", err, ErrNotEnoughSamples)
	}
}

func TestPercentileInvalidRange(t *testing.T) {
	h, _ := New(4, 0.5)
	h.Add(0, 1.0)
	if _, err := h.Percentile(-1); err != ErrInvalidPercentile {
		t.Fatalf("Percentile(-1) err = %v, want %v", err, ErrInvalidPercentile)
	}
	if _, err := h.Percentile(101); err != ErrInvalidPercentile {
		t.Fatalf("Percentile(101) err = %v, want %v", err, ErrInvalidPercentile)
	}
}

func TestAddSingleSample(t *testing.T) {
	h, _ := New(4, 0.5)
	h.Add(0, 10.0)

	if got := h.TotalCount(); got != 1 {
		t.Fatalf("TotalCount = %d, want 1", got)
	}
	if !floatEq(h.TotalWeight(), 1.0) {
		t.Fatalf("TotalWeight = %v, want 1.0", h.TotalWeight())
	}

	for _, p := range []float64{0, 25, 50, 100} {
		got, err := h.Percentile(p)
		if err != nil {
			t.Fatalf("Percentile(%v) err = %v", p, err)
		}
		if !floatEq(got, 10.0) {
			t.Fatalf("Percentile(%v) = %v, want 10.0", p, got)
		}
	}
}

func TestAddTwoSamplesEqualWeight(t *testing.T) {
	h, _ := New(4, 0.5)
	h.Add(0, 1.0)
	h.Add(0, 3.0)

	if !floatEq(h.TotalWeight(), 2.0) {
		t.Fatalf("TotalWeight = %v, want 2.0", h.TotalWeight())
	}
	if got, _ := h.Percentile(0); !floatEq(got, 1.0) {
		t.Fatalf("p0 = %v, want 1.0", got)
	}
	if got, _ := h.Percentile(50); !floatEq(got, 1.0) {
		t.Fatalf("p50 = %v, want 1.0", got)
	}
	if got, _ := h.Percentile(100); !floatEq(got, 3.0) {
		t.Fatalf("p100 = %v, want 3.0", got)
	}
}

func TestBucketMergeOnCap(t *testing.T) {
	h, _ := New(2, 0.5)
	h.Add(0, 1.0)
	h.Add(0, 2.0)
	h.Add(0, 3.0)

	if got := len(h.buckets); got != 2 {
		t.Fatalf("len(buckets) = %d, want 2", got)
	}
	if got := h.TotalCount(); got != 3 {
		t.Fatalf("TotalCount = %d, want 3", got)
	}
	// Mean of (1, 2) merged is 1.5, single sample is 3.0. With equal weights
	// 2 and 1, p50 target is 2.0 of 3.0 total — lands in the merged bucket.
	got, _ := h.Percentile(50)
	if !floatEq(got, 1.5) {
		t.Fatalf("p50 after merge = %v, want 1.5", got)
	}
}

func TestDecayReducesWeightOverTime(t *testing.T) {
	h, _ := New(8, 0.5)
	h.Add(0, 10.0) // weight = 1.0
	h.Add(1, 10.0) // weight = 0.5

	tw := h.TotalWeight()
	if !floatEq(tw, 1.5) {
		t.Fatalf("TotalWeight = %v, want 1.5", tw)
	}

	// p50 target is 0.75 of 1.5 = 0.75. Cumulative sorted weight crosses at
	// the second sample (weight 1.0 > 0.75).
	got, _ := h.Percentile(50)
	if !floatEq(got, 10.0) {
		t.Fatalf("p50 = %v, want 10.0", got)
	}
}

func TestPercentileSelectsLowerInterpolated(t *testing.T) {
	h, _ := New(8, 0.5)
	h.Add(0, 5.0) // weight 1.0
	h.Add(0, 15.0) // weight 1.0, total = 2.0

	// p50 target is exactly 1.0 (first sample boundary). Lower rule: pick
	// first sample that meets or exceeds target.
	got, _ := h.Percentile(50)
	if !floatEq(got, 5.0) {
		t.Fatalf("p50 = %v, want 5.0 (lower rule)", got)
	}
	got, _ = h.Percentile(49.999)
	if !floatEq(got, 5.0) {
		t.Fatalf("p49.999 = %v, want 5.0", got)
	}
	got, _ = h.Percentile(50.001)
	if !floatEq(got, 15.0) {
		t.Fatalf("p50.001 = %v, want 15.0", got)
	}
}

func TestStressMergePreservesCount(t *testing.T) {
	h, _ := New(3, 0.9)
	for i := 0; i < 100; i++ {
		h.Add(float64(i), float64(i+1))
	}
	if got := h.TotalCount(); got != 100 {
		t.Fatalf("TotalCount = %d, want 100", got)
	}
	if got := len(h.buckets); got > 3 {
		t.Fatalf("len(buckets) = %d, exceeds cap 3", got)
	}
	// Sanity: weighted median should be between min and max of inserted.
	got, err := h.Percentile(50)
	if err != nil {
		t.Fatalf("p50 err = %v", err)
	}
	if got < 1 || got > 100 {
		t.Fatalf("p50 = %v, outside inserted range", got)
	}
}

func TestBoundaryPercentilesZeroAndHundred(t *testing.T) {
	h, _ := New(4, 0.9)
	h.Add(0, 7.0)
	h.Add(0, 3.0)
	h.Add(0, 11.0)
	if got, _ := h.Percentile(0); !floatEq(got, 3.0) {
		t.Fatalf("p0 = %v, want 3.0", got)
	}
	if got, _ := h.Percentile(100); !floatEq(got, 11.0) {
		t.Fatalf("p100 = %v, want 11.0", got)
	}
}
