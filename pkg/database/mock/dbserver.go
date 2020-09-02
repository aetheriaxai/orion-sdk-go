package server

import (
	"context"
	"encoding/json"

	"github.ibm.com/blockchaindb/protos/types"
)

type mockdbserver struct {
	dbs    map[string]*mockdb
	height *height
}

type mockdb struct {
	values       map[string]*value
	defaultValue []byte
	defaultMeta  *types.Metadata
	server       *mockdbserver
}

func restartMockServer() *mockdbserver {
	mockserver := &mockdbserver{dbs: make(map[string]*mockdb, 0)}
	mockserver.dbs["_dbs"] = &mockdb{
		values: make(map[string]*value),
		server: mockserver,
	}
	mockserver.dbs["_users"] = &mockdb{
		values: make(map[string]*value),
		server: mockserver,
	}
	mockserver.dbs["testDb"] = &mockdb{
		server: mockserver,
	}

	testDbConfig := &types.DatabaseConfig{
		Name: "testDb",
		ReadAccessUsers: []string{
			"any",
		},
		WriteAccessUsers: []string{
			"any",
		},
	}
	testDbConfigBytes, _ := json.Marshal(testDbConfig)
	mockserver.dbs["_dbs"].values = map[string]*value{
		"testDb": {
			values: [][]byte{
				testDbConfigBytes,
			},
			metas: []*types.Metadata{
				{
					Version: nil,
					AccessControl: &types.AccessControl{
						ReadUsers:      map[string]bool{},
						ReadWriteUsers: map[string]bool{},
					},
				},
			},
			index: 0,
		},
	}

	key1result := &value{
		values: make([][]byte, 0),
		metas:  make([]*types.Metadata, 0),
		index:  0,
	}

	key1result.values = append(key1result.values, []byte("Testvalue11"))
	key1result.values = append(key1result.values, []byte("Testvalue12"))
	key1result.metas = append(key1result.metas, &types.Metadata{
		Version: &types.Version{
			BlockNum: 0,
			TxNum:    0,
		},
	})
	key1result.metas = append(key1result.metas, &types.Metadata{
		Version: &types.Version{
			BlockNum: 1,
			TxNum:    0,
		},
	})

	key2result := &value{
		values: make([][]byte, 0),
		metas:  make([]*types.Metadata, 0),
		index:  0,
	}
	key2result.values = append(key2result.values, []byte("Testvalue21"))
	key2result.metas = append(key2result.metas, &types.Metadata{
		Version: &types.Version{
			BlockNum: 0,
			TxNum:    1,
		},
	})

	keyNilResult := &value{
		values: make([][]byte, 0),
		metas:  make([]*types.Metadata, 0),
		index:  0,
	}
	keyNilResult.values = append(keyNilResult.values, nil)
	keyNilResult.metas = append(keyNilResult.metas, &types.Metadata{
		Version: &types.Version{
			BlockNum: 0,
			TxNum:    1,
		},
	})

	defaultResult := []byte("Default1")
	defaultMeta := &types.Metadata{
		Version: &types.Version{
			BlockNum: 1,
			TxNum:    1,
		},
	}

	ledgerHeight := &height{
		results: make([]*types.Digest, 0),
		index:   0,
	}
	ledgerHeight.results = append(ledgerHeight.results, &types.Digest{
		Height: 0,
	})
	ledgerHeight.results = append(ledgerHeight.results, &types.Digest{
		Height: 1,
	})

	results := make(map[string]*value)
	results["key1"] = key1result
	results["key2"] = key2result
	results["keynil"] = keyNilResult

	dbStatusResults := make(map[string]*dbStatus)
	testDBResult := &dbStatus{
		values: make([]*types.GetStatusResponse, 0),
		index:  0,
	}

	testDBResult.values = append(testDBResult.values, &types.GetStatusResponse{
		Header: &types.ResponseHeader{
			NodeID: nodeID,
		},
		Exist: true,
	})
	dbStatusResults["testDb"] = testDBResult

	mockserver.dbs["testDb"].values = results
	mockserver.dbs["testDb"].defaultValue = defaultResult
	mockserver.dbs["testDb"].defaultMeta = defaultMeta
	mockserver.height = ledgerHeight

	return mockserver
}

func (dbs *mockdbserver) GetStatus(ctx context.Context, req *types.GetStatusQueryEnvelope) (*types.GetStatusResponseEnvelope, error) {
	_, ok := dbs.dbs[req.Payload.DBName]
	return dbStatusToEnv(&types.GetStatusResponse{
		Header: &types.ResponseHeader{
			NodeID: nodeID,
		},
		Exist: ok,
	})
}

func (db *mockdb) GetState(req *types.GetStateQueryEnvelope) ([]byte, *types.Metadata) {
	val, ok := db.values[req.Payload.Key]
	if !ok {
		return nil, nil
	}
	if val.index < len(val.values) {
		res := val.values[val.index]
		meta := val.metas[val.index]
		val.index += 1
		return res, meta
	}
	return val.values[len(val.values)-1], val.metas[len(val.metas)-1]
}

func (db *mockdb) PutState(req *types.KVWrite) error {
	_, ok := db.values[req.Key]
	if req.IsDelete {
		if !ok {
			db.values[req.Key] = &value{
				values: make([][]byte, 0),
				metas:  make([]*types.Metadata, 0),
			}
			return nil
		}
		db.values[req.Key].values = append(db.values[req.Key].values, nil)
		db.values[req.Key].metas = append(db.values[req.Key].metas, nil)
		db.values[req.Key].index += 1
	}
	if !ok {
		db.values[req.Key] = &value{
			values: [][]byte{
				req.Value,
			},
			metas: []*types.Metadata{
				{
					Version: &types.Version{
						BlockNum: db.server.height.results[db.server.height.index].Height,
						TxNum:    0,
					},
					AccessControl: req.ACL,
				},
			},
			index: 0,
		}
	} else {
		db.values[req.Key].values = append(db.values[req.Key].values, req.Value)
		db.values[req.Key].metas = append(db.values[req.Key].metas, &types.Metadata{
			Version: &types.Version{
				BlockNum: db.server.height.results[db.server.height.index].Height,
				TxNum:    0,
			},
			AccessControl: req.ACL,
		})
	}
	return nil
}