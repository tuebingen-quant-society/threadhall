package agentd

import (
	"context"
	"fmt"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

type CapabilityCatalog interface {
	Discover(context.Context) ([]agenttask.Capability, error)
}

type CapabilitySink interface {
	SyncCapabilities(context.Context, []agenttask.Capability) error
}

func SyncRuntimeCapabilities(ctx context.Context, catalog CapabilityCatalog, sink CapabilitySink) error {
	items, err := catalog.Discover(ctx)
	if err != nil {
		return fmt.Errorf("discover Codex capabilities: %w", err)
	}
	if err := sink.SyncCapabilities(ctx, items); err != nil {
		return fmt.Errorf("publish Codex capabilities: %w", err)
	}
	return nil
}
