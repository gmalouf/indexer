package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/algorand/indexer/v3/api/generated/v2"
	"github.com/algorand/indexer/v3/idb"
	"github.com/algorand/indexer/v3/idb/mocks"
	"github.com/algorand/indexer/v3/types"

	sdkcrypto "github.com/algorand/go-algorand-sdk/v2/crypto"
	algorandJson "github.com/algorand/go-algorand-sdk/v2/encoding/json"
	"github.com/algorand/go-algorand-sdk/v2/encoding/msgpack"
	sdk "github.com/algorand/go-algorand-sdk/v2/types"
)

func TestTransactionParamToTransactionFilter(t *testing.T) {
	tests := []struct {
		name          string
		params        generated.SearchForTransactionsParams
		filter        idb.TransactionFilter
		errorContains []string
	}{
		{
			"Default",
			generated.SearchForTransactionsParams{},
			idb.TransactionFilter{Limit: defaultOpts.DefaultTransactionsLimit},
			nil,
		},
		{
			"Limit",
			generated.SearchForTransactionsParams{Limit: uint64Ptr(defaultOpts.DefaultTransactionsLimit + 10)},
			idb.TransactionFilter{Limit: defaultOpts.DefaultTransactionsLimit + 10},
			nil,
		},
		{
			"Limit Max",
			generated.SearchForTransactionsParams{Limit: uint64Ptr(defaultOpts.MaxTransactionsLimit + 10)},
			idb.TransactionFilter{Limit: defaultOpts.MaxTransactionsLimit},
			nil,
		},
		{
			"Int field",
			generated.SearchForTransactionsParams{AssetId: uint64Ptr(1234)},
			idb.TransactionFilter{AssetID: uint64Ptr(1234), Limit: defaultOpts.DefaultTransactionsLimit},
			nil,
		},
		{
			"Pointer field",
			generated.SearchForTransactionsParams{Round: uint64Ptr(1234)},
			idb.TransactionFilter{Round: uint64Ptr(1234), Limit: defaultOpts.DefaultTransactionsLimit},
			nil,
		},
		{
			"Base64 field",
			generated.SearchForTransactionsParams{NotePrefix: strPtr(base64.StdEncoding.EncodeToString([]byte("SomeData")))},
			idb.TransactionFilter{NotePrefix: []byte("SomeData"), Limit: defaultOpts.DefaultTransactionsLimit},
			nil,
		},
		{
			"Enum fields",
			generated.SearchForTransactionsParams{TxType: (*generated.SearchForTransactionsParamsTxType)(strPtr("pay")), SigType: (*generated.SearchForTransactionsParamsSigType)(strPtr("lsig"))},
			idb.TransactionFilter{TypeEnum: 1, SigType: "lsig", Limit: defaultOpts.DefaultTransactionsLimit},
			nil,
		},
		{
			"Date time fields",
			generated.SearchForTransactionsParams{AfterTime: timePtr(time.Date(2020, 3, 4, 12, 0, 0, 0, time.FixedZone("UTC", 0)))},
			idb.TransactionFilter{AfterTime: time.Date(2020, 3, 4, 12, 0, 0, 0, time.FixedZone("UTC", 0)), Limit: defaultOpts.DefaultTransactionsLimit},
			nil,
		},
		{
			"Invalid Enum fields",
			generated.SearchForTransactionsParams{TxType: (*generated.SearchForTransactionsParamsTxType)(strPtr("micro")), SigType: (*generated.SearchForTransactionsParamsSigType)(strPtr("handshake"))},
			idb.TransactionFilter{},
			[]string{errUnknownSigType, errUnknownTxType},
		},
		{
			"As many fields as possible",
			generated.SearchForTransactionsParams{
				Limit:               uint64Ptr(defaultOpts.DefaultTransactionsLimit + 1),
				Next:                strPtr("next-token"),
				NotePrefix:          strPtr(base64.StdEncoding.EncodeToString([]byte("custom-note"))),
				TxType:              (*generated.SearchForTransactionsParamsTxType)(strPtr("pay")),
				SigType:             (*generated.SearchForTransactionsParamsSigType)(strPtr("sig")),
				Txid:                strPtr("YXGBWVBK764KGYPX6ENIADKXPWLBNAZ7MTXDZULZWGOBO2W6IAR622VSLA"),
				Round:               nil,
				MinRound:            uint64Ptr(2),
				MaxRound:            uint64Ptr(3),
				AssetId:             uint64Ptr(4),
				BeforeTime:          timePtr(time.Date(2021, 1, 1, 1, 0, 0, 0, time.FixedZone("UTC", 0))),
				AfterTime:           timePtr(time.Date(2022, 2, 2, 2, 0, 0, 0, time.FixedZone("UTC", 0))),
				CurrencyGreaterThan: uint64Ptr(5),
				CurrencyLessThan:    uint64Ptr(6),
				Address:             strPtr("YXGBWVBK764KGYPX6ENIADKXPWLBNAZ7MTXDZULZWGOBO2W6IAR622VSLA"),
				AddressRole:         (*generated.SearchForTransactionsParamsAddressRole)(strPtr("sender")),
				ExcludeCloseTo:      boolPtr(true),
				ApplicationId:       uint64Ptr(7),
			},
			idb.TransactionFilter{
				Limit:             defaultOpts.DefaultTransactionsLimit + 1,
				NextToken:         "next-token",
				NotePrefix:        []byte("custom-note"),
				TypeEnum:          1,
				SigType:           "sig",
				Txid:              "YXGBWVBK764KGYPX6ENIADKXPWLBNAZ7MTXDZULZWGOBO2W6IAR622VSLA",
				Round:             nil,
				MinRound:          2,
				MaxRound:          3,
				AssetID:           uint64Ptr(4),
				BeforeTime:        time.Date(2021, 1, 1, 1, 0, 0, 0, time.FixedZone("UTC", 0)),
				AfterTime:         time.Date(2022, 2, 2, 2, 0, 0, 0, time.FixedZone("UTC", 0)),
				AlgosGT:           nil,
				AlgosLT:           nil,
				AssetAmountGT:     uint64Ptr(5),
				AssetAmountLT:     uint64Ptr(6),
				EffectiveAmountGT: nil,
				EffectiveAmountLT: nil,
				Address:           []byte{197, 204, 27, 84, 42, 255, 184, 163, 97, 247, 241, 26, 128, 13, 87, 125, 150, 22, 131, 63, 100, 238, 60, 209, 121, 177, 156, 23, 106, 222, 64, 35},
				AddressRole:       9,
				Offset:            nil,
				OffsetLT:          nil,
				OffsetGT:          nil,
				ApplicationID:     uint64Ptr(7),
			},
			nil,
		},
		{
			name:          "Illegal Address",
			params:        generated.SearchForTransactionsParams{Address: strPtr("Not-our-base32-thing")},
			filter:        idb.TransactionFilter{},
			errorContains: []string{errUnableToParseAddress},
		},
		{
			name:          "Unknown address role error",
			params:        generated.SearchForTransactionsParams{AddressRole: (*generated.SearchForTransactionsParamsAddressRole)(strPtr("unknown"))},
			filter:        idb.TransactionFilter{},
			errorContains: []string{errUnknownAddressRole},
		},
		{
			name:          "Bitmask sender + closeTo(true)",
			params:        generated.SearchForTransactionsParams{AddressRole: (*generated.SearchForTransactionsParamsAddressRole)(strPtr("sender")), ExcludeCloseTo: boolPtr(true)},
			filter:        idb.TransactionFilter{AddressRole: 9, Limit: defaultOpts.DefaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask sender + closeTo(false)",
			params:        generated.SearchForTransactionsParams{AddressRole: (*generated.SearchForTransactionsParamsAddressRole)(strPtr("sender")), ExcludeCloseTo: boolPtr(false)},
			filter:        idb.TransactionFilter{AddressRole: 9, Limit: defaultOpts.DefaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask receiver + closeTo(true)",
			params:        generated.SearchForTransactionsParams{AddressRole: (*generated.SearchForTransactionsParamsAddressRole)(strPtr("receiver")), ExcludeCloseTo: boolPtr(true)},
			filter:        idb.TransactionFilter{AddressRole: 18, Limit: defaultOpts.DefaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask receiver + closeTo(false)",
			params:        generated.SearchForTransactionsParams{AddressRole: (*generated.SearchForTransactionsParamsAddressRole)(strPtr("receiver")), ExcludeCloseTo: boolPtr(false)},
			filter:        idb.TransactionFilter{AddressRole: 54, Limit: defaultOpts.DefaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask receiver + implicit closeTo (false)",
			params:        generated.SearchForTransactionsParams{AddressRole: (*generated.SearchForTransactionsParamsAddressRole)(strPtr("receiver"))},
			filter:        idb.TransactionFilter{AddressRole: 54, Limit: defaultOpts.DefaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Bitmask freeze-target",
			params:        generated.SearchForTransactionsParams{AddressRole: (*generated.SearchForTransactionsParamsAddressRole)(strPtr("freeze-target"))},
			filter:        idb.TransactionFilter{AddressRole: 64, Limit: defaultOpts.DefaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Currency to Algos when no asset-id",
			params:        generated.SearchForTransactionsParams{CurrencyGreaterThan: uint64Ptr(10), CurrencyLessThan: uint64Ptr(20)},
			filter:        idb.TransactionFilter{AlgosGT: uint64Ptr(10), AlgosLT: uint64Ptr(20), Limit: defaultOpts.DefaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Searching by application-id",
			params:        generated.SearchForTransactionsParams{ApplicationId: uint64Ptr(1234)},
			filter:        idb.TransactionFilter{ApplicationID: uint64Ptr(1234), Limit: defaultOpts.DefaultTransactionsLimit},
			errorContains: nil,
		},
		{
			name:          "Search all asset transfer by amount",
			params:        generated.SearchForTransactionsParams{TxType: (*generated.SearchForTransactionsParamsTxType)(strPtr("axfer")), CurrencyGreaterThan: uint64Ptr(10)},
			filter:        idb.TransactionFilter{TypeEnum: idb.TypeEnumAssetTransfer, AssetAmountGT: uint64Ptr(10), Limit: defaultOpts.DefaultTransactionsLimit},
			errorContains: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			si := testServerImplementation(nil)
			filter, err := si.transactionParamsToTransactionFilter(test.params)
			if len(test.errorContains) > 0 {
				require.Error(t, err)
				for _, msg := range test.errorContains {
					assert.Contains(t, err.Error(), msg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.filter, filter)
			}
		})
	}
}

func TestValidateTransactionFilter(t *testing.T) {
	tests := []struct {
		name          string
		filter        idb.TransactionFilter
		errorContains []string
	}{
		{
			"Default",
			idb.TransactionFilter{Limit: defaultOpts.DefaultTransactionsLimit},
			nil,
		},
		{
			name: "Round + MinRound Error",
			filter: idb.TransactionFilter{
				Round:    uint64Ptr(10),
				MaxRound: 15,
			},
			errorContains: []string{errInvalidRoundAndMinMax},
		},
		{
			name: "Round + MinRound Error",
			filter: idb.TransactionFilter{
				Round:    uint64Ptr(10),
				MinRound: 5,
			},
			errorContains: []string{errInvalidRoundAndMinMax},
		},
		{
			name: "Swapped Min/Max Round",
			filter: idb.TransactionFilter{
				MinRound: 15,
				MaxRound: 5,
			},
			errorContains: []string{errInvalidRoundMinMax},
		},
		{
			name: "Zero address close address role",
			filter: idb.TransactionFilter{
				Address:     addrSlice(sdk.Address{}),
				AddressRole: idb.AddressRoleSender | idb.AddressRoleCloseRemainderTo,
			},
			errorContains: []string{errZeroAddressCloseRemainderToRole},
		},
		{
			name: "Zero address asset sender and asset close address role",
			filter: idb.TransactionFilter{
				Address:     addrSlice(sdk.Address{}),
				AddressRole: idb.AddressRoleAssetSender | idb.AddressRoleAssetCloseTo,
			},
			errorContains: []string{
				errZeroAddressAssetSenderRole, errZeroAddressAssetCloseToRole},
		},
		{
			name: "Round > math.MaxInt64",
			filter: idb.TransactionFilter{
				Round: uint64Ptr(math.MaxInt64 + 1),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "MinRound > math.MaxInt64",
			filter: idb.TransactionFilter{
				MinRound: uint64(math.MaxInt64 + 1),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "MaxRound > math.MaxInt64",
			filter: idb.TransactionFilter{
				MaxRound: uint64(math.MaxInt64 + 1),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "application-id > math.MaxInt64",
			filter: idb.TransactionFilter{
				ApplicationID: uint64Ptr(math.MaxInt64 + 1),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "asset-id > math.MaxInt64",
			filter: idb.TransactionFilter{
				AssetID: uint64Ptr(math.MaxInt64 + 1),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "offset > math.MaxInt64",
			filter: idb.TransactionFilter{
				Offset: uint64Ptr(uint64(math.MaxInt64 + 1)),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "offsetLT > math.MaxInt64",
			filter: idb.TransactionFilter{
				OffsetLT: uint64Ptr(uint64(math.MaxInt64 + 1)),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "offsetGT > math.MaxInt64",
			filter: idb.TransactionFilter{
				OffsetGT: uint64Ptr(uint64(math.MaxInt64 + 1)),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "algosLT > math.MaxInt64",
			filter: idb.TransactionFilter{
				AlgosLT: uint64Ptr(uint64(math.MaxInt64 + 1)),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "algosGT > math.MaxInt64",
			filter: idb.TransactionFilter{
				AlgosGT: uint64Ptr(uint64(math.MaxInt64 + 1)),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "effectiveAmountLT > math.MaxInt64",
			filter: idb.TransactionFilter{
				EffectiveAmountLT: uint64Ptr(uint64(math.MaxInt64 + 1)),
			},
			errorContains: []string{errValueExceedingInt64},
		},
		{
			name: "effectiveAmountGT > math.MaxInt64",
			filter: idb.TransactionFilter{
				EffectiveAmountGT: uint64Ptr(uint64(math.MaxInt64 + 1)),
			},
			errorContains: []string{errValueExceedingInt64},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTransactionFilter(&test.filter)
			if len(test.errorContains) > 0 {
				require.Error(t, err)
				for _, msg := range test.errorContains {
					assert.Contains(t, err.Error(), msg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func loadResourceFileOrPanic(path string) []byte {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("Failed to load resource file: '%s'", path))
	}
	var ret idb.TxnRow
	_ = msgpack.Decode(data, &ret)
	return data
}

func loadTransactionFromFile(path string) generated.Transaction {
	data := loadResourceFileOrPanic(path)
	var ret generated.Transaction
	if err := json.Unmarshal(data, &ret); err != nil {
		panic(fmt.Sprintf("Failed to build transaction from file: %s", path))
	}
	return ret
}

func loadBlockFromFile(path string) generated.Block {
	data := loadResourceFileOrPanic(path)
	var ret generated.Block
	if err := json.Unmarshal(data, &ret); err != nil {
		panic(fmt.Sprintf("Failed to build block from file: %s", path))
	}
	return ret
}

func TestFetchTransactions(t *testing.T) {
	// Add in txnRows (with TxnBytes to parse), verify that they are properly serialized to generated.TransactionResponse
	tests := []struct {
		name     string
		txnBytes [][]byte
		response []generated.Transaction
		created  uint64
	}{
		{
			name: "Payment",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/payment.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/payment.response"),
			},
		},
		{
			name: "Key Registration",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/keyreg.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/keyreg.response"),
			},
		},
		{
			name: "Key Registration with state proof key",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/keyregwithsprfkey.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/keyregwithsprfkey.response"),
			},
		},
		{
			name: "Asset Configuration",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/asset_config.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/asset_config.response"),
			},
			created: 100,
		},
		{
			name: "Asset Transfer",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/asset_transfer.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/asset_transfer.response"),
			},
		},
		{
			name: "Asset Freeze",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/asset_freeze.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/asset_freeze.response"),
			},
		},
		{
			name: "Multisig Transaction",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/multisig.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/multisig.response"),
			},
		},
		{
			name: "Rekey Transaction",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/rekey.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/rekey.response"),
			},
		},
		{
			name: "Application Call (1)",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_call_1.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_call_1.response"),
			},
			created: 10,
		},
		{
			name: "Application Call (2)",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_call_2.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_call_2.response"),
			},
			created: 10,
		},
		{
			name: "Application Call (3)",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_call_3.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_call_3.response"),
			},
			created: 10,
		},
		{
			name: "App Call with Boxes and Reject Version Txn",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_call_box_create.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_call_box_create.response"),
			},
		},
		{
			name: "Application Clear",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_clear.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_clear.response"),
			},
		},
		{
			name: "Application Close",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_close.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_close.response"),
			},
		},
		{
			name: "Application Update",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_update.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_update.response"),
			},
		},
		{
			name: "Application Delete",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_delete.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_delete.response"),
			},
		},
		{
			name: "Application Non ASCII Key",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_nonascii.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_nonascii.response"),
			},
		},
		{
			name: "Application Optin",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_optin.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_optin.response"),
			},
		},
		{
			name: "Application With Foreign App",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_foreign.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_foreign.response"),
			},
		},
		{
			name: "Application With Foreign Assets",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_foreign_assets.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_foreign_assets.response"),
			},
		},
		{
			name: "Application with logs",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_call_logs.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_call_logs.response"),
			},
		},
		{
			name: "Application with inner txns",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_call_inner.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_call_inner.response"),
			},
		},
		{
			name: "Application inner asset create",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/app_call_inner_acfg.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/app_call_inner_acfg.response"),
			},
		},
		{
			name: "Conditionally index into shared addresses when sharing resources across a group",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/resource_sharing_1.txn"),
				loadResourceFileOrPanic("test_resources/resource_sharing_2.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/resource_sharing_1.response"),
				loadTransactionFromFile("test_resources/resource_sharing_2.response"),
			},
		},
		{
			name: "State Proof Txn",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/state_proof.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/state_proof.response"),
			},
		},
		{
			name: "State Proof Txn - High Reveal Index",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/state_proof_with_index.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/state_proof_with_index.response"),
			},
		},
		{
			name: "Heartbeat Txn",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/heartbeat.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/heartbeat.response"),
			},
		},
	}

	// use for the branch below and createTxn helper func to add a new test case
	var addNewTest = false
	if addNewTest {
		tests = tests[:0]
		tests = append(tests, struct {
			name     string
			txnBytes [][]byte
			response []generated.Transaction
			created  uint64
		}{
			name:     "HeartBeat Txn",
			txnBytes: [][]byte{loadResourceFileOrPanic("test_resources/heartbeat.txn")},
		})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Setup the mocked responses
			mockIndexer := &mocks.IndexerDb{}
			si := testServerImplementation(mockIndexer)
			si.EnableAddressSearchRoundRewind = true
			si.timeout = 1 * time.Second

			roundTime := time.Now()
			roundTime64 := uint64(roundTime.Unix())

			ch := make(chan idb.TxnRow, len(test.txnBytes))
			for _, bytes := range test.txnBytes {
				stxnad := new(sdk.SignedTxnWithAD)
				err := msgpack.Decode(bytes, stxnad)
				require.NoError(t, err)
				txnRow := idb.TxnRow{
					Round:     1,
					Intra:     2,
					RoundTime: roundTime,
					Txn:       stxnad,
					AssetID:   test.created,
					Extra: idb.TxnExtra{
						AssetCloseAmount: 0,
					},
					Error: nil,
				}
				ch <- txnRow
			}

			close(ch)
			var outCh <-chan idb.TxnRow = ch
			var round uint64 = 1
			mockIndexer.On("Transactions", mock.Anything, mock.Anything).Return(outCh, round)

			// Call the function
			results, _, _, err := si.fetchTransactions(context.Background(), idb.TransactionFilter{})
			require.NoError(t, err)

			// Automatically print it out when writing the test.
			printIt := len(test.response) == 0
			if printIt {
				fmt.Printf("Test: %s\n", test.name)
				for _, result := range results {
					fmt.Println("-------------------")
					str, _ := json.Marshal(result)
					fmt.Printf("%s\n", str)
				}
				fmt.Println("-------------------")
				fmt.Printf(`Add the code below as a new entry into 'tests' array and update file names:
		{
			name: "%s",
			txnBytes: [][]byte{
				loadResourceFileOrPanic("test_resources/REPLACEME.txn"),
			},
			response: []generated.Transaction{
				loadTransactionFromFile("test_resources/REPLACEME.response"),
			},
		},
`, test.name)
				fmt.Println("-------------------")
			}

			// Verify the results
			require.Equal(t, len(test.response), len(results))
			for i, expected := range test.response {
				actual := results[i]
				// This is set in the mock above, so override it in the expected value.
				expected.RoundTime = &roundTime64
				fmt.Println(roundTime64)
				if expected.InnerTxns != nil {
					for j := range *expected.InnerTxns {
						(*expected.InnerTxns)[j].RoundTime = &roundTime64
					}
				}
				assert.EqualValues(t, expected, actual)
			}
		})
	}
}

