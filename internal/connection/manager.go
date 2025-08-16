package connection

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/gopcua/opcua"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/errors"
)

// Manager handles OPC UA client connection with automatic retry logic
type Manager struct {
	client   *opcua.Client
	endpoint string
}

// NewManager creates a new connection manager for the given endpoint
func NewManager(endpoint string) *Manager {
	return &Manager{
		endpoint: endpoint,
	}
}

// Connect establishes a connection to the OPC UA server with retry logic
func (m *Manager) Connect(ctx context.Context) (*opcua.Client, error) {
	connectOperation := func() (*opcua.Client, error) {
		log.Printf("Attempting to connect to OPC UA server at %s", m.endpoint)
		client, err := opcua.NewClient(m.endpoint)
		if err != nil {
			return nil, errors.NewConnectionError(m.endpoint, fmt.Errorf("failed to create OPC UA client: %w", err))
		}
		
		if err := client.Connect(ctx); err != nil {
			log.Printf("Connection failed: %v", err)
			return nil, errors.NewConnectionError(m.endpoint, err)
		}
		
		log.Print("Connected successfully to OPC UA server")
		return client, nil
	}

	notify := func(err error, duration time.Duration) {
		log.Printf("Connection failed, retrying in %v: %v", duration, err)
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 1 * time.Second
	expBackoff.MaxInterval = 30 * time.Second
	expBackoff.Multiplier = 2.0
	expBackoff.RandomizationFactor = 0.1

	client, err := backoff.Retry(ctx, connectOperation,
		backoff.WithBackOff(expBackoff),
		backoff.WithMaxElapsedTime(5*time.Minute),
		backoff.WithNotify(notify),
	)
	if err != nil {
		return nil, errors.NewConnectionError(m.endpoint, fmt.Errorf("failed to connect after retries: %w", err))
	}

	m.client = client
	return client, nil
}

// Close closes the client connection
func (m *Manager) Close(ctx context.Context) error {
	if m.client != nil {
		return m.client.Close(ctx)
	}
	return nil
}

// Client returns the current client instance
func (m *Manager) Client() *opcua.Client {
	return m.client
}