package state

import (
	"errors"

	"github.com/holiman/uint256"
	"github.com/iden3/go-iden3-crypto/keccak256"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/common"
	dstypes "github.com/ledgerwatch/erigon/zk/datastream/types"
)

var (
	LAST_BLOCK_STORAGE_POS      = libcommon.HexToHash("0x0")
	STATE_ROOT_STORAGE_POS      = libcommon.HexToHash("0x1")
	TIMESTAMP_STORAGE_POS       = libcommon.HexToHash("0x2")
	BLOCK_INFO_ROOT_STORAGE_POS = libcommon.HexToHash("0x3")
	ADDRESS_SCALABLE_L2         = libcommon.HexToAddress("0x000000000000000000000000000000005ca1ab1e")
	GER_MANAGER_ADDRESS         = libcommon.HexToAddress("0xa40D5f56745a118D0906a34E69aeC8C0Db1cB8fA")
)

type ReadOnlyHermezDb interface {
	GetEffectiveGasPricePercentage(txHash libcommon.Hash) (uint8, error)
	GetStateRoot(l2BlockNo uint64) (libcommon.Hash, error)
	GetBatchNoByL2Block(l2BlockNo uint64) (uint64, error)
	GetBatchGlobalExitRoots(fromBatchNum, toBatchNum uint64) ([]*dstypes.GerUpdate, error)
	GetBlockGlobalExitRoot(l2BlockNo uint64) (libcommon.Hash, libcommon.Hash, error)
}

func (sdb *IntraBlockState) GetTxCount() (uint64, error) {
	counter, ok := sdb.stateReader.(TxCountReader)
	if !ok {
		return 0, errors.New("state reader does not support GetTxCount")
	}
	return counter.GetTxCount()
}

func (sdb *IntraBlockState) PostExecuteStateSet(chainConfig *chain.Config, blockNum uint64, blockInfoRoot *libcommon.Hash) {
	//ETROG
	if chainConfig.IsForkID7Etrog(blockNum) {
		sdb.scalableSetBlockInfoRoot(blockInfoRoot)
	}
}

func (sdb *IntraBlockState) PreExecuteStateSet(chainConfig *chain.Config, blockNumber uint64, blockTimestamp uint64, stateRoot *libcommon.Hash) {
	if !sdb.Exist(ADDRESS_SCALABLE_L2) {
		// create account if not exists
		sdb.CreateAccount(ADDRESS_SCALABLE_L2, true)
	}

	//save block number
	sdb.scalableSetBlockNum(blockNumber)

	//ETROG
	if chainConfig.IsForkID7Etrog(blockNumber) {
		currentTimestamp := sdb.ScalableGetTimestamp()
		if blockTimestamp > currentTimestamp {
			sdb.ScalableSetTimestamp(blockTimestamp)
		}

		//save prev block hash
		sdb.scalableSetBlockHash(blockNumber-1, stateRoot)
	}
}

func (sdb *IntraBlockState) scalableSetBlockInfoRoot(l1InfoRoot *libcommon.Hash) {
	l1InfoRootBigU := uint256.NewInt(0).SetBytes(l1InfoRoot.Bytes())

	sdb.SetState(ADDRESS_SCALABLE_L2, &BLOCK_INFO_ROOT_STORAGE_POS, *l1InfoRootBigU)

}
func (sdb *IntraBlockState) scalableSetBlockNum(blockNum uint64) {
	sdb.SetState(ADDRESS_SCALABLE_L2, &LAST_BLOCK_STORAGE_POS, *uint256.NewInt(blockNum))
}

func (sdb *IntraBlockState) ScalableSetTimestamp(timestamp uint64) {
	sdb.SetState(ADDRESS_SCALABLE_L2, &TIMESTAMP_STORAGE_POS, *uint256.NewInt(timestamp))
}

func (sdb *IntraBlockState) scalableSetBlockHash(blockNum uint64, blockHash *libcommon.Hash) {
	// create mapping with keccak256(blockNum,position) -> smt root
	d1 := common.LeftPadBytes(uint256.NewInt(blockNum).Bytes(), 32)
	d2 := common.LeftPadBytes(STATE_ROOT_STORAGE_POS.Bytes(), 32)
	mapKey := keccak256.Hash(d1, d2)
	mkh := libcommon.BytesToHash(mapKey)

	hashAsBigU := uint256.NewInt(0).SetBytes(blockHash.Bytes())

	sdb.SetState(ADDRESS_SCALABLE_L2, &mkh, *hashAsBigU)
}

func (sdb *IntraBlockState) ScalableSetSmtRootHash(roHermezDb ReadOnlyHermezDb) error {
	txNum := uint256.NewInt(0)
	slot0 := libcommon.HexToHash("0x0")
	sdb.GetState(ADDRESS_SCALABLE_L2, &slot0, txNum)

	// create mapping with keccak256(txnum,1) -> smt root
	d1 := common.LeftPadBytes(txNum.Bytes(), 32)
	d2 := common.LeftPadBytes(uint256.NewInt(1).Bytes(), 32)
	mapKey := keccak256.Hash(d1, d2)
	mkh := libcommon.BytesToHash(mapKey)

	rpcHash, err := roHermezDb.GetStateRoot(txNum.Uint64())
	if err != nil {
		return err
	}

	if txNum.Uint64() >= 1 {
		// set mapping of keccak256(txnum,1) -> smt root
		rpcHashU256 := uint256.NewInt(0).SetBytes(rpcHash.Bytes())
		sdb.SetState(ADDRESS_SCALABLE_L2, &mkh, *rpcHashU256)
	}

	return nil
}

func (sdb *IntraBlockState) ScalableSetBlockNumberToHash(blockNumber uint64, rodb ReadOnlyHermezDb) error {
	rpcHash, err := rodb.GetStateRoot(blockNumber)
	if err != nil {
		return err
	}

	sdb.scalableSetBlockHash(blockNumber, &rpcHash)

	return nil
}

func (sdb *IntraBlockState) ReadGerManagerL1BlockHash(ger libcommon.Hash) libcommon.Hash {
	d1 := common.LeftPadBytes(ger.Bytes(), 32)
	d2 := common.LeftPadBytes(uint256.NewInt(0).Bytes(), 32)
	mapKey := keccak256.Hash(d1, d2)
	mkh := libcommon.BytesToHash(mapKey)
	key := uint256.NewInt(0)
	sdb.GetState(GER_MANAGER_ADDRESS, &mkh, key)
	if key.Uint64() == 0 {
		return libcommon.Hash{}
	}
	return libcommon.BytesToHash(key.Bytes())
}

func (sdb *IntraBlockState) WriteGerManagerL1BlockHash(ger, l1BlockHash libcommon.Hash) {
	d1 := common.LeftPadBytes(ger.Bytes(), 32)
	d2 := common.LeftPadBytes(uint256.NewInt(0).Bytes(), 32)
	mapKey := keccak256.Hash(d1, d2)
	mkh := libcommon.BytesToHash(mapKey)
	val := uint256.NewInt(0).SetBytes(l1BlockHash.Bytes())
	sdb.SetState(GER_MANAGER_ADDRESS, &mkh, *val)
}

func (sdb *IntraBlockState) ScalableGetTimestamp() uint64 {
	timestamp := uint256.NewInt(0)

	sdb.GetState(ADDRESS_SCALABLE_L2, &TIMESTAMP_STORAGE_POS, timestamp)
	return timestamp.Uint64()
}