func TestFetchAccountsRewindRoundTooLarge(t *testing.T) {
	ch := make(chan idb.AccountRow)
	close(ch)
	var outCh <-chan idb.AccountRow = ch

	db := &mocks.IndexerDb{}
	db.On("GetAccounts", mock.Anything, mock.Anything).Return(outCh, uint64(7)).Once()

	si := testServerImplementation(db)
	si.EnableAddressSearchRoundRewind = true
	atRound := uint64(8)
	_, _, err := si.fetchAccounts(context.Background(), idb.AccountQueryOptions{}, &atRound)
	assert.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), errRewindingAccount), err.Error())
}

func TestLookupApplicationLogsByID(t *testing.T) {
	mockIndexer := &mocks.IndexerDb{}
	si := testServerImplementation(mockIndexer)
	si.EnableAddressSearchRoundRewind = true

	txnBytes := loadResourceFileOrPanic("test_resources/app_call_logs.txn")
	var stxn sdk.SignedTxnWithAD
	err := msgpack.Decode(txnBytes, &stxn)
	assert.NoError(t, err)

	roundTime := time.Now()
	ch := make(chan idb.TxnRow, 1)
	ch <- idb.TxnRow{
		Round:     1,
		Intra:     2,
		RoundTime: roundTime,
		Txn:       &stxn,
		AssetID:   0,
		Extra: idb.TxnExtra{
			AssetCloseAmount: 0,
		},
		Error: nil,
	}

	close(ch)
	var outCh <-chan idb.TxnRow = ch
	var round uint64 = 1
	mockIndexer.On("Transactions", mock.Anything, mock.Anything).Return(outCh, round)

	appIdx := stxn.Txn.ApplicationID
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v2/applications/:appIdx/logs")
	c.SetParamNames("appIdx")
	c.SetParamValues(fmt.Sprintf("%d", appIdx))

	params := generated.LookupApplicationLogsByIDParams{}
	err = si.LookupApplicationLogsByID(c, 444, params)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response generated.ApplicationLogsResponse
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, uint64(appIdx), response.ApplicationId)
	assert.NotNil(t, response.LogData)
	ld := *response.LogData
	assert.Equal(t, 1, len(ld))
	assert.Equal(t, sdkcrypto.TransactionIDString(stxn.Txn), ld[0].Txid)
	assert.Equal(t, len(stxn.ApplyData.EvalDelta.Logs), len(ld[0].Logs))
	for i, log := range ld[0].Logs {
		assert.Equal(t, []byte(stxn.ApplyData.EvalDelta.Logs[i]), log)
	}
}

