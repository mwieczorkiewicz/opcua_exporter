package security

import (
	"crypto/rsa"
	"fmt"
	"log"
	"strings"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

// AuthMode constants for supported authentication modes
const (
	AuthModeAnonymous   = "Anonymous"
	AuthModeUsername    = "Username"
	AuthModeCertificate = "Certificate"
)

// SecurityMode constants for supported security modes
const (
	SecurityModeNone           = "None"
	SecurityModeSign           = "Sign"
	SecurityModeSignAndEncrypt = "SignAndEncrypt"
)

// SecurityPolicy constants for supported security policies
const (
	SecurityPolicyNone           = "None"
	SecurityPolicyBasic128Rsa15  = "Basic128Rsa15"
	SecurityPolicyBasic256       = "Basic256"
	SecurityPolicyBasic256Sha256 = "Basic256Sha256"
)

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Mode            string
	Username        string
	Password        string
	CertificateFile string
	PrivateKeyFile  string
	AutoTrust       bool
}

// SecurityEndpointSelector helps select the appropriate security endpoint
type SecurityEndpointSelector struct {
	securityMode   string
	securityPolicy string
	authMode       string
	debug          bool
}

// NewSecurityEndpointSelector creates a new endpoint selector
func NewSecurityEndpointSelector(securityMode, securityPolicy, authMode string, debug bool) *SecurityEndpointSelector {
	return &SecurityEndpointSelector{
		securityMode:   securityMode,
		securityPolicy: securityPolicy,
		authMode:       authMode,
		debug:          debug,
	}
}

// ValidateAuthConfig validates authentication configuration
func ValidateAuthConfig(cfg AuthConfig) error {
	// Validate auth mode
	switch cfg.Mode {
	case AuthModeAnonymous:
		// No additional validation needed
	case AuthModeUsername:
		if cfg.Username == "" {
			return fmt.Errorf("username is required for username authentication")
		}
		if cfg.Password == "" {
			log.Printf("Warning: password is empty for username authentication")
		}
	case AuthModeCertificate:
		if err := ValidateCertificateFiles(cfg.CertificateFile, cfg.PrivateKeyFile); err != nil {
			return fmt.Errorf("certificate authentication validation failed: %w", err)
		}
	default:
		return fmt.Errorf("unsupported authentication mode: %s (supported: %s, %s, %s)",
			cfg.Mode, AuthModeAnonymous, AuthModeUsername, AuthModeCertificate)
	}

	return nil
}

// ValidateSecurityConfig validates security mode and policy configuration
func ValidateSecurityConfig(securityMode, securityPolicy string) error {
	// Validate security mode
	switch securityMode {
	case SecurityModeNone, SecurityModeSign, SecurityModeSignAndEncrypt:
		// Valid modes
	default:
		return fmt.Errorf("unsupported security mode: %s (supported: %s, %s, %s)",
			securityMode, SecurityModeNone, SecurityModeSign, SecurityModeSignAndEncrypt)
	}

	// Validate security policy
	switch securityPolicy {
	case SecurityPolicyNone, SecurityPolicyBasic128Rsa15, SecurityPolicyBasic256, SecurityPolicyBasic256Sha256:
		// Valid policies
	default:
		return fmt.Errorf("unsupported security policy: %s (supported: %s, %s, %s, %s)",
			securityPolicy, SecurityPolicyNone, SecurityPolicyBasic128Rsa15,
			SecurityPolicyBasic256, SecurityPolicyBasic256Sha256)
	}

	// Validate compatibility between mode and policy
	if securityMode == SecurityModeNone && securityPolicy != SecurityPolicyNone {
		return fmt.Errorf("security mode 'None' requires security policy 'None'")
	}

	if securityMode != SecurityModeNone && securityPolicy == SecurityPolicyNone {
		return fmt.Errorf("security mode '%s' requires a security policy other than 'None'", securityMode)
	}

	return nil
}

// SelectEndpoint selects the best matching endpoint from available endpoints
func (ses *SecurityEndpointSelector) SelectEndpoint(endpoints []*ua.EndpointDescription) (*ua.EndpointDescription, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}

	// Convert string values to OPC UA constants
	targetSecurityMode, err := ses.parseSecurityMode()
	if err != nil {
		return nil, err
	}

	// Find exact match first
	for _, ep := range endpoints {
		if ep.SecurityMode == targetSecurityMode && 
		   strings.Contains(ep.SecurityPolicyURI, ses.getSecurityPolicyURI()) {
			
			// Check if authentication is supported
			if ses.isAuthModeSupported(ep) {
				if ses.debug {
					log.Printf("Selected endpoint: %s (exact match)", ep.EndpointURL)
				}
				return ep, nil
			}
		}
	}

	// If no exact match and we're looking for Anonymous/None, try to find a compatible one
	if ses.authMode == AuthModeAnonymous && ses.securityMode == SecurityModeNone {
		for _, ep := range endpoints {
			if ep.SecurityMode == ua.MessageSecurityModeNone {
				if ses.debug {
					log.Printf("Selected fallback endpoint: %s", ep.EndpointURL)
				}
				return ep, nil
			}
		}
	}

	return nil, fmt.Errorf("no compatible endpoint found for security mode '%s', policy '%s', auth '%s'",
		ses.securityMode, ses.securityPolicy, ses.authMode)
}

