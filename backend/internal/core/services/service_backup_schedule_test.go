package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNextRunAt(t *testing.T) {
	loc := time.Local

	tests := []struct {
		name      string
		frequency string
		timeOfDay string
		from      time.Time
		want      time.Time
	}{
		{
			name:      "daily later today",
			frequency: FrequencyDaily,
			timeOfDay: "15:30",
			from:      time.Date(2026, 7, 24, 10, 0, 0, 0, loc),
			want:      time.Date(2026, 7, 24, 15, 30, 0, 0, loc),
		},
		{
			name:      "daily already passed rolls to tomorrow",
			frequency: FrequencyDaily,
			timeOfDay: "03:00",
			from:      time.Date(2026, 7, 24, 10, 0, 0, 0, loc),
			want:      time.Date(2026, 7, 25, 3, 0, 0, 0, loc),
		},
		{
			name:      "daily exact same minute rolls to tomorrow",
			frequency: FrequencyDaily,
			timeOfDay: "10:00",
			from:      time.Date(2026, 7, 24, 10, 0, 0, 0, loc),
			want:      time.Date(2026, 7, 25, 10, 0, 0, 0, loc),
		},
		{
			name:      "weekly later today",
			frequency: FrequencyWeekly,
			timeOfDay: "23:59",
			from:      time.Date(2026, 7, 24, 10, 0, 0, 0, loc), // Friday
			want:      time.Date(2026, 7, 24, 23, 59, 0, 0, loc),
		},
		{
			name:      "weekly already passed rolls to same weekday next week",
			frequency: FrequencyWeekly,
			timeOfDay: "03:00",
			from:      time.Date(2026, 7, 24, 10, 0, 0, 0, loc), // Friday
			want:      time.Date(2026, 7, 31, 3, 0, 0, 0, loc),  // next Friday
		},
		{
			name:      "daily midnight boundary",
			frequency: FrequencyDaily,
			timeOfDay: "00:00",
			from:      time.Date(2026, 7, 24, 0, 0, 1, 0, loc),
			want:      time.Date(2026, 7, 25, 0, 0, 0, 0, loc),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NextRunAt(tc.frequency, tc.timeOfDay, tc.from)
			require.NoError(t, err)
			assert.True(t, got.Equal(tc.want), "got %v, want %v", got, tc.want)
		})
	}
}

func TestNextRunAt_Invalid(t *testing.T) {
	from := time.Date(2026, 7, 24, 10, 0, 0, 0, time.Local)

	for _, tc := range []struct{ frequency, timeOfDay string }{
		{"monthly", "03:00"},
		{FrequencyDaily, "24:00"},
		{FrequencyDaily, "03:60"},
		{FrequencyDaily, "not-a-time"},
		{FrequencyDaily, "3"},
	} {
		_, err := NextRunAt(tc.frequency, tc.timeOfDay, from)
		assert.Error(t, err, "frequency=%q timeOfDay=%q", tc.frequency, tc.timeOfDay)
	}
}
