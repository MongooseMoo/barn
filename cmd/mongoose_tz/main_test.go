package main

import (
	"testing"
	"time"
)

func TestOffset(t *testing.T) {
	for _, tc := range []struct{ date, zone, want string }{
		{"2026-01-15T12:00:00Z", "America/Denver", "-0700"},
		{"2026-07-15T12:00:00Z", "America/Denver", "-0600"},
		{"2026-09-17T12:00:00Z", "UTC", "+0000"},
		{"2026-09-17T12:00:00Z", "Asia/Kathmandu", "+0545"},
		{"2026-09-17T12:00:00Z", "America/St_Johns", "-0230"},
	} {
		t.Run(tc.zone+tc.date, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.date)
			if err != nil {
				t.Fatal(err)
			}
			got, err := offset(now, tc.zone)
			if err != nil || got != tc.want {
				t.Fatalf("offset = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
