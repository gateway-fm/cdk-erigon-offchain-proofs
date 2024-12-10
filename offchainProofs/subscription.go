package offchainProofs

import (
	"context"
	"io"
	"sync/atomic"
	"time"

	"github.com/ledgerwatch/log/v3"

	proofs_pb "github.com/ledgerwatch/erigon/offchainProofs/offchain-proofs"
)

func (s *offchainProofsService) Subscribe() *Subscription {
	stream, err := s.service.StreamNewEvents(context.Background(), &proofs_pb.Blank{})
	if err != nil {
		log.Error("offchainProofsService: err create stream", err, "retry in 500 ms")
		time.Sleep(time.Millisecond * 500)
		return s.Subscribe()
	}

	sub := &Subscription{
		eventsChan: make(chan *proofs_pb.VerifyBatchesTrustedAggregatorEvent),
		stream:     stream,
		isStopped:  atomic.Bool{},
	}

	sub.isStopped.Store(false)

	go sub.listenStream()

	return sub
}

type Subscription struct {
	eventsChan chan *proofs_pb.VerifyBatchesTrustedAggregatorEvent
	stream     proofs_pb.OffchainProofsService_StreamNewEventsClient
	stop       chan struct{}
	isStopped  atomic.Bool
	srv        proofs_pb.OffchainProofsServiceClient
}

func (s *Subscription) Close() {
	close(s.stop)
	close(s.eventsChan)
	s.isStopped.Store(true)
}

func (s *Subscription) Chan() chan *proofs_pb.VerifyBatchesTrustedAggregatorEvent {
	return s.eventsChan
}

func (s *Subscription) listenStream() {
	if s.isStopped.Load() {
		return
	}

	for {
		select {
		case <-s.stop:
			return
		default:
			event, err := s.stream.Recv()
			if err == io.EOF {
				log.Info("offchainProofsService: stream closed by server retry in 500 ms")
				go s.resubscribe()
				return
			}
			if err != nil {
				log.Info("offchainProofsService: error receive event", err, "retry in 500 ms")
				go s.resubscribe()
				return
			}

			s.eventsChan <- event
		}
	}
}

func (s *Subscription) resubscribe() {
	if s.isStopped.Load() {
		return
	}
	time.Sleep(time.Millisecond * 500)

	var err error

	s.stream, err = s.srv.StreamNewEvents(context.Background(), &proofs_pb.Blank{})
	if err != nil {
		log.Error("offchainProofsService: err create stream on resubscribe", err, "retry in 500 ms")
		time.Sleep(time.Millisecond * 500)
		go s.resubscribe()
		return
	}

	go s.listenStream()
}
