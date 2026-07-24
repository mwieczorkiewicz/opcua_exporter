package connection

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/config"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/errors"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/security"
)

// Manager handles OPC UA client connection with automatic retry logic
type Manager struct {
	client         *opcua.Client
	endpoint       string
	securityConfig config.SecurityConfig
	timeouts       config.ConnectionTimeouts
	debug          bool
}

// NewManager creates a new connection manager for the given endpoint and configuration
func NewManager(endpoint string, securityConfig config.SecurityConfig, timeouts config.ConnectionTimeouts, debug bool) *Manager {
	return &Manager{
		endpoint:       endpoint,
		securityConfig: securityConfig,
		timeouts:       timeouts,
		debug:          debug,
	}
}

// Connect establishes a connection to the OPC UA server with retry logic
func (m *Manager) Connect(ctx context.Context) (*opcua.Client, error) {
	connectOperation := func() (*opcua.Client, error) {
		log.Printf("Attempting to connect to OPC UA server at %s", m.endpoint)

		// Validate security configuration
		if err := m.validateSecurityConfig(); err != nil {
			return nil, backoff.Permanent(errors.NewConnectionError(m.endpoint, fmt.Errorf("security configuration validation failed: %w", err)))
		}

		// Discover endpoints when using encryption/signing, or whenever username/certificate
		// auth is configured. gopcua requires the server's UserToken PolicyID (from
		// GetEndpoints); without it some servers return StatusBadIdentityTokenInvalid.
		if m.needsEndpointDiscovery() {
			client, err := m.connectWithEndpointDiscovery(ctx)
			if err != nil {
				log.Printf("Connection with endpoint discovery failed: %v", err)
				return nil, errors.NewConnectionError(m.endpoint, err)
			}
			log.Print("Connected successfully to OPC UA server")
			return client, nil
		}

		// Anonymous + SecurityMode None can connect without GetEndpoints.
		client, err := m.connectInsecure(ctx)
		if err != nil {
			log.Printf("Connection failed: %v", err)
			return nil, errors.NewConnectionError(m.endpoint, err)
		}
		log.Print("Connected successfully to OPC UA server (insecure)")
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
		backoff.WithMaxElapsedTime(m.timeouts.ConnectionRetryTimeout),
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

// validateSecurityConfig validates the security configuration
func (m *Manager) validateSecurityConfig() error {
	// Validate authentication configuration
	authConfig := security.AuthConfig{
		Mode:            m.securityConfig.AuthMode,
		Username:        m.securityConfig.Username,
		Password:        m.securityConfig.Password,
		CertificateFile: m.securityConfig.CertificateFile,
		PrivateKeyFile:  m.securityConfig.PrivateKeyFile,
		AutoTrust:       m.securityConfig.AutoTrust,
	}

	if err := security.ValidateAuthConfig(authConfig); err != nil {
		return fmt.Errorf("authentication configuration invalid: %w", err)
	}

	// Validate security mode, policy, and certificate requirements
	if err := security.ValidateSecurityConfigWithCertificates(
		m.securityConfig.SecurityMode,
		m.securityConfig.SecurityPolicy,
		m.securityConfig.CertificateFile,
		m.securityConfig.PrivateKeyFile,
	); err != nil {
		return fmt.Errorf("security configuration invalid: %w", err)
	}

	// Log security configuration issues
	m.logSecurityIssues(authConfig)

	return nil
}

func (m *Manager) needsEndpointDiscovery() bool {
	if m.securityConfig.AlwaysDiscoverEndpoints {
		return true
	}
	if m.securityConfig.SecurityMode != security.SecurityModeNone {
		return true
	}
	return m.securityConfig.AuthMode != security.AuthModeAnonymous &&
		m.securityConfig.AuthMode != ""
}

// connectInsecure creates and connects an insecure client
func (m *Manager) connectInsecure(ctx context.Context) (*opcua.Client, error) {
	authConfig := security.AuthConfig{
		Mode:            m.securityConfig.AuthMode,
		Username:        m.securityConfig.Username,
		Password:        m.securityConfig.Password,
		CertificateFile: m.securityConfig.CertificateFile,
		PrivateKeyFile:  m.securityConfig.PrivateKeyFile,
		AutoTrust:       m.securityConfig.AutoTrust,
	}

	options, err := security.CreateInsecureClientOptions(authConfig, m.timeouts, m.debug)
	if err != nil {
		return nil, fmt.Errorf("failed to create insecure client options: %w", err)
	}

	client, err := opcua.NewClient(m.endpoint, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OPC UA client: %w", err)
	}

	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return client, nil
}

// connectWithEndpointDiscovery discovers endpoints and creates a secure client
func (m *Manager) connectWithEndpointDiscovery(ctx context.Context) (*opcua.Client, error) {
	// Step 1: Create discovery client (insecure for endpoint discovery)
	discoveryClient, err := opcua.NewClient(m.endpoint, opcua.SecurityMode(ua.MessageSecurityModeNone))
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	// Step 2: Open secure channel only (Dial). GetEndpoints does not require a
	// session; Connect would send CreateSession and some servers
	// that mandate SignAndEncrypt reject that on an insecure channel.
	if err := discoveryClient.Dial(ctx); err != nil {
		return nil, fmt.Errorf("failed to dial discovery client: %w", err)
	}
	defer discoveryClient.Close(ctx) // Clean up discovery client

	// Step 3: Get available endpoints
	endpointsResp, err := discoveryClient.GetEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints: %w", err)
	}

	endpoints := endpointsResp.Endpoints
	if m.debug {
		log.Printf("Server offers %d endpoints", len(endpoints))
		for i, ep := range endpoints {
			log.Printf("Endpoint %d: %s (security: %s, policy: %s)",
				i, ep.EndpointURL, ep.SecurityMode, ep.SecurityPolicyURI)
		}
	}

	// Step 4: Select appropriate endpoint
	selector := security.NewEndpointSelector(
		m.securityConfig.SecurityMode,
		m.securityConfig.SecurityPolicy,
		m.securityConfig.AuthMode,
		m.debug,
	)

	selectedEndpoint, err := selector.SelectEndpoint(endpoints)
	if err != nil {
		return nil, fmt.Errorf("failed to select compatible endpoint: %w", err)
	}

	if m.debug {
		log.Printf("Selected endpoint: %s", selectedEndpoint.EndpointURL)
	}

	// Step 5: Create secure client with selected endpoint
	authConfig := security.AuthConfig{
		Mode:            m.securityConfig.AuthMode,
		Username:        m.securityConfig.Username,
		Password:        m.securityConfig.Password,
		CertificateFile: m.securityConfig.CertificateFile,
		PrivateKeyFile:  m.securityConfig.PrivateKeyFile,
		AutoTrust:       m.securityConfig.AutoTrust,
	}

	options, err := security.CreateSecureClientOptions(authConfig, selectedEndpoint, m.timeouts, m.debug)
	if err != nil {
		return nil, fmt.Errorf("failed to create secure client options: %w", err)
	}

	// Use the configured endpoint URL for the TCP connection. GetEndpoints may
	// return a hostname that cluster DNS cannot resolve;
	// security parameters already come from selectedEndpoint via options above.
	client, err := opcua.NewClient(m.endpoint, options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create secure OPC UA client: %w", err)
	}

	// Step 6: Connect with security
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect securely: %w", err)
	}

	return client, nil
}

// logSecurityIssues logs potential security configuration issues
func (m *Manager) logSecurityIssues(authConfig security.AuthConfig) {
	// Check for insecure configurations
	if authConfig.AutoTrust {
		log.Printf("WARNING: Auto-trust is enabled - this bypasses certificate validation and is insecure for production")
	}

	if authConfig.Mode == security.AuthModeUsername && authConfig.Password == "" {
		log.Printf("WARNING: Username authentication configured but password is empty")
	}

	if m.securityConfig.SecurityMode == security.SecurityModeNone {
		log.Printf("INFO: Security mode is 'None' - connection will not be encrypted")
	}

	// Check for certificate file configuration issues
	if authConfig.CertificateFile != "" && authConfig.PrivateKeyFile == "" {
		log.Printf("WARNING: Certificate file specified but private key file is missing")
	}

	if authConfig.CertificateFile == "" && authConfig.PrivateKeyFile != "" {
		log.Printf("WARNING: Private key file specified but certificate file is missing")
	}

	// Log what security is being used
	if m.debug {
		log.Printf("Security configuration: mode=%s, policy=%s, auth=%s",
			m.securityConfig.SecurityMode, m.securityConfig.SecurityPolicy, m.securityConfig.AuthMode)
	}
}