func TestLookupApplicationLogsByIDWithLimit(t *testing.T) {
	mockIndexer := &mocks.IndexerDb{}
	si := testServerImplementation(mockIndexer)
	si.EnableAddressSearchRoundRewind = true

	txnBytes := loadResourceFileOrPanic("test_resources/app_call_logs.txn")
	var stxn sdk.SignedTxnWithAD
	err := msgpack.Decode(txnBytes, &stxn)
	assert.NoError(t, err)

	roundTime := time.Now()
	ch := make(chan idb.TxnRow, 1)
	ch <- idb.TxnRow{
		Round:     1,
		Intra:     2,
		RoundTime: roundTime,
		Txn:       &stxn,
		AssetID:   0,
		Extra: idb.TxnExtra{
			AssetCloseAmount: 0,
		},
		Error: nil,
	}

	close(ch)
	var outCh <-chan idb.TxnRow = ch
	var round uint64 = 1

	// Verify that the filter has RequireApplicationLogs set to true
	// The filter.Limit should be requestedLimit + 1 (i.e., 2 when requesting 1)
	mockIndexer.On("Transactions", mock.Anything, mock.MatchedBy(func(filter idb.TransactionFilter) bool {
		return filter.RequireApplicationLogs == true && filter.Limit == 2
	})).Return(outCh, round)

	appIdx := stxn.Txn.ApplicationID
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/v2/applications/:appIdx/logs")
	c.SetParamNames("appIdx")
	c.SetParamValues(fmt.Sprintf("%d", appIdx))

	// Test with limit=1
	params := generated.LookupApplicationLogsByIDParams{Limit: uint64Ptr(1)}
	err = si.LookupApplicationLogsByID(c, uint64(appIdx), params)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var response generated.ApplicationLogsResponse
	err = json.Unmarshal(rec.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, uint64(appIdx), response.ApplicationId)
	assert.NotNil(t, response.LogData)
	ld := *response.LogData
	assert.Equal(t, 1, len(ld))
	assert.Equal(t, sdkcrypto.TransactionIDString(stxn.Txn), ld[0].Txid)
	assert.Equal(t, len(stxn.ApplyData.EvalDelta.Logs), len(ld[0].Logs))
	for i, log := range ld[0].Logs {
		assert.Equal(t, []byte(stxn.ApplyData.EvalDelta.Logs[i]), log)
	}
}

