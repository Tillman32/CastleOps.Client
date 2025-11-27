package cache

import (
	"fmt"

	"github.com/castleops/client/internal/config"
	"github.com/rs/zerolog"
)

// New creates a new cache based on the configuration
func New(cfg *config.Config, logger zerolog.Logger) (Cache, error) {
	switch cfg.Cache.Type {
	case "memory", "":
		return NewMemoryCache(MemoryConfig{
			RetentionDays: cfg.Metrics.RetentionDays,
			Logger:        logger,
		}), nil
	case "sqlite":
		// SQLite implementation would go here
		// For now, fall back to memory cache
		logger.Warn().Msg("SQLite cache not yet implemented, using memory cache")
		return NewMemoryCache(MemoryConfig{
			RetentionDays: cfg.Metrics.RetentionDays,
			Logger:        logger,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported cache type: %s", cfg.Cache.Type)
	}
}
