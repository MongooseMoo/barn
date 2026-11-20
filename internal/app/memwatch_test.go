package app

import "testing"

// The watchdog fires once per doubling of the live heap over its baseline:
// first at 2x, then 4x, 8x, ... A jump past several doublings fires once and
// re-arms at the next doubling above the reading.
func TestHeapTripRule(t *testing.T) {
	const mb = 1 << 20
	for _, tc := range []struct {
		name      string
		live      uint64
		threshold uint64
		trip      bool
		next      uint64
	}{
		{"below threshold", 150 * mb, 200 * mb, false, 200 * mb},
		{"at threshold", 200 * mb, 200 * mb, true, 400 * mb},
		{"between doublings", 300 * mb, 400 * mb, false, 400 * mb},
		{"next doubling", 450 * mb, 400 * mb, true, 800 * mb},
		{"jump past several doublings", 1700 * mb, 400 * mb, true, 3200 * mb},
		{"heap shrank", 50 * mb, 800 * mb, false, 800 * mb},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trip, next := heapTrip(tc.live, tc.threshold)
			if trip != tc.trip || next != tc.next {
				t.Errorf("heapTrip(%d, %d) = %v, %d; want %v, %d", tc.live, tc.threshold, trip, next, tc.trip, tc.next)
			}
		})
	}
}
