package outbox

import (
	"context"
	"testing"

	"bidding-service/internal/clients"
	"bidding-service/internal/domain"
)

type fakeOutbox struct {
	events    []*domain.OutboxEvent
	processed []uint
}

func (f *fakeOutbox) Save(context.Context, *domain.OutboxEvent) error { return nil }
func (f *fakeOutbox) GetUnprocessed(context.Context, int) ([]*domain.OutboxEvent, error) {
	return f.events, nil
}
func (f *fakeOutbox) MarkProcessed(_ context.Context, ids []uint) error {
	f.processed = append(f.processed, ids...)
	return nil
}

type fakeReleaser struct {
	err  map[string]error
	seen []string
}

func (f *fakeReleaser) Release(_ context.Context, id string) error {
	f.seen = append(f.seen, id)
	return f.err[id]
}

// docs/23 §5.2: outbox escrow.release gọi thẳng auth-service. Nhả được, hoặc khoản
// đã chốt từ trước (409) → xong; lỗi tạm thời → để lại, vòng sau thử tiếp.
func TestPublisherReleasesOutbidHolds(t *testing.T) {
	ob := &fakeOutbox{events: []*domain.OutboxEvent{
		{ID: 1, Topic: "escrow.release", Payload: []byte(`{"hold_id":"bid-1"}`)},
		{ID: 2, Topic: "escrow.release", Payload: []byte(`{"hold_id":"bid-2"}`)},
		{ID: 3, Topic: "escrow.release", Payload: []byte(`{"hold_id":"bid-3"}`)},
	}}
	rel := &fakeReleaser{err: map[string]error{
		"bid-2": clients.ErrAlreadySettled,
		"bid-3": &clients.APIError{Status: 503},
	}}
	NewPublisher(ob, nil, nil, rel).processEvents(context.Background())
	if len(rel.seen) != 3 {
		t.Fatalf("phải gọi auth-service cho cả 3 khoản, nhận %v", rel.seen)
	}
	if len(ob.processed) != 2 || ob.processed[0] != 1 || ob.processed[1] != 2 {
		t.Fatalf("chỉ đánh dấu xong 1 và 2, nhận %v", ob.processed)
	}
}
