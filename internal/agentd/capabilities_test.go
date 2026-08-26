package agentd

import (
	"context"
	"errors"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

type catalogStub struct {
	items []agenttask.Capability
	err   error
}

func (stub catalogStub) Discover(context.Context) ([]agenttask.Capability, error) {
	return stub.items, stub.err
}

type capabilitySinkStub struct {
	items []agenttask.Capability
}

func (stub *capabilitySinkStub) SyncCapabilities(_ context.Context, items []agenttask.Capability) error {
	stub.items = items
	return nil
}

func TestSyncRuntimeCapabilitiesPublishesDiscoveredCatalog(t *testing.T) {
	want := []agenttask.Capability{{Kind: "plugin", ID: "drive", Name: "Google Drive"}}
	sink := &capabilitySinkStub{}

	if err := SyncRuntimeCapabilities(context.Background(), catalogStub{items: want}, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 1 || sink.items[0].ID != "drive" {
		t.Fatalf("synced capabilities = %#v", sink.items)
	}
}

func TestSyncRuntimeCapabilitiesDoesNotEraseCatalogOnDiscoveryFailure(t *testing.T) {
	sink := &capabilitySinkStub{}
	err := SyncRuntimeCapabilities(context.Background(), catalogStub{err: errors.New("unavailable")}, sink)
	if err == nil || sink.items != nil {
		t.Fatalf("err = %v, synced capabilities = %#v", err, sink.items)
	}
}
