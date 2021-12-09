package vanguardchain

//func serviceInit(t *testing.T, numberOfElements byte) (*Service, *logTest.Hook) {
//	ctx := context.Background()
//	hook := logTest.NewGlobal()
//	ctrl := gomock.NewController(t)
//	defer ctrl.Finish()
//
//	mockedBeaconClient := mock.NewMockBeaconChainClient(ctrl)
//	mockedNodeClient := mock.NewMockNodeClient(ctrl)
//
//	testDB := dbSetup(ctx, t, numberOfElements)
//	s, err := NewService(ctx, "127.0.0.1:4000", testDB)
//	require.NoError(t, err)
//
//	s.beaconClient = mockedBeaconClient
//	s.nodeClient = mockedNodeClient
//
//	return s, hook
//}

//func dbSetup(ctx context.Context, t *testing.T, numberOfElements byte) db.Database {
//	vanguardDb := testDB.SetupDB(t)
//	var slotInfo *types.SlotInfo
//	for i := byte(0); i < numberOfElements; i++ {
//		slotInfo = &types.SlotInfo{
//			PandoraHeaderHash: common.BytesToHash([]byte{i}),
//			VanguardBlockHash: common.BytesToHash([]byte{i}),
//		}
//
//		err := vanguardDb.SaveVerifiedSlotInfo(uint64(i), slotInfo)
//		assert.NoError(t, err)
//	}
//	err := vanguardDb.SaveLatestVerifiedSlot(ctx, uint64(numberOfElements-1))
//	assert.NoError(t, err)
//	assert.NotNil(t, slotInfo)
//	assert.NotNil(t, slotInfo.PandoraHeaderHash)
//	err = vanguardDb.SaveLatestVerifiedHeaderHash(slotInfo.PandoraHeaderHash)
//	assert.NoError(t, err)
//
//	err = vanguardDb.SaveLatestFinalizedSlot(32)
//	assert.NoError(t, err)
//
//	err = vanguardDb.SaveLatestFinalizedEpoch(1)
//	assert.NoError(t, err)
//
//	// Save Epoch info
//	var totalConsensusInfos []*types.MinimalEpochConsensusInfo
//	for i := byte(0); i < numberOfElements; i++ {
//		consensusInfo := testutil.NewMinimalConsensusInfo(uint64(i))
//		epochInfoV2 := consensusInfo.ConvertToEpochInfo()
//		totalConsensusInfos = append(totalConsensusInfos, epochInfoV2)
//		err = vanguardDb.SaveConsensusInfo(ctx, epochInfoV2)
//		assert.NoError(t, err)
//	}
//	err = vanguardDb.SaveLatestEpoch(ctx, uint64(numberOfElements-1))
//	assert.NoError(t, err)
//
//	return vanguardDb
//}
