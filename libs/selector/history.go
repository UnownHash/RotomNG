package selector

import (
	"fmt"
	"time"

	"github.com/UnownHash/RotomNG/libs/settings"
)

// SelectionHistoryConfig holds the configuration for a SelectionHistory.
type SelectionHistoryConfig struct {
	// Enabled controls whether max selections is enforced. When false,
	// IsAtMaxSelections always returns false regardless of recorded selections.
	Enabled bool

	// MaxSelections is the maximum number of selections allowed within Duration.
	MaxSelections int

	// Duration is the sliding-window length. Selections older than Duration
	// are pruned and do not count toward the limit.
	Duration time.Duration
}

// Validate checks that the configuration values are sensible when enabled.
func (cfg SelectionHistoryConfig) Validate() error {
	if cfg.Enabled {
		if cfg.MaxSelections <= 0 {
			return fmt.Errorf("selection history config MaxSelections must be > 0, not %d", cfg.MaxSelections)
		}
		if cfg.Duration <= 0 {
			return fmt.Errorf("selection history config Duration must be > 0, not %s", cfg.Duration)
		}
	}
	return nil
}

// SelectionHistory implements a sliding-window selection limiter.
// It tracks selection timestamps and prunes expired entries on access.
//
// Not thread-safe — the caller must serialize access.
type SelectionHistory struct {
	settingsContainer *settings.Container[SelectionHistoryConfig]
	history           []time.Time
}

// NewSelectionHistory creates a new SelectionHistory with the given Config.
func NewSelectionHistory(settingsContainer *settings.Container[SelectionHistoryConfig]) *SelectionHistory {
	return &SelectionHistory{
		settingsContainer: settingsContainer,
		history:           make([]time.Time, 0),
	}
}

// Prune will remove expired entries.
func (rl *SelectionHistory) Prune(now time.Time) {
	cfg := rl.settingsContainer.GetSettings()
	rl.prune(cfg, now)
}

// Record prunes expired entries then appends now to the history.
func (rl *SelectionHistory) Record(now time.Time) {
	cfg := rl.settingsContainer.GetSettings()
	rl.prune(cfg, now)
	rl.history = append(rl.history, now)
}

// IsAtMaxSelections returns true if max selections have been reached
// within the configured duration and Enabled is true.
func (rl *SelectionHistory) IsAtMaxSelections(now time.Time) bool {
	cfg := rl.settingsContainer.GetSettings()
	rl.prune(cfg, now)
	if cfg.Enabled {
		return len(rl.history) >= cfg.MaxSelections
	}
	return false
}

// Reset removes all history.
func (rl *SelectionHistory) Reset() {
	rl.history = make([]time.Time, 0)
}

// UpdateConfig replaces the current configuration with cfg.
func (rl *SelectionHistory) UpdateConfig(cfg SelectionHistoryConfig) error {
	err := rl.settingsContainer.PutSettings(cfg)
	if err != nil {
		return fmt.Errorf("selection history failed to update config: %w", err)
	}
	return nil
}

// prune removes entries from history that are outside the current window.
func (rl *SelectionHistory) prune(cfg SelectionHistoryConfig, now time.Time) {
	entries := rl.history
	if len(entries) == 0 {
		return
	}
	cutoff := now.Add(-cfg.Duration)
	for idx, t := range entries {
		if t.After(cutoff) {
			rl.history = entries[idx:]
			return
		}
	}
	// All entries expired; keep the key but reset to empty slice.
	rl.history = entries[:0]
}
