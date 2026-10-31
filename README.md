# exponential-histogram-bucket

A fixed-size bucket window for computing exponentially decaying percentiles over streaming latency data. Each sample carries a caller-supplied timestamp; weights decay exponentially with time, and the bucket count is bounded by merging the two oldest buckets when capacity is reached.

## Usage

```go
package main

import (
	"fmt"

	ehb "your-module/exponential-histogram-bucket"
)

func main() {
	h, err := ehb.New(maxBuckets: 8, decay: 0.5)
	if err != nil {
		panic(err)
	}
	h.Add(0.0, 42.0)   // time, latency_ms
	h.Add(1.0, 55.0)
	h.Add(2.0, 38.0)

	p50, err := h.Percentile(50)
	if err != nil {
		panic(err)
	}
	fmt.Printf("p50 = %v\n", p50)
}
```

Replace `your-module` with the import path of the module declared in `go.mod`.

## Why

Streaming latency monitors want a recent-moment percentile without keeping every sample. A bounded bucket window with exponential decay approximates a sliding window whose "memory length" is controlled by the decay factor. The trade-off here is simplicity over tight error bounds: the library caps the bucket count by merging the two oldest adjacent buckets when the capacity is reached, which is cheap and deterministic but introduces a small averaging error when the window is full and highly skewed. For high-cardinality production telemetry that error is acceptable and often preferable to unbounded growth.

The `Add` method takes `time` as a plain `float64` rather than reading a wall clock so tests can be deterministic. Pass whatever monotonic or epoch scale your application uses; the only constraint is that successive `Add` calls use a consistent scale.

## Edge cases

- `New` requires `maxBuckets >= 2` and `0 < decay < 1`. A decay of `1.0` is rejected because it would make the window grow without bound.
- `Percentile` on an empty histogram returns `ErrNotEnoughSamples`.
- `Percentile` uses the lower interpolation rule: the first sample whose cumulative decayed weight meets or exceeds the target percentile is returned. This means `p50` of two equal-weight samples `{1, 3}` returns `1`, not `2`. This choice is documented rather than configurable.
- `Add` does not enforce monotonic time; if you insert samples with non-monotonic timestamps the decay weights will be inconsistent with a real time-ordered stream. The caller is responsible for time ordering.