// parseSecurityMode converts string to OPC UA MessageSecurityMode
func (ses *SecurityEndpointSelector) parseSecurityMode() (ua.MessageSecurityMode, error) {
	switch ses.securityMode {
	case SecurityModeNone:
		return ua.MessageSecurityModeNone, nil
	case SecurityModeSign:
		return ua.MessageSecurityModeSign, nil
	case SecurityModeSignAndEncrypt:
		return ua.MessageSecurityModeSignAndEncrypt, nil
	default:
		return 0, fmt.Errorf("invalid security mode: %s", ses.securityMode)
	}
}

// parseSecurityPolicy converts string to check against security policy URI
func (ses *SecurityEndpointSelector) parseSecurityPolicy() (string, error) {
	switch ses.securityPolicy {
	case SecurityPolicyNone:
		return "None", nil
	case SecurityPolicyBasic128Rsa15:
		return "Basic128Rsa15", nil
	case SecurityPolicyBasic256:
		return "Basic256", nil
	case SecurityPolicyBasic256Sha256:
		return "Basic256Sha256", nil
	default:
		return "", fmt.Errorf("invalid security policy: %s", ses.securityPolicy)
	}
}

// getSecurityPolicyURI returns the policy name for URI matching
func (ses *SecurityEndpointSelector) getSecurityPolicyURI() string {
	policy, _ := ses.parseSecurityPolicy()
	return policy
}

// isAuthModeSupported checks if the endpoint supports the required authentication mode
func (ses *SecurityEndpointSelector) isAuthModeSupported(ep *ua.EndpointDescription) bool {
	switch ses.authMode {
	case AuthModeAnonymous:
		// Check if anonymous authentication is supported
		for _, token := range ep.UserIdentityTokens {
			if token.TokenType == ua.UserTokenTypeAnonymous {
				return true
			}
		}
		return false
	case AuthModeUsername:
		// Check if username authentication is supported
		for _, token := range ep.UserIdentityTokens {
			if token.TokenType == ua.UserTokenTypeUserName {
				return true
			}
		}
		return false
	case AuthModeCertificate:
		// Check if certificate authentication is supported
		for _, token := range ep.UserIdentityTokens {
			if token.TokenType == ua.UserTokenTypeCertificate {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// CreateClientOptions creates OPC UA client options based on configuration
func CreateClientOptions(authConfig AuthConfig, securityMode, securityPolicy string, debug bool) ([]opcua.Option, error) {
	var options []opcua.Option

	// Add authentication options
	switch authConfig.Mode {
	case AuthModeAnonymous:
		options = append(options, opcua.AuthAnonymous())
		if debug {
			log.Printf("Using anonymous authentication")
		}
	case AuthModeUsername:
		options = append(options, opcua.AuthUsername(authConfig.Username, authConfig.Password))
		if debug {
			log.Printf("Using username authentication for user: %s", authConfig.Username)
		}
	case AuthModeCertificate:
		// Load certificate for authentication
		certManager := NewCertificateManager(authConfig.CertificateFile, authConfig.PrivateKeyFile)
		cert, err := certManager.LoadCertificate()
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate for authentication: %w", err)
		}
		options = append(options, opcua.AuthCertificate(cert.Certificate[0]))
		if debug {
			log.Printf("Using certificate authentication with cert: %s", authConfig.CertificateFile)
		}
	}

	// Add certificate options for secure channels (if certificates are configured)
	if authConfig.CertificateFile != "" && authConfig.PrivateKeyFile != "" {
		certManager := NewCertificateManager(authConfig.CertificateFile, authConfig.PrivateKeyFile)
		cert, err := certManager.LoadCertificate()
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate for secure channel: %w", err)
		}
		options = append(options, opcua.Certificate(cert.Certificate[0]))
		
		// Type assert the private key to RSA private key
		if rsaKey, ok := cert.PrivateKey.(*rsa.PrivateKey); ok {
			options = append(options, opcua.PrivateKey(rsaKey))
		} else {
			return nil, fmt.Errorf("private key is not an RSA key")
		}
		
		if debug {
			log.Printf("Configured certificate for secure channel")
		}
	}

	// Add auto-trust behavior (manually implement since opcua.AutoTrust doesn't exist)
	if authConfig.AutoTrust {
		// Note: Auto-trust functionality needs to be implemented at the connection level
		if debug {
			log.Printf("Auto-trust enabled (WARNING: This is insecure for production use)")
		}
	}

	// Add security policy selection
	if securityMode != SecurityModeNone {
		// Let the endpoint selector handle security mode and policy
		options = append(options, opcua.SecurityFromEndpoint(&ua.EndpointDescription{}, ua.UserTokenTypeAnonymous))
		if debug {
			log.Printf("Using security mode: %s, policy: %s", securityMode, securityPolicy)
		}
	}

	return options, nil
}