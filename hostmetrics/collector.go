package hostmetrics

import (
	"context"
	"log"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
)

const (
	SampleInterval = 30 * time.Second
	Retention      = 24 * time.Hour
	cleanupEvery   = 20
	// Max chart points returned to the dashboard (evenly spaced over 24h).
	MaxChartPoints = 96
)

// Start begins background sampling until ctx is cancelled.
func Start(ctx context.Context) {
	go func() {
		if err := RecordSample(); err != nil {
			log.Printf("host metrics: initial sample failed: %v", err)
		}

		ticker := time.NewTicker(SampleInterval)
		defer ticker.Stop()
		n := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := RecordSample(); err != nil {
					log.Printf("host metrics: sample failed: %v", err)
				}
				n++
				if n%cleanupEvery == 0 {
					if err := Cleanup(); err != nil {
						log.Printf("host metrics: cleanup failed: %v", err)
					}
				}
			}
		}
	}()
}

// RecordSample collects current host metrics and persists one row.
func RecordSample() error {
	db := config.GetDB()
	if db == nil {
		return nil
	}
	m := utils.CollectHostMetrics()
	collectedAt := time.Now()
	if parsed, err := time.Parse(time.RFC3339, m.CollectedAt); err == nil {
		collectedAt = parsed
	}
	row := models.HostMetricSample{
		CollectedAt:        collectedAt,
		CPUUsagePercent:    m.CPUUsagePercent,
		MemoryUsagePercent: m.MemoryUsagePercent,
		DiskUsagePercent:   m.DiskUsagePercent,
		MemoryTotalMB:      m.MemoryTotalMB,
		MemoryUsedMB:       m.MemoryUsedMB,
		DiskTotalGB:        m.DiskTotalGB,
		DiskUsedGB:         m.DiskUsedGB,
	}
	return db.Create(&row).Error
}

// Cleanup deletes samples older than Retention.
func Cleanup() error {
	db := config.GetDB()
	if db == nil {
		return nil
	}
	cutoff := time.Now().Add(-Retention)
	return db.Where("collected_at < ?", cutoff).Delete(&models.HostMetricSample{}).Error
}

// ListRecent returns samples from the retention window, oldest first, downsampled for charts.
func ListRecent() ([]models.HostMetricSample, error) {
	db := config.GetDB()
	if db == nil {
		return nil, nil
	}
	since := time.Now().Add(-Retention)
	var rows []models.HostMetricSample
	if err := db.Where("collected_at >= ?", since).
		Order("collected_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return downsample(rows, MaxChartPoints), nil
}

func downsample(samples []models.HostMetricSample, max int) []models.HostMetricSample {
	if max <= 0 || len(samples) <= max {
		return samples
	}
	if max == 1 {
		return []models.HostMetricSample{samples[len(samples)-1]}
	}
	out := make([]models.HostMetricSample, 0, max)
	last := len(samples) - 1
	for i := 0; i < max; i++ {
		idx := i * last / (max - 1)
		out = append(out, samples[idx])
	}
	return out
}
