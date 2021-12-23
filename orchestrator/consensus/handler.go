package consensus

import (
	"bytes"
	"github.com/lukso-network/lukso-orchestrator/orchestrator/cache"
	"github.com/lukso-network/lukso-orchestrator/orchestrator/utils"
	"github.com/pkg/errors"
	"time"

	"github.com/ethereum/go-ethereum/common"
	eth1Types "github.com/ethereum/go-ethereum/core/types"
	"github.com/lukso-network/lukso-orchestrator/shared/types"
)

// processPandoraHeader method process incoming pandora shard header from pandora chain
// - First it checks the pandora header hash in verified shard info db. If it's already in db then it's already verified, so return nil
// - If it is not in verified db, then this method finds vanguard shard into pending cache.
// - If vanguard shard is already into pending cache, then calls insertIntoChain method to verify the sharding info and
// checks consecutiveness and trigger reorg if vanguard block's parent hash does not match with latest verified slot's hash
func (s *Service) processPandoraHeader(headerInfo *types.PandoraHeaderInfo) error {
	if headerInfo == nil {
		return nil
	}

	slot := headerInfo.Slot
	// short circuit check, if this header is already in verified sharding info db then send confirmation instantly
	if shardInfo := s.getShardingInfo(slot); shardInfo != nil && shardInfo.NotNil() {
		if shardInfo.GetPanShardRoot() == headerInfo.Header.Hash() {
			log.WithField("shardInfo", shardInfo.FormattedStr()).Debug("Pandora shard header is already in verified shard info db")
			s.publishBlockConfirmation(shardInfo.GetPanShardRoot(), shardInfo.GetVanSlotRoot(), types.Verified)
			return nil
		}
	}

	latestStepId := s.db.LatestStepID()
	latestShardInfo, err := s.db.VerifiedShardInfo(latestStepId)
	if err != nil {
		return errors.Wrap(err, "DB is corrupted! Failed to retrieve latest shard info")
	}

	if latestStepId > 0 && (latestShardInfo == nil || latestShardInfo.IsNil()) {
		return errors.New("nil latest shard info")
	}

	// Checking if current reorg slot contains this pandora block then latest shard info will be updated
	if s.curReorgStatus != nil && s.curReorgStatus.PandoraHash == headerInfo.Header.Hash() {
		log.WithField("reorgStatus", s.curReorgStatus.FormattedStr()).WithField("panParentHash", headerInfo.Header.ParentHash).
			Info("Got new pandora header for the reorg slot")
		latestShardInfo = s.curReorgStatus.ParentShardInfo
		latestStepId = s.curReorgStatus.ParentStepId
	}

	// first push the header into the cache.
	// it will update the cache if already present or enter a new info
	if err := s.panHeaderCache.Put(slot, &cache.PanCacheInsertParams{
		CurrentVerifiedHeader: headerInfo.Header,
		LastVerifiedShardInfo: latestShardInfo,
	}); err != nil {
		log.WithError(err).WithField("blockNumber", headerInfo.Header.Number).
			WithField("slot", headerInfo.Slot).WithField("headerRoot", headerInfo.Header.Hash()).
			WithField("parentRoot", headerInfo.Header.ParentHash).
			Info("Parent not found in db and cache, discarding the pandora header")

		s.publishBlockConfirmation(headerInfo.Header.Hash(), common.Hash{}, types.Invalid)
		return nil
	}

	// now mark it as we are making a decision on it
	err = s.panHeaderCache.MarkInProgress(slot)
	if err != nil {
		return err
	}
	defer s.panHeaderCache.MarkNotInProgress(slot)

	vanShardInfo := s.vanShardCache.Get(slot)
	if vanShardInfo != nil && vanShardInfo.GetVanShard() != nil {
		return s.insertIntoChain(vanShardInfo.GetVanShard(), headerInfo.Header, latestShardInfo, latestStepId)
	}

	return nil
}

