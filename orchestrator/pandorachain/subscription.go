package pandorachain

import (
	"context"
	eth1Types "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/lukso-network/lukso-orchestrator/shared/types"
)

// subscribePendingHeaders subscribes to pandora client from latest saved slot using given rpc client
func (s *Service) SubscribePendingHeaders(ctx context.Context, crit *types.PandoraPendingHeaderFilter, namespace string,
	client *rpc.Client) (*rpc.ClientSubscription, error) {

	ch := make(chan *eth1Types.Header, 10000000)
	sub, err := client.Subscribe(ctx, namespace, ch, "newPendingBlockHeaders", crit)
	if nil != err {
		return nil, err
	}
	log.WithField("filterCriteria", crit).Debug("subscribed to pandora chain for pending block headers")

	// Start up a dispatcher to feed into the callback
	go func() {
		for {
			select {
			case newPendingHeader := <-ch:
				log.WithField("header", newPendingHeader).WithField(
					"generatedHeaderHash", newPendingHeader.Hash()).WithField(
					"extraData", newPendingHeader.Extra).Debug("Got header info from pandora")
				// dispatch newPendingHeader to handler
				s.OnNewPendingHeader(ctx, newPendingHeader)
			case err := <-sub.Err():
				if err != nil {
					log.WithError(err).Debug("Got subscription error")
					s.conInfoSubErrCh <- err
				}
				return
			}
		}
	}()

	return sub, nil
}
