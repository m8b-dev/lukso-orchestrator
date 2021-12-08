package events

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/event"
	eventTypes "github.com/lukso-network/lukso-orchestrator/shared/types"
	"time"
)

var (
	deadline = 5 * time.Minute
)

type MockBackend struct {
	ConsensusInfoFeed    event.Feed
	verifiedSlotInfoFeed event.Feed

	ConsensusInfos    []*eventTypes.MinimalEpochConsensusInfoV2
	verifiedSlotInfos map[uint64]*eventTypes.SlotInfo
	CurEpoch          uint64
}

var _ Backend = &MockBackend{}

func (b *MockBackend) ConsensusInfoByEpochRange(fromEpoch uint64) ([]*eventTypes.MinimalEpochConsensusInfoV2, error) {
	consensusInfos := make([]*eventTypes.MinimalEpochConsensusInfoV2, 0)
	for _, consensusInfo := range b.ConsensusInfos {
		consensusInfos = append(consensusInfos, consensusInfo)
	}
	return consensusInfos, nil
}

func (b *MockBackend) SubscribeNewEpochEvent(ch chan<- *eventTypes.MinimalEpochConsensusInfoV2) event.Subscription {
	return b.ConsensusInfoFeed.Subscribe(ch)
}

func (b *MockBackend) SubscribeNewVerifiedSlotInfoEvent(ch chan<- *eventTypes.SlotInfoWithStatus) event.Subscription {
	return b.verifiedSlotInfoFeed.Subscribe(ch)
}

func (mb *MockBackend) GetSlotStatus(ctx context.Context, slot uint64, hash common.Hash, requestType bool) eventTypes.Status {
	return eventTypes.Pending
}

func (mb *MockBackend) LatestEpoch() uint64 {
	return 100
}

func (mb *MockBackend) VerifiedSlotInfos(fromSlot uint64) map[uint64]*eventTypes.SlotInfo {
	slotInfos := make(map[uint64]*eventTypes.SlotInfo)
	for slot, slotInfo := range mb.verifiedSlotInfos {
		slotInfos[slot] = slotInfo
	}
	return slotInfos
}

func (mb *MockBackend) LatestVerifiedSlot() uint64 {
	return 100
}

func (mb *MockBackend) LatestFinalizedSlot() uint64 {
	return 100
}