func TestTimeouts(t *testing.T) {
	// function pointers to execute the different DB operations. We really only
	// care that they timeout with WaitUntil, but the return arguments need to
	// be correct to avoid a panic.
	mostMockFunctions := func(method string) func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time) {
		return func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time) {
			mockIndexer.
				On(method, mock.Anything, mock.Anything, mock.Anything).
				WaitUntil(timeout).
				Return(nil, uint64(0))
		}
	}
	transactionFunc := mostMockFunctions("Transactions")
	applicationsFunc := mostMockFunctions("Applications")
	accountsFunc := mostMockFunctions("GetAccounts")
	assetsFunc := mostMockFunctions("Assets")
	balancesFunc := mostMockFunctions("AssetBalances")
	blockFunc := func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time) {
		mockIndexer.
			On("GetBlock", mock.Anything, mock.Anything, mock.Anything).
			WaitUntil(timeout).
			Return(sdk.BlockHeader{}, nil, nil)
	}
	healthFunc := func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time) {
		mockIndexer.
			On("Health", mock.Anything, mock.Anything, mock.Anything).
			WaitUntil(timeout).
			Return(idb.Health{}, nil)
	}

	// Call each of the handlers and let the database timeout.
	testcases := []struct {
		name        string
		errString   string
		mockCall    func(mockIndexer *mocks.IndexerDb, timeout <-chan time.Time)
		callHandler func(ctx echo.Context, si ServerImplementation) error
	}{
		{
			name:      "SearchForTransactions",
			errString: errTransactionSearch,
			mockCall:  transactionFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForTransactions(ctx, generated.SearchForTransactionsParams{})
			},
		},
		{
			name:      "LookupAccountTransactions",
			errString: errTransactionSearch,
			mockCall:  transactionFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountTransactions(ctx, "MONEYMBRSMUAM2NGL6PCEQEDVHFWAQB6DU47NUS6P5DJM4OJFN7E7DSVBA", generated.LookupAccountTransactionsParams{})
			},
		},
		{
			name:      "LookupAssetTransactions",
			errString: errTransactionSearch,
			mockCall:  transactionFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetTransactions(ctx, 1, generated.LookupAssetTransactionsParams{})
			},
		},
		{
			name:      "LookupApplicaitonLogsByID",
			errString: errTransactionSearch,
			mockCall:  transactionFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupApplicationLogsByID(ctx, 1, generated.LookupApplicationLogsByIDParams{})
			},
		},
		{
			name:      "LookupApplicationByID",
			errString: errFailedSearchingApplication,
			mockCall:  applicationsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupApplicationByID(ctx, 0, generated.LookupApplicationByIDParams{})
			},
		},
		{
			name:      "SearchForApplications",
			errString: errFailedSearchingApplication,
			mockCall:  applicationsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForApplications(ctx, generated.SearchForApplicationsParams{})
			},
		},
		{
			name:      "SearchForAccount",
			errString: errFailedSearchingAccount,
			mockCall:  accountsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForAccounts(ctx, generated.SearchForAccountsParams{})
			},
		},
		{
			name:      "LookupAccountByID",
			errString: errFailedSearchingAccount,
			mockCall:  accountsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountByID(ctx,
					"PBH2JQNVP5SBXLTOWNHHPGU6FUMBVS4ZDITPK5RA5FG2YIIFS6UYEMFM2Y",
					generated.LookupAccountByIDParams{})
			},
		},
		{
			name:      "SearchForAssets",
			errString: errFailedSearchingAsset,
			mockCall:  assetsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForAssets(ctx, generated.SearchForAssetsParams{})
			},
		},
		{
			name:      "LookupAssetByID",
			errString: errFailedSearchingAsset,
			mockCall:  assetsFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetByID(ctx, 1, generated.LookupAssetByIDParams{})
			},
		},
		{
			name:      "LookupAssetBalances",
			errString: errFailedSearchingAssetBalances,
			mockCall:  balancesFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetBalances(ctx, 1, generated.LookupAssetBalancesParams{})
			},
		},
		{
			name:      "LookupBlock",
			errString: errLookingUpBlockForRound,
			mockCall:  blockFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupBlock(ctx, 100, generated.LookupBlockParams{})
			},
		},
		{
			name:      "Health",
			errString: errFailedLookingUpHealth,
			mockCall:  healthFunc,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.MakeHealthCheck(ctx)
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			timeout := make(chan time.Time, 1)
			defer func() {
				timeout <- time.Now()
				close(timeout)
			}()

			// Make a mock indexer and tell the mock to timeout.
			mockIndexer := &mocks.IndexerDb{}

			si := testServerImplementation(mockIndexer)
			si.timeout = 5 * time.Millisecond

			// Setup context...
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec1 := httptest.NewRecorder()
			c := e.NewContext(req, rec1)

			// configure the mock to timeout, then call the handler.
			tc.mockCall(mockIndexer, timeout)
			err := tc.callHandler(c, *si)

			require.NoError(t, err)
			bodyStr := rec1.Body.String()
			require.Equal(t, http.StatusServiceUnavailable, rec1.Code)
			require.Contains(t, bodyStr, tc.errString)
			require.Contains(t, bodyStr, "timeout")
		})
	}
}