// processVanguardShardInfo
func (s *Service) processVanguardShardInfo(vanShardInfo *types.VanguardShardInfo) error {
	if vanShardInfo == nil {
		return nil
	}

	slot := vanShardInfo.Slot

	// short circuit check, if this header is already in verified sharding info db then send confirmation instantly
	if shardInfo := s.getShardingInfo(slot); shardInfo != nil && shardInfo.NotNil() {
		if shardInfo.GetVanSlotRoot() != common.BytesToHash(vanShardInfo.BlockRoot[:]) {
			log.WithField("shardInfo", shardInfo.FormattedStr()).Debug("Van header is already in verified shard info db")
			return nil
		}
	}

	headStepId := s.db.LatestStepID()
	headShardInfo, err := s.db.VerifiedShardInfo(headStepId)
	if err != nil {
		return errors.Wrap(err, "DB is corrupted! Failed to retrieve latest shard info")
	}

	if headStepId > 0 && (headShardInfo == nil || headShardInfo.IsNil()) {
		return errors.New("nil latest shard info")
	}

	// if reorg triggers here, orc will start processing reorg
	parentShardInfo, parentStepId, err := s.checkReorg(vanShardInfo, headShardInfo, headStepId)
	if err != nil {
		log.WithError(err).Error("Failed to check reorg")
		return nil
	}

	// If any reorg is detected by previous checkReorg method, then update in-memory reorg status and reset the headShardInfo and headStepId
	if parentShardInfo != nil && parentShardInfo.NotNil() {
		if s.curReorgStatus == nil || bytes.Equal(s.curReorgStatus.BlockRoot[:], vanShardInfo.BlockRoot[:]) {
			s.curReorgStatus = &types.ReorgStatus{
				Slot:            slot,
				BlockRoot:       vanShardInfo.BlockRoot,
				ParentStepId:    parentStepId,
				ParentShardInfo: parentShardInfo,
				PandoraHash:     common.BytesToHash(vanShardInfo.ShardInfo.Hash),
				HasResolved:     false,
			}
		}

		headShardInfo = parentShardInfo
		headStepId = parentStepId
	}

	disableDelete := false
	nowTime := uint64(time.Now().Unix())
	currentSlot := (nowTime - s.genesisTime) / s.secondsPerSlot

	// if incoming vanguard block's slot is less than current slot time then we do not delete
	// still orchestrator can resolve reorg if any reorg triggers
	if vanShardInfo.Slot < currentSlot {
		disableDelete = true
	}

	log.WithField("currentSlot", currentSlot).WithField("blockSlot", vanShardInfo.Slot).
		WithField("disableDelete", disableDelete).Debug("Caching incoming slot into vanguard cache")

	// first push the shardInfo into the cache.
	// it will update the cache if already present or enter a new info
	if err := s.vanShardCache.Put(slot, &cache.VanCacheInsertParams{
		DisableDelete:         disableDelete,
		CurrentShardInfo:      vanShardInfo,
		LastVerifiedShardInfo: headShardInfo,
	}); err != nil {
		log.WithError(err).WithField("slot", vanShardInfo.Slot).WithField("blockRoot", common.BytesToHash(vanShardInfo.BlockRoot[:])).
			Info("Unknown parent in db and cache so discarding this vanguard block")

		return nil
	}

	// now mark it as we are making a decision on it
	err = s.vanShardCache.MarkInProgress(slot)
	if err != nil {
		return err
	}
	defer s.vanShardCache.MarkNotInProgress(slot)

	pandoraHeaderInfo := s.panHeaderCache.Get(slot)
	if pandoraHeaderInfo != nil && pandoraHeaderInfo.GetPanHeader() != nil {
		return s.insertIntoChain(vanShardInfo, pandoraHeaderInfo.GetPanHeader(), headShardInfo, headStepId)
	}

	return nil
}

