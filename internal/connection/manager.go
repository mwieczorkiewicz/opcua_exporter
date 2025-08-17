package connection

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/gopcua/opcua"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/config"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/errors"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/security"
)

// Manager handles OPC UA client connection with automatic retry logic
type Manager struct {
	client         *opcua.Client
	endpoint       string
	securityConfig config.SecurityConfig
	debug          bool
}

// NewManager creates a new connection manager for the given endpoint and configuration
func NewManager(endpoint string, securityConfig config.SecurityConfig, debug bool) *Manager {
	return &Manager{
		endpoint:       endpoint,
		securityConfig: securityConfig,
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
		
		// Create client options based on security configuration
		options, err := m.createClientOptions()
		if err != nil {
			return nil, backoff.Permanent(errors.NewConnectionError(m.endpoint, fmt.Errorf("failed to create client options: %w", err)))
		}
		
		// Create client with security options
		client, err := opcua.NewClient(m.endpoint, options...)
		if err != nil {
			return nil, backoff.Permanent(errors.NewConnectionError(m.endpoint, fmt.Errorf("failed to create OPC UA client: %w", err)))
		}
		
		// Connect with endpoint selection if security is enabled
		if m.securityConfig.SecurityMode != security.SecurityModeNone {
			if err := m.connectWithEndpointSelection(ctx, client); err != nil {
				log.Printf("Secure connection failed: %v", err)
				return nil, errors.NewConnectionError(m.endpoint, err)
			}
		} else {
			if err := client.Connect(ctx); err != nil {
				log.Printf("Connection failed: %v", err)
				return nil, errors.NewConnectionError(m.endpoint, err)
			}
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
	
	// Validate security mode and policy
	if err := security.ValidateSecurityConfig(m.securityConfig.SecurityMode, m.securityConfig.SecurityPolicy); err != nil {
		return fmt.Errorf("security mode/policy configuration invalid: %w", err)
	}
	
	// Log security configuration issues
	m.logSecurityIssues(authConfig)
	
	return nil
}

// createClientOptions creates OPC UA client options based on security configuration
func (m *Manager) createClientOptions() ([]opcua.Option, error) {
	authConfig := security.AuthConfig{
		Mode:            m.securityConfig.AuthMode,
		Username:        m.securityConfig.Username,
		Password:        m.securityConfig.Password,
		CertificateFile: m.securityConfig.CertificateFile,
		PrivateKeyFile:  m.securityConfig.PrivateKeyFile,
		AutoTrust:       m.securityConfig.AutoTrust,
	}
	
	return security.CreateClientOptions(authConfig, m.securityConfig.SecurityMode, m.securityConfig.SecurityPolicy, m.debug)
}

// connectWithEndpointSelection connects using endpoint selection for secure connections
func (m *Manager) connectWithEndpointSelection(ctx context.Context, client *opcua.Client) error {
	// Get available endpoints
	endpointsResp, err := client.GetEndpoints(ctx)
	if err != nil {
		return fmt.Errorf("failed to get endpoints: %w", err)
	}
	
	endpoints := endpointsResp.Endpoints
	if m.debug {
		log.Printf("Server offers %d endpoints", len(endpoints))
		for i, ep := range endpoints {
			log.Printf("Endpoint %d: %s (security: %s, policy: %s)", 
				i, ep.EndpointURL, ep.SecurityMode, ep.SecurityPolicyURI)
		}
	}
	
	// Select appropriate endpoint
	selector := security.NewSecurityEndpointSelector(
		m.securityConfig.SecurityMode,
		m.securityConfig.SecurityPolicy,
		m.securityConfig.AuthMode,
		m.debug,
	)
	
	selectedEndpoint, err := selector.SelectEndpoint(endpoints)
	if err != nil {
		return fmt.Errorf("failed to select compatible endpoint: %w", err)
	}
	
	if m.debug {
		log.Printf("Selected endpoint: %s", selectedEndpoint.EndpointURL)
	}
	
	// Connect using the selected endpoint
	return client.Connect(ctx)
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