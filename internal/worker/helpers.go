package worker

import (
	"github.com/namlee0005/liquidity-guard-bot/internal/engine"
	"github.com/namlee0005/liquidity-guard-bot/internal/models"
)

// SpreadBoundsFromModel converts the models.SpreadConfig (MongoDB document type)
// to the engine.SpreadBounds used by SpreadCalculator.
func SpreadBoundsFromModel(s models.SpreadConfig) engine.SpreadBounds {
	return engine.SpreadBounds{
		MinPct: s.MinSpreadPct,
		MaxPct: s.MaxSpreadPct,
	}
}
