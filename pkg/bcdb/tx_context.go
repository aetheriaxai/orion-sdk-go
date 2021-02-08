package bcdb

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/protobuf/proto"
	"github.ibm.com/blockchaindb/server/pkg/logger"
	"github.ibm.com/blockchaindb/server/pkg/types"
)

const (
	contextTimeoutMargin = time.Second
)

type commonTxContext struct {
	userID     string
	signer     Signer
	userCert   []byte
	replicaSet map[string]*url.URL
	nodesCerts map[string]*x509.Certificate
	restClient RestClient
	txEnvelope proto.Message
	timeout    time.Duration
	txSpent    bool
	logger     *logger.SugarLogger
}

type txContext interface {
	composeEnvelope(txID string) (proto.Message, error)
	// structs which embed the commonTXContext must implement this to clean the parts of the state that are not common.
	cleanCtx()
}

func (t *commonTxContext) commit(tx txContext, postEndpoint string, sync bool) (string, *types.TxReceipt, error) {
	if t.txSpent {
		return "", nil, ErrTxSpent
	}

	replica := t.selectReplica()
	postEndpointResolved := replica.ResolveReference(&url.URL{Path: postEndpoint})

	txResponse := &types.TxResponseEnvelope{}

	txID, err := ComputeTxID(t.userCert)
	if err != nil {
		return "", nil, err
	}

	t.logger.Debugf("compose transaction enveloped with txID = %s", txID)
	t.txEnvelope, err = tx.composeEnvelope(txID)
	if err != nil {
		t.logger.Errorf("failed to compose transaction envelope, due to %s", err)
		return txID, nil, err
	}
	ctx := context.Background()
	serverTimeout := time.Duration(0)
	if sync {
		serverTimeout = t.timeout
		contextTimeout := t.timeout + contextTimeoutMargin
		var cancelFnc context.CancelFunc
		ctx, cancelFnc = context.WithTimeout(context.Background(), contextTimeout)
		defer cancelFnc()
	}
	defer tx.cleanCtx()

	response, err := t.restClient.Submit(ctx, postEndpointResolved.String(), t.txEnvelope, serverTimeout)
	if err != nil {
		t.logger.Errorf("failed to submit transaction txID = %s, due to %s", txID, err)
		return txID, nil, err
	}

	if response.StatusCode != http.StatusOK {
		var errMsg string
		if response.StatusCode == http.StatusAccepted {
			return txID, nil, &ServerTimeout{TxID: txID}
		}
		if response.Body != nil {
			errRes := &types.HttpResponseErr{}
			if err := json.NewDecoder(response.Body).Decode(errRes); err != nil {
				t.logger.Errorf("failed to parse the server's error message, due to %s", err)
				errMsg = "(failed to parse the server's error message)"
			} else {
				errMsg = errRes.Error()
			}
		}

		return txID, nil, errors.New(fmt.Sprintf("failed to submit transaction, server returned: status: %s, message: %s", response.Status, errMsg))
	}

	err = json.NewDecoder(response.Body).Decode(txResponse)
	if err != nil {
		t.logger.Errorf("failed to decode json response, due to %s", err)
		return txID, nil, err
	}

	t.txSpent = true
	tx.cleanCtx()
	return txID, txResponse.GetPayload().GetReceipt(), nil
}

func (t *commonTxContext) abort(tx txContext) error {
	if t.txSpent {
		return ErrTxSpent
	}

	t.txSpent = true
	tx.cleanCtx()
	return nil
}

func (t *commonTxContext) selectReplica() *url.URL {
	// Pick first replica to send request to
	for _, replica := range t.replicaSet {
		return replica
	}
	return nil
}

func (t *commonTxContext) handleRequest(rawurl string, query proto.Message, res proto.Message) error {
	parsedURL, err := url.Parse(rawurl)
	if err != nil {
		return err
	}
	restURL := t.selectReplica().ResolveReference(parsedURL).String()
	response, err := t.restClient.Query(context.TODO(), restURL, query)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		var errMsg string
		if response.Body != nil {
			errRes := &types.HttpResponseErr{}
			if err := json.NewDecoder(response.Body).Decode(errRes); err != nil {
				t.logger.Errorf("failed to parse the server's error message, due to %s", err)
				errMsg = "(failed to parse the server's error message)"
			} else {
				errMsg = errRes.Error()
			}
		}
		return errors.New(fmt.Sprintf("error handling request, server returned: status: %s, message: %s", response.Status, errMsg))
	}
	err = json.NewDecoder(response.Body).Decode(res)
	if err != nil {
		t.logger.Errorf("failed to decode json response, due to %s", err)
		return err
	}
	return nil
}

func (t *commonTxContext) TxEnvelope() (proto.Message, error) {
	if t.txEnvelope == nil {
		return nil, ErrTxNotFinalized
	}
	return t.txEnvelope, nil
}

var ErrTxNotFinalized = errors.New("can't access tx envelope, transaction not finalized")