// insertIntoChain method
//	- verifies shard info and pandora header
//  - write into db
//  - send status to pandora chain
func (s *Service) insertIntoChain(
	vanShardInfo *types.VanguardShardInfo,
	header *eth1Types.Header,
	latestShardInfo *types.MultiShardInfo,
	latestStepId uint64,
) error {

	status := types.Invalid
	if compareShardingInfo(header, vanShardInfo.ShardInfo) && s.verifyShardInfo(latestShardInfo, header, vanShardInfo, latestStepId) {

		if s.curReorgStatus != nil && s.curReorgStatus.Slot == vanShardInfo.Slot &&
			bytes.Equal(s.curReorgStatus.BlockRoot[:], vanShardInfo.BlockRoot[:]) && !s.curReorgStatus.HasResolved {

			log.WithField("reorgStatus", s.curReorgStatus.FormattedStr()).Info("Reverting db due reorg!")
			if err := s.processReorg(s.curReorgStatus.ParentStepId, s.curReorgStatus.ParentShardInfo); err != nil {
				log.WithError(err).Error("Failed to process reorg!")
				return nil
			}
			s.curReorgStatus.HasResolved = true
			//TODO(Atif): Need to clear cache of pandora and vanguard after a successful reorg.
			// For clearing pandora and vanguard cache we need to check MarkInProgress logic
		}

		newShardInfo := utils.PrepareMultiShardData(vanShardInfo, header, TotalExecutionShardCount, ShardsPerVanBlock)
		// Write shard info into db
		if err := s.writeShardInfoInDB(newShardInfo); err != nil {
			return errors.Wrap(err, "failed to write shard info in db")
		}
		// write finalize info into db
		s.writeFinalizeInfo(vanShardInfo.FinalizedSlot, vanShardInfo.FinalizedEpoch)
		status = types.Verified

		//removing slot that is already verified
		s.panHeaderCache.ForceDelSlot(vanShardInfo.Slot)
		s.vanShardCache.ForceDelSlot(vanShardInfo.Slot)

	}

	// sending confirmation status to pandora
	s.publishBlockConfirmation(header.Hash(), common.BytesToHash(vanShardInfo.BlockRoot[:]), status)
	return nil
}

func (s *Service) getShardingInfo(slot uint64) *types.MultiShardInfo {
	// Removing slot infos from verified slot info db
	stepId, err := s.db.GetStepIdBySlot(slot)
	if err != nil {
		return nil
	}

	shardInfo, err := s.db.VerifiedShardInfo(stepId)
	if err != nil {
		return nil
	}

	return shardInfo
}

// WriteShardInfoInDB method converts vanShardInfo and panHeader to multiShardingInfo
// Store multiShardingInfo into db
// Update stepId into db
func (s *Service) writeShardInfoInDB(shardInfo *types.MultiShardInfo) error {
	latestStepId := s.db.LatestStepID()
	nextStepId := latestStepId + 1
	if err := s.db.SaveVerifiedShardInfo(nextStepId, shardInfo); err != nil {
		return err
	}

	if err := s.db.SaveLatestStepID(nextStepId); err != nil {
		return err
	}

	if err := s.db.SaveSlotStepIndex(shardInfo.SlotInfo.Slot, nextStepId); err != nil {
		return err
	}

	log.WithField("stepId", nextStepId).WithField("shardInfo", shardInfo.FormattedStr()).
		Info("Inserted sharding info into verified DB")
	return nil
}

// writeFinalizeInfo method store latest finalize slot and epoch if needed
func (s *Service) writeFinalizeInfo(finalizeSlot, finalizeEpoch uint64) {
	curFinalizeSlot := s.db.FinalizedSlot()
	if finalizeSlot > curFinalizeSlot {
		if err := s.db.SaveFinalizedSlot(finalizeSlot); err != nil {
			log.WithError(err).Warn("Failed to store new finalized info")
		}
	}

	curFinalizeEpoch := s.db.FinalizedEpoch()
	if finalizeEpoch > curFinalizeEpoch {
		if err := s.db.SaveFinalizedEpoch(finalizeEpoch); err != nil {
			log.WithError(err).Warn("Failed to store new finalized epoch")
		}
	}
}

// publishBlockConfirmation
func (s *Service) publishBlockConfirmation(blockHash, slotHash common.Hash, status types.Status) {
	s.verifiedSlotInfoFeed.Send(&types.SlotInfoWithStatus{
		PandoraHeaderHash: blockHash,
		VanguardBlockHash: slotHash,
		Status:            status,
	})
}
