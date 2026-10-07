package main

import (
	"testing"
	"time"

	"github.com/MongooseMoo/barn/builtins"
	"github.com/MongooseMoo/barn/types"
)

func TestTZOffsetMatchesHelper(t *testing.T) {
	fn, ok := builtins.NewRegistry().Get("tz_offset")
	if !ok {
		t.Fatal("tz_offset is not registered")
	}
	for _, date := range []time.Time{
		time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC),
	} {
		for _, zone := range []string{"UTC", "America/Edmonton", "America/St_Johns", "Asia/Kathmandu", "Europe/London", "Australia/Lord_Howe"} {
			t.Run(zone+date.Format(time.RFC3339), func(t *testing.T) {
				want, err := offset(date, zone)
				if err != nil {
					t.Fatal(err)
				}
				result := fn(nil, []types.Value{types.NewStr(zone), types.NewInt(date.Unix())})
				if result.Flow != types.FlowNormal || result.Val.Type() != types.TYPE_STR || result.Val.Str() != want {
					t.Fatalf("tz_offset = %+v; helper offset = %q", result, want)
				}
			})
		}
	}
}

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
