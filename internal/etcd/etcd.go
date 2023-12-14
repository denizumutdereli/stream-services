package etcd

import (
	"context"
	"time"

	"github.com/denizumutdereli/stream-services/internal/config"
	"github.com/google/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.uber.org/zap"
)

type leaderElectionManagerInterface interface {
	IsConnected() bool
	BecomeLeader(ctx context.Context) error
	ResignLeadership(ctx context.Context) error
	SetNodeReadiness(ctx context.Context) error
	CountReadyNodes(ctx context.Context) (int, error)
	Close() error
}

type LeaderElectionManager struct {
	config           *config.Config
	serviceName      string
	client           *clientv3.Client
	election         *concurrency.Election
	session          *concurrency.Session
	logger           *zap.Logger
	nodeID           string
	connectionStatus bool
}

var _ leaderElectionManagerInterface = (*LeaderElectionManager)(nil)

func NewLeaderElectionManager(ctx context.Context, config *config.Config) (*LeaderElectionManager, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{config.EtcdUrl},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	session, err := concurrency.NewSession(client)
	if err != nil {
		return nil, err
	}

	serviceName := config.ServiceName
	logger := config.Logger
	election := concurrency.NewElection(session, "/"+serviceName+"_service/leader")

	nodeUUID := uuid.New().String()

	return &LeaderElectionManager{
		config:           config,
		client:           client,
		election:         election,
		session:          session,
		serviceName:      serviceName,
		logger:           logger,
		nodeID:           nodeUUID,
		connectionStatus: false,
	}, nil
}

func (s *LeaderElectionManager) IsConnected() bool {
	return s.connectionStatus
}

func (s *LeaderElectionManager) BecomeLeader(ctx context.Context) error {
	s.logger.Info("Attempting to become leader for", zap.String("service", s.serviceName))
	if err := s.election.Campaign(ctx, "I am the leader"); err != nil {
		return err
	}
	s.logger.Info("Successfully became leader for", zap.String("service", s.serviceName))
	return nil
}

func (s *LeaderElectionManager) ResignLeadership(ctx context.Context) error {
	if err := s.election.Resign(ctx); err != nil {
		return err
	}
	s.logger.Info("Relinquished leadership for", zap.String("service", s.serviceName))
	return nil
}

func (s *LeaderElectionManager) SetNodeReadiness(ctx context.Context) error {

	nodeID := s.nodeID

	key := "/readiness/" + s.serviceName + "/" + nodeID

	lease, err := s.client.Grant(ctx, 5) // 10 seconds lease
	if err != nil {
		return err
	}

	_, err = s.client.Put(ctx, key, "ready", clientv3.WithLease(lease.ID))
	if err != nil {
		return err
	}

	keepAlive, err := s.client.KeepAlive(ctx, lease.ID)
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-keepAlive:
				if !ok {
					return
				}
			}
		}
	}()

	return nil
}

func (s *LeaderElectionManager) CountReadyNodes(ctx context.Context) (int, error) {
	prefix := "/readiness/" + s.serviceName + "/"
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return 0, err
	}
	return len(resp.Kvs), nil
}

func (s *LeaderElectionManager) Close() error {
	s.session.Close()
	return s.client.Close()
}
