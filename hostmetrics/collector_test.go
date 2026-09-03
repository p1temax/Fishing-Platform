package hostmetrics

import (
	"testing"
	"time"

	"fishing-platform-backend/models"
)

func TestDownsampleHostMetricSamples(t *testing.T) {
	now := time.Now()
	in := make([]models.HostMetricSample, 0, 10)
	for i := 0; i < 10; i++ {
		in = append(in, models.HostMetricSample{
			ID:          uint(i + 1),
			CollectedAt: now.Add(time.Duration(i) * time.Minute),
		})
	}
	got := downsample(in, 4)
	if len(got) != 4 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ID != 1 || got[3].ID != 10 {
		t.Fatalf("endpoints = %d,%d", got[0].ID, got[3].ID)
	}
	if len(downsample(in, 100)) != 10 {
		t.Fatal("should keep all when under max")
	}
}