func TestApplicationLimits(t *testing.T) {
	testcases := []struct {
		name     string
		limit    *uint64
		expected uint64
	}{
		{
			name:     "Default",
			limit:    nil,
			expected: defaultOpts.DefaultApplicationsLimit,
		},
		{
			name:     "Max",
			limit:    uint64Ptr(math.MaxUint64),
			expected: defaultOpts.MaxApplicationsLimit,
		},
	}

	for _, tc := range testcases {

		// Mock backend to capture default limits
		mockIndexer := &mocks.IndexerDb{}
		si := testServerImplementation(mockIndexer)
		si.timeout = 5 * time.Millisecond

		t.Run(tc.name, func(t *testing.T) {
			// Setup context...
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec1 := httptest.NewRecorder()
			c := e.NewContext(req, rec1)

			// check parameters passed to the backend
			mockIndexer.
				On("Applications", mock.Anything, mock.Anything, mock.Anything).
				Return(nil, uint64(0)).
				Run(func(args mock.Arguments) {
					require.Len(t, args, 2)
					require.IsType(t, idb.ApplicationQuery{}, args[1])
					params := args[1].(idb.ApplicationQuery)
					require.Equal(t, params.Limit, tc.expected+1) // +1 for pagination detection
				})

			err := si.SearchForApplications(c, generated.SearchForApplicationsParams{
				Limit: tc.limit,
			})
			require.NoError(t, err)
		})
	}
}

