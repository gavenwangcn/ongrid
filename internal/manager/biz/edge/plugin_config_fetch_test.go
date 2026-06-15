package edge

import (
	"context"
	"testing"
)

type stubHostDeviceLookup struct {
	devID uint64
	err   error
}

func (s stubHostDeviceLookup) LookupHostDevice(_ context.Context, _ uint64) (uint64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.devID, nil
}

func TestFetchForEdge_DeviceIDForLabels(t *testing.T) {
	repo := &fakePluginConfigRepo{}
	uc := NewPluginConfigUC(repo, nil, fakeEndpointResolver{}, nil)
	uc.SetHostDeviceLookup(stubHostDeviceLookup{devID: 1})

	snap, err := uc.FetchForEdge(context.Background(), 99)
	if err != nil {
		t.Fatal(err)
	}
	if snap.EdgeID != 99 {
		t.Fatalf("EdgeID = %d, want 99", snap.EdgeID)
	}
	if snap.DeviceID != 1 {
		t.Fatalf("DeviceID = %d, want 1 (host device for LogQL filters)", snap.DeviceID)
	}
}

func TestFetchForEdge_DeviceIDFallsBackToEdgeID(t *testing.T) {
	repo := &fakePluginConfigRepo{}
	uc := NewPluginConfigUC(repo, nil, fakeEndpointResolver{}, nil)

	snap, err := uc.FetchForEdge(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if snap.DeviceID != 7 {
		t.Fatalf("DeviceID = %d, want 7 when host lookup missing", snap.DeviceID)
	}
}