func TestBigNumbers(t *testing.T) {

	testcases := []struct {
		name        string
		errString   string
		callHandler func(ctx echo.Context, si ServerImplementation) error
	}{
		{
			name:      "SearchForTransactionsInvalidRound",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForTransactions(ctx, generated.SearchForTransactionsParams{Round: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "SearchForTransactionsInvalidAppID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForTransactions(ctx, generated.SearchForTransactionsParams{ApplicationId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "SearchForTransactionsInvalidAssetID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForTransactions(ctx, generated.SearchForTransactionsParams{AssetId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "LookupAccountTransactionsInvalidRound",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountTransactions(ctx, "", generated.LookupAccountTransactionsParams{Round: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "LookupAccountTransactionsInvalidAssetID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountTransactions(ctx, "", generated.LookupAccountTransactionsParams{AssetId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "LookupAssetTransactionsInvalidAssetID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetTransactions(ctx, math.MaxInt64+1, generated.LookupAssetTransactionsParams{})
			},
		},
		{
			name:      "LookupAssetTransactionsInvalidRound",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetTransactions(ctx, 12, generated.LookupAssetTransactionsParams{Round: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "LookupApplicaitonLogsByID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupApplicationLogsByID(ctx, math.MaxInt64+1, generated.LookupApplicationLogsByIDParams{})
			},
		},
		{
			name:      "LookupApplicationByID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupApplicationByID(ctx, math.MaxInt64+1, generated.LookupApplicationByIDParams{})
			},
		},
		{
			name:      "SearchForApplications",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForApplications(ctx, generated.SearchForApplicationsParams{ApplicationId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "SearchForAccountInvalidRound",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForAccounts(ctx, generated.SearchForAccountsParams{Round: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "SearchForAccountInvalidAppID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForAccounts(ctx, generated.SearchForAccountsParams{ApplicationId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "SearchForAccountInvalidAssetID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForAccounts(ctx, generated.SearchForAccountsParams{AssetId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "LookupAccountByID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountByID(ctx,
					"PBH2JQNVP5SBXLTOWNHHPGU6FUMBVS4ZDITPK5RA5FG2YIIFS6UYEMFM2Y",
					generated.LookupAccountByIDParams{Round: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "SearchForAssets",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.SearchForAssets(ctx, generated.SearchForAssetsParams{AssetId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "LookupAssetByID",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetByID(ctx, math.MaxInt64+1, generated.LookupAssetByIDParams{})
			},
		},
		{
			name:      "LookupAssetBalances",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAssetBalances(ctx, math.MaxInt64+1, generated.LookupAssetBalancesParams{})
			},
		},
		{
			name:      "LookupBlock",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupBlock(ctx, math.MaxInt64+1, generated.LookupBlockParams{})
			},
		},
		{
			name:      "LookupAccountAppLocalStates",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountAppLocalStates(ctx, "10", generated.LookupAccountAppLocalStatesParams{ApplicationId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "LookupAccountAssets",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountAssets(ctx, "10", generated.LookupAccountAssetsParams{AssetId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "LookupAccountCreatedApplications",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountCreatedApplications(ctx, "10", generated.LookupAccountCreatedApplicationsParams{ApplicationId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
		{
			name:      "LookupAccountCreatedAssets",
			errString: errValueExceedingInt64,
			callHandler: func(ctx echo.Context, si ServerImplementation) error {
				return si.LookupAccountCreatedAssets(ctx, "10", generated.LookupAccountCreatedAssetsParams{AssetId: uint64Ptr(uint64(math.MaxInt64 + 1))})
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {

			// Make a mock indexer.
			mockIndexer := &mocks.IndexerDb{}

			si := testServerImplementation(mockIndexer)

			// Setup context...
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec1 := httptest.NewRecorder()
			c := e.NewContext(req, rec1)

			// call handler
			require.NoError(t, tc.callHandler(c, *si))
			assert.Equal(t, http.StatusNotFound, rec1.Code)
			bodyStr := rec1.Body.String()
			require.Contains(t, bodyStr, tc.errString)
		})
	}
}

func TestFetchBlock(t *testing.T) {
	testcases := []struct {
		name         string
		blockBytes   []byte
		blockOptions idb.GetBlockOptions
		expected     generated.Block
		created      uint64
	}{
		{
			name:         "State Proof Block",
			blockBytes:   loadResourceFileOrPanic("test_resources/stpf_block.block"),
			blockOptions: idb.GetBlockOptions{Transactions: true},
			expected:     loadBlockFromFile("test_resources/stpf_block_response.json"),
		},
		{
			name:         "State Proof Block - High Reveal Index",
			blockBytes:   loadResourceFileOrPanic("test_resources/stpf_block_high_index.block"),
			blockOptions: idb.GetBlockOptions{Transactions: true},
			expected:     loadBlockFromFile("test_resources/stpf_block_high_index_response.json"),
		},
		{
			name:         "Proposer and Incentives Block",
			blockBytes:   loadResourceFileOrPanic("test_resources/proposer_incentives_block.block"),
			blockOptions: idb.GetBlockOptions{Transactions: true},
			expected:     loadBlockFromFile("test_resources/proposer_incentives_block_response.json"),
		},
	}

	for _, tc := range testcases {
		// Mock backend
		mockIndexer := &mocks.IndexerDb{}
		si := testServerImplementation(mockIndexer)
		si.timeout = 1 * time.Second

		roundTime := time.Now()
		roundTime64 := uint64(roundTime.Unix())

		t.Run(tc.name, func(t *testing.T) {
			blk := new(types.EncodedBlockCert)
			err := msgpack.Decode(tc.blockBytes, blk)
			require.NoError(t, err)
			txnRows := make([]idb.TxnRow, len(blk.Block.Payset))
			for idx, stxn := range blk.Block.Payset {
				txnRows[idx] = idb.TxnRow{
					Round:     1,
					Intra:     2,
					RoundTime: roundTime,
					Txn:       &stxn.SignedTxnWithAD,
					AssetID:   tc.created,
					Extra: idb.TxnExtra{
						AssetCloseAmount: 0,
					},
					Error: nil,
				}
			}
			// sdk.BlockHeader, []idb.TxnRow, error
			mockIndexer.
				On("GetBlock", mock.Anything, mock.Anything, mock.Anything).
				Return(blk.Block.BlockHeader, txnRows, nil)

			blkOutput, err := si.fetchBlock(context.Background(), 1, tc.blockOptions)
			require.NoError(t, err)
			actualStr, _ := json.Marshal(blkOutput)
			fmt.Printf("%s\n", actualStr)

			// Set RoundTime which is overridden in the mock above
			if tc.expected.Transactions != nil {
				for i := range *tc.expected.Transactions {
					actual := (*blkOutput.Transactions)[i]
					(*tc.expected.Transactions)[i].RoundTime = &roundTime64
					if (*tc.expected.Transactions)[i].InnerTxns != nil {
						for j := range *(*tc.expected.Transactions)[i].InnerTxns {
							(*(*tc.expected.Transactions)[i].InnerTxns)[j].RoundTime = &roundTime64
						}
					}
					assert.EqualValues(t, (*tc.expected.Transactions)[i], actual)
				}
			}
			assert.EqualValues(t, tc.expected, blkOutput)
		})
	}
}

// TestPaginationBehavior tests all 10 endpoints with comprehensive pagination scenarios
func TestPaginationBehavior(t *testing.T) {
	defaultOpts := ExtraOptions{
		DefaultTransactionsLimit: 1000,
		DefaultAccountsLimit:     100,
		DefaultAssetsLimit:       100,
		DefaultBalancesLimit:     1000,
		DefaultApplicationsLimit: 50,
		DefaultBlocksLimit:       100,
		DefaultBoxesLimit:        20,
		MaxTransactionsLimit:     10000,
		MaxAccountsLimit:         1000,
		MaxAssetsLimit:           1000,
		MaxBalancesLimit:         10000,
		MaxApplicationsLimit:     1000,
		MaxBlocksLimit:           1000,
		MaxBoxesLimit:            100,
	}

	// Helper function to create server implementation
	createServerImpl := func() (*mocks.IndexerDb, *ServerImplementation) {
		mockIndexer := &mocks.IndexerDb{}
		si := &ServerImplementation{
			db:   mockIndexer,
			opts: defaultOpts,
		}
		return mockIndexer, si
	}

	// ========== 1. SearchForAccounts ==========
	t.Run("SearchForAccounts - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		accountsChan := make(chan idb.AccountRow, 3)
		for i := 0; i < 3; i++ {
			accountsChan <- idb.AccountRow{Account: generated.Account{Address: fmt.Sprintf("ACCOUNT%d", i)}}
		}
		close(accountsChan)

		mockIndexer.On("GetAccounts", mock.Anything, mock.MatchedBy(func(opts idb.AccountQueryOptions) bool {
			return opts.Limit == 4
		})).Return((<-chan idb.AccountRow)(accountsChan), uint64(100))

		ctx := createTestContext(t, "/v2/accounts?limit=3", nil)
		err := si.SearchForAccounts(ctx, generated.SearchForAccountsParams{Limit: uint64Ptr(3)})
		require.NoError(t, err)

		var response generated.AccountsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Accounts, 3)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("SearchForAccounts - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		accountsChan := make(chan idb.AccountRow, 4)
		for i := 0; i < 4; i++ {
			accountsChan <- idb.AccountRow{Account: generated.Account{Address: fmt.Sprintf("ACCOUNT%d", i)}}
		}
		close(accountsChan)

		mockIndexer.On("GetAccounts", mock.Anything, mock.MatchedBy(func(opts idb.AccountQueryOptions) bool {
			return opts.Limit == 4
		})).Return((<-chan idb.AccountRow)(accountsChan), uint64(100))

		ctx := createTestContext(t, "/v2/accounts?limit=3", nil)
		err := si.SearchForAccounts(ctx, generated.SearchForAccountsParams{Limit: uint64Ptr(3)})
		require.NoError(t, err)

		var response generated.AccountsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Accounts, 3)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	// ========== 2. SearchForTransactions ==========
	t.Run("SearchForTransactions - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		txnsChan := make(chan idb.TxnRow, 2)
		for i := 0; i < 2; i++ {
			var sender sdk.Address
			sender[31] = byte(i)
			txnsChan <- idb.TxnRow{
				Round: uint64(1000 + i), Intra: i, RoundTime: time.Now(),
				Txn: &sdk.SignedTxnWithAD{
					SignedTxn: sdk.SignedTxn{Txn: sdk.Transaction{Type: sdk.PaymentTx, Header: sdk.Header{Sender: sender}}},
					ApplyData: sdk.ApplyData{ApplicationID: 1},
				},
			}
		}
		close(txnsChan)

		mockIndexer.On("Transactions", mock.Anything, mock.MatchedBy(func(filter idb.TransactionFilter) bool {
			return filter.Limit == 3
		})).Return((<-chan idb.TxnRow)(txnsChan), uint64(100))

		ctx := createTestContext(t, "/v2/transactions?limit=2", nil)
		err := si.SearchForTransactions(ctx, generated.SearchForTransactionsParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.TransactionsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Transactions, 2)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("SearchForTransactions - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		txnsChan := make(chan idb.TxnRow, 3)
		for i := 0; i < 3; i++ {
			var sender sdk.Address
			sender[31] = byte(i)
			txnsChan <- idb.TxnRow{
				Round: uint64(1000 + i), Intra: i, RoundTime: time.Now(),
				Txn: &sdk.SignedTxnWithAD{
					SignedTxn: sdk.SignedTxn{Txn: sdk.Transaction{Type: sdk.PaymentTx, Header: sdk.Header{Sender: sender}}},
					ApplyData: sdk.ApplyData{ApplicationID: 1},
				},
			}
		}
		close(txnsChan)

		mockIndexer.On("Transactions", mock.Anything, mock.MatchedBy(func(filter idb.TransactionFilter) bool {
			return filter.Limit == 3
		})).Return((<-chan idb.TxnRow)(txnsChan), uint64(100))

		ctx := createTestContext(t, "/v2/transactions?limit=2", nil)
		err := si.SearchForTransactions(ctx, generated.SearchForTransactionsParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.TransactionsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Transactions, 2)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	// ========== 3. SearchForAssets ==========
	t.Run("SearchForAssets - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		assetsChan := make(chan idb.AssetRow, 2)
		for i := 0; i < 2; i++ {
			var creator sdk.Address
			creator[31] = byte(i)
			assetsChan <- idb.AssetRow{AssetID: uint64(i + 1), Creator: creator[:], Params: sdk.AssetParams{Total: 1000, Decimals: 6}}
		}
		close(assetsChan)

		mockIndexer.On("Assets", mock.Anything, mock.MatchedBy(func(filter idb.AssetsQuery) bool {
			return filter.Limit == 3
		})).Return((<-chan idb.AssetRow)(assetsChan), uint64(100))

		ctx := createTestContext(t, "/v2/assets?limit=2", nil)
		err := si.SearchForAssets(ctx, generated.SearchForAssetsParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.AssetsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Assets, 2)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("SearchForAssets - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		assetsChan := make(chan idb.AssetRow, 3)
		for i := 0; i < 3; i++ {
			var creator sdk.Address
			creator[31] = byte(i)
			assetsChan <- idb.AssetRow{AssetID: uint64(i + 1), Creator: creator[:], Params: sdk.AssetParams{Total: 1000, Decimals: 6}}
		}
		close(assetsChan)

		mockIndexer.On("Assets", mock.Anything, mock.MatchedBy(func(filter idb.AssetsQuery) bool {
			return filter.Limit == 3
		})).Return((<-chan idb.AssetRow)(assetsChan), uint64(100))

		ctx := createTestContext(t, "/v2/assets?limit=2", nil)
		err := si.SearchForAssets(ctx, generated.SearchForAssetsParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.AssetsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Assets, 2)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	// ========== 4. SearchForApplications ==========
	t.Run("SearchForApplications - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		appsChan := make(chan idb.ApplicationRow, 1)
		appsChan <- idb.ApplicationRow{
			Application: generated.Application{Id: uint64(1), Params: generated.ApplicationParams{ApprovalProgram: []byte("test")}},
		}
		close(appsChan)

		mockIndexer.On("Applications", mock.Anything, mock.MatchedBy(func(filter idb.ApplicationQuery) bool {
			return filter.Limit == 2
		})).Return((<-chan idb.ApplicationRow)(appsChan), uint64(100))

		ctx := createTestContext(t, "/v2/applications?limit=1", nil)
		err := si.SearchForApplications(ctx, generated.SearchForApplicationsParams{Limit: uint64Ptr(1)})
		require.NoError(t, err)

		var response generated.ApplicationsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Applications, 1)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("SearchForApplications - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		appsChan := make(chan idb.ApplicationRow, 2)
		for i := 1; i <= 2; i++ {
			appsChan <- idb.ApplicationRow{
				Application: generated.Application{Id: uint64(i), Params: generated.ApplicationParams{ApprovalProgram: []byte("test")}},
			}
		}
		close(appsChan)

		mockIndexer.On("Applications", mock.Anything, mock.MatchedBy(func(filter idb.ApplicationQuery) bool {
			return filter.Limit == 2
		})).Return((<-chan idb.ApplicationRow)(appsChan), uint64(100))

		ctx := createTestContext(t, "/v2/applications?limit=1", nil)
		err := si.SearchForApplications(ctx, generated.SearchForApplicationsParams{Limit: uint64Ptr(1)})
		require.NoError(t, err)

		var response generated.ApplicationsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Applications, 1)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	// ========== 5. LookupAccountAssets ==========
	t.Run("LookupAccountAssets - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		assetsChan := make(chan idb.AssetBalanceRow, 2)
		for i := 1; i <= 2; i++ {
			var addr sdk.Address
			addr[31] = byte(i)
			assetsChan <- idb.AssetBalanceRow{Address: addr[:], AssetID: uint64(i * 10), Amount: uint64(i * 100)}
		}
		close(assetsChan)

		mockIndexer.On("AssetBalances", mock.Anything, mock.MatchedBy(func(query idb.AssetBalanceQuery) bool {
			return query.Limit == 3
		})).Return((<-chan idb.AssetBalanceRow)(assetsChan), uint64(100))

		ctx := createTestContext(t, "/v2/accounts/test/assets?limit=2", nil)
		err := si.LookupAccountAssets(ctx, "GJR76Q6OXNZ2CYIVCFCDTJRBAAR6TYEJJENEII3G2U3JH546SPBQA62IFY",
			generated.LookupAccountAssetsParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.AssetHoldingsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Assets, 2)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("LookupAccountAssets - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		assetsChan := make(chan idb.AssetBalanceRow, 3)
		for i := 1; i <= 3; i++ {
			var addr sdk.Address
			addr[31] = byte(i)
			assetsChan <- idb.AssetBalanceRow{Address: addr[:], AssetID: uint64(i * 10), Amount: uint64(i * 100)}
		}
		close(assetsChan)

		mockIndexer.On("AssetBalances", mock.Anything, mock.MatchedBy(func(query idb.AssetBalanceQuery) bool {
			return query.Limit == 3
		})).Return((<-chan idb.AssetBalanceRow)(assetsChan), uint64(100))

		ctx := createTestContext(t, "/v2/accounts/test/assets?limit=2", nil)
		err := si.LookupAccountAssets(ctx, "GJR76Q6OXNZ2CYIVCFCDTJRBAAR6TYEJJENEII3G2U3JH546SPBQA62IFY",
			generated.LookupAccountAssetsParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.AssetHoldingsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Assets, 2)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	// ========== 6. LookupAssetBalances ==========
	t.Run("LookupAssetBalances - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		balancesChan := make(chan idb.AssetBalanceRow, 2)
		for i := 1; i <= 2; i++ {
			var addr sdk.Address
			addr[31] = byte(i)
			balancesChan <- idb.AssetBalanceRow{Address: addr[:], AssetID: 1, Amount: uint64(i * 100)}
		}
		close(balancesChan)

		mockIndexer.On("AssetBalances", mock.Anything, mock.MatchedBy(func(query idb.AssetBalanceQuery) bool {
			return query.Limit == 3 && *query.AssetID == 1
		})).Return((<-chan idb.AssetBalanceRow)(balancesChan), uint64(100))

		ctx := createTestContext(t, "/v2/assets/1/balances?limit=2", nil)
		err := si.LookupAssetBalances(ctx, 1, generated.LookupAssetBalancesParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.AssetBalancesResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Balances, 2)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("LookupAssetBalances - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		balancesChan := make(chan idb.AssetBalanceRow, 3)
		for i := 1; i <= 3; i++ {
			var addr sdk.Address
			addr[31] = byte(i)
			balancesChan <- idb.AssetBalanceRow{Address: addr[:], AssetID: 1, Amount: uint64(i * 100)}
		}
		close(balancesChan)

		mockIndexer.On("AssetBalances", mock.Anything, mock.MatchedBy(func(query idb.AssetBalanceQuery) bool {
			return query.Limit == 3 && *query.AssetID == 1
		})).Return((<-chan idb.AssetBalanceRow)(balancesChan), uint64(100))

		ctx := createTestContext(t, "/v2/assets/1/balances?limit=2", nil)
		err := si.LookupAssetBalances(ctx, 1, generated.LookupAssetBalancesParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.AssetBalancesResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Balances, 2)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	// ========== 7. LookupAccountAppLocalStates ==========
	t.Run("LookupAccountAppLocalStates - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		appsChan := make(chan idb.AppLocalStateRow, 2)
		for i := 1; i <= 2; i++ {
			appsChan <- idb.AppLocalStateRow{AppLocalState: generated.ApplicationLocalState{Id: uint64(i)}}
		}
		close(appsChan)

		mockIndexer.On("AppLocalState", mock.Anything, mock.MatchedBy(func(query idb.ApplicationQuery) bool {
			return query.Limit == 3
		})).Return((<-chan idb.AppLocalStateRow)(appsChan), uint64(100))

		ctx := createTestContext(t, "/v2/accounts/test/apps-local-state?limit=2", nil)
		err := si.LookupAccountAppLocalStates(ctx, "GJR76Q6OXNZ2CYIVCFCDTJRBAAR6TYEJJENEII3G2U3JH546SPBQA62IFY",
			generated.LookupAccountAppLocalStatesParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.ApplicationLocalStatesResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.AppsLocalStates, 2)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("LookupAccountAppLocalStates - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		appsChan := make(chan idb.AppLocalStateRow, 3)
		for i := 1; i <= 3; i++ {
			appsChan <- idb.AppLocalStateRow{AppLocalState: generated.ApplicationLocalState{Id: uint64(i)}}
		}
		close(appsChan)

		mockIndexer.On("AppLocalState", mock.Anything, mock.MatchedBy(func(query idb.ApplicationQuery) bool {
			return query.Limit == 3
		})).Return((<-chan idb.AppLocalStateRow)(appsChan), uint64(100))

		ctx := createTestContext(t, "/v2/accounts/test/apps-local-state?limit=2", nil)
		err := si.LookupAccountAppLocalStates(ctx, "GJR76Q6OXNZ2CYIVCFCDTJRBAAR6TYEJJENEII3G2U3JH546SPBQA62IFY",
			generated.LookupAccountAppLocalStatesParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.ApplicationLocalStatesResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.AppsLocalStates, 2)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	// ========== 8. SearchForApplicationBoxes ==========
	t.Run("SearchForApplicationBoxes - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		boxesChan := make(chan idb.ApplicationBoxRow, 2)
		for i := 1; i <= 2; i++ {
			boxName := []byte(fmt.Sprintf("box%d", i))
			boxesChan <- idb.ApplicationBoxRow{App: 1, Box: generated.Box{Name: boxName, Value: []byte("data")}}
		}
		close(boxesChan)

		mockIndexer.On("ApplicationBoxes", mock.Anything, mock.MatchedBy(func(query idb.ApplicationBoxQuery) bool {
			return query.ApplicationID == 1 && query.Limit == 3
		})).Return((<-chan idb.ApplicationBoxRow)(boxesChan), uint64(100))

		ctx := createTestContext(t, "/v2/applications/1/boxes?limit=2", nil)
		err := si.SearchForApplicationBoxes(ctx, 1, generated.SearchForApplicationBoxesParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.BoxesResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Boxes, 2)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("SearchForApplicationBoxes - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		boxesChan := make(chan idb.ApplicationBoxRow, 3)
		for i := 1; i <= 3; i++ {
			boxName := []byte(fmt.Sprintf("box%d", i))
			boxesChan <- idb.ApplicationBoxRow{App: 1, Box: generated.Box{Name: boxName, Value: []byte("data")}}
		}
		close(boxesChan)

		mockIndexer.On("ApplicationBoxes", mock.Anything, mock.MatchedBy(func(query idb.ApplicationBoxQuery) bool {
			return query.ApplicationID == 1 && query.Limit == 3
		})).Return((<-chan idb.ApplicationBoxRow)(boxesChan), uint64(100))

		ctx := createTestContext(t, "/v2/applications/1/boxes?limit=2", nil)
		err := si.SearchForApplicationBoxes(ctx, 1, generated.SearchForApplicationBoxesParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.BoxesResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Boxes, 2)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	// ========== 9. LookupApplicationLogsByID ==========
	t.Run("LookupApplicationLogsByID - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		txnsChan := make(chan idb.TxnRow, 2)
		for i := 1; i <= 2; i++ {
			var sender sdk.Address
			sender[31] = byte(i)
			txnsChan <- idb.TxnRow{
				Round: uint64(1000 + i), Intra: i, RoundTime: time.Now(),
				Txn: &sdk.SignedTxnWithAD{
					SignedTxn: sdk.SignedTxn{Txn: sdk.Transaction{Type: sdk.ApplicationCallTx, Header: sdk.Header{Sender: sender}}},
					ApplyData: sdk.ApplyData{ApplicationID: 1},
				},
			}
		}
		close(txnsChan)

		mockIndexer.On("Transactions", mock.Anything, mock.MatchedBy(func(filter idb.TransactionFilter) bool {
			return filter.Limit == 3 && filter.RequireApplicationLogs
		})).Return((<-chan idb.TxnRow)(txnsChan), uint64(100))

		ctx := createTestContext(t, "/v2/applications/1/logs?limit=2", nil)
		err := si.LookupApplicationLogsByID(ctx, 1, generated.LookupApplicationLogsByIDParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.ApplicationLogsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("LookupApplicationLogsByID - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		txnsChan := make(chan idb.TxnRow, 3)
		for i := 1; i <= 3; i++ {
			var sender sdk.Address
			sender[31] = byte(i)
			txnsChan <- idb.TxnRow{
				Round: uint64(1000 + i), Intra: i, RoundTime: time.Now(),
				Txn: &sdk.SignedTxnWithAD{
					SignedTxn: sdk.SignedTxn{Txn: sdk.Transaction{Type: sdk.ApplicationCallTx, Header: sdk.Header{Sender: sender}}},
					ApplyData: sdk.ApplyData{ApplicationID: 1},
				},
			}
		}
		close(txnsChan)

		mockIndexer.On("Transactions", mock.Anything, mock.MatchedBy(func(filter idb.TransactionFilter) bool {
			return filter.Limit == 3 && filter.RequireApplicationLogs
		})).Return((<-chan idb.TxnRow)(txnsChan), uint64(100))

		ctx := createTestContext(t, "/v2/applications/1/logs?limit=2", nil)
		err := si.LookupApplicationLogsByID(ctx, 1, generated.LookupApplicationLogsByIDParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.ApplicationLogsResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	// ========== 10. SearchForBlockHeaders ==========
	t.Run("SearchForBlockHeaders - no next token when results < limit", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		blocksChan := make(chan idb.BlockRow, 2)
		for i := 1; i <= 2; i++ {
			blocksChan <- idb.BlockRow{
				BlockHeader: sdk.BlockHeader{Round: sdk.Round(i), TimeStamp: int64(1000000 + i)},
			}
		}
		close(blocksChan)

		mockIndexer.On("BlockHeaders", mock.Anything, mock.MatchedBy(func(filter idb.BlockHeaderFilter) bool {
			return filter.Limit == 3
		})).Return((<-chan idb.BlockRow)(blocksChan), uint64(100))

		ctx := createTestContext(t, "/v2/blocks?limit=2", nil)
		err := si.SearchForBlockHeaders(ctx, generated.SearchForBlockHeadersParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.BlockHeadersResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Blocks, 2)
		require.Nil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})

	t.Run("SearchForBlockHeaders - has next token when results = limit+1", func(t *testing.T) {
		mockIndexer, si := createServerImpl()

		blocksChan := make(chan idb.BlockRow, 3)
		for i := 1; i <= 3; i++ {
			blocksChan <- idb.BlockRow{
				BlockHeader: sdk.BlockHeader{Round: sdk.Round(i), TimeStamp: int64(1000000 + i)},
			}
		}
		close(blocksChan)

		mockIndexer.On("BlockHeaders", mock.Anything, mock.MatchedBy(func(filter idb.BlockHeaderFilter) bool {
			return filter.Limit == 3
		})).Return((<-chan idb.BlockRow)(blocksChan), uint64(100))

		ctx := createTestContext(t, "/v2/blocks?limit=2", nil)
		err := si.SearchForBlockHeaders(ctx, generated.SearchForBlockHeadersParams{Limit: uint64Ptr(2)})
		require.NoError(t, err)

		var response generated.BlockHeadersResponse
		responseBytes := ctx.Response().Writer.(*httptest.ResponseRecorder).Body.Bytes()
		err = algorandJson.Decode(responseBytes, &response)
		require.NoError(t, err)
		require.Len(t, response.Blocks, 2)
		require.NotNil(t, response.NextToken)
		mockIndexer.AssertExpectations(t)
	})
}

func createTestContext(t *testing.T, url string, body []byte) echo.Context {
	e := echo.New()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(http.MethodGet, url, nil)
	}
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	return ctx
}
