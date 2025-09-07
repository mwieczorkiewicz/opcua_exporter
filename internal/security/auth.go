package security

import (
	"crypto/rsa"
	"fmt"
	"log"
	"strings"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/mwieczorkiewicz/opcua_exporter/internal/config"
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

// EndpointSelector helps select the appropriate security endpoint
type EndpointSelector struct {
	securityMode   string
	securityPolicy string
	authMode       string
	debug          bool
}

// NewEndpointSelector creates a new endpoint selector
func NewEndpointSelector(securityMode, securityPolicy, authMode string, debug bool) *EndpointSelector {
	return &EndpointSelector{
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

// ValidateSecurityConfigWithCertificates validates security configuration including certificate requirements
func ValidateSecurityConfigWithCertificates(securityMode, securityPolicy, certificateFile, privateKeyFile string) error {
	// First validate basic security config
	if err := ValidateSecurityConfig(securityMode, securityPolicy); err != nil {
		return err
	}

	// Check if certificates are required for the security configuration
	requiresCertificates := requiresCertificatesForSecurity(securityMode, securityPolicy)

	if requiresCertificates {
		if certificateFile == "" || privateKeyFile == "" {
			return fmt.Errorf("security policy '%s' with mode '%s' requires both certificate and private key files to be specified",
				securityPolicy, securityMode)
		}

		// Validate that the certificate files exist and are readable
		if err := ValidateCertificateFiles(certificateFile, privateKeyFile); err != nil {
			return fmt.Errorf("certificate validation failed for security policy '%s': %w", securityPolicy, err)
		}
	}

	return nil
}

// requiresCertificatesForSecurity determines if the security configuration requires certificates
func requiresCertificatesForSecurity(securityMode, securityPolicy string) bool {
	// None security mode never requires certificates
	if securityMode == SecurityModeNone {
		return false
	}

	// Sign and SignAndEncrypt modes with crypto policies require certificates
	switch securityPolicy {
	case SecurityPolicyBasic128Rsa15, SecurityPolicyBasic256, SecurityPolicyBasic256Sha256:
		return true
	case SecurityPolicyNone:
		return false
	default:
		// For unknown policies, assume certificates are required if not "None"
		return true
	}
}

// SelectEndpoint selects the best matching endpoint from available endpoints
func (ses *EndpointSelector) SelectEndpoint(endpoints []*ua.EndpointDescription) (*ua.EndpointDescription, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}

	// Convert string values to OPC UA constants
	targetSecurityMode, err := ses.parseSecurityMode()
	if err != nil {
		return nil, err
	}

	targetPolicyURI := ses.getSecurityPolicyURI()

	// Find exact match first
	exactMatches := []*ua.EndpointDescription{}
	compatibleMatches := []*ua.EndpointDescription{}

	for _, ep := range endpoints {
		// Check security mode match
		securityModeMatch := ep.SecurityMode == targetSecurityMode

		// Check security policy match
		var policyMatch bool
		if targetPolicyURI == "None" {
			policyMatch = strings.HasSuffix(ep.SecurityPolicyURI, "#None") || ep.SecurityPolicyURI == ""
		} else {
			policyMatch = strings.Contains(ep.SecurityPolicyURI, targetPolicyURI)
		}

		// Check authentication support
		authSupported := ses.isAuthModeSupported(ep)

		if ses.debug {
			log.Printf("Evaluating endpoint %s: security=%s (want %s), policy=%s (want %s), auth=%t",
				ep.EndpointURL, ep.SecurityMode, targetSecurityMode, ep.SecurityPolicyURI, targetPolicyURI, authSupported)
		}

		if securityModeMatch && policyMatch && authSupported {
			exactMatches = append(exactMatches, ep)
		} else if authSupported {
			// Keep as fallback if auth is supported but security doesn't match exactly
			compatibleMatches = append(compatibleMatches, ep)
		}
	}

	// Prefer exact matches
	if len(exactMatches) > 0 {
		selected := exactMatches[0] // Take first exact match
		if ses.debug {
			log.Printf("Selected endpoint: %s (exact match)", selected.EndpointURL)
		}
		return selected, nil
	}

	// If no exact match and we're looking for Anonymous/None, try compatible ones
	if ses.authMode == AuthModeAnonymous && ses.securityMode == SecurityModeNone {
		for _, ep := range compatibleMatches {
			if ep.SecurityMode == ua.MessageSecurityModeNone {
				if ses.debug {
					log.Printf("Selected fallback endpoint: %s", ep.EndpointURL)
				}
				return ep, nil
			}
		}
	}

	// If still no match, provide detailed error with available options
	availableInfo := []string{}
	for _, ep := range endpoints {
		info := fmt.Sprintf("URL=%s, Security=%s, Policy=%s, AuthTokens=%d",
			ep.EndpointURL, ep.SecurityMode, ep.SecurityPolicyURI, len(ep.UserIdentityTokens))
		availableInfo = append(availableInfo, info)
	}

	return nil, fmt.Errorf("no compatible endpoint found for security mode '%s', policy '%s', auth '%s'. Available endpoints: %s",
		ses.securityMode, ses.securityPolicy, ses.authMode, strings.Join(availableInfo, "; "))
}

// parseSecurityMode converts string to OPC UA MessageSecurityMode
func (ses *EndpointSelector) parseSecurityMode() (ua.MessageSecurityMode, error) {
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
func (ses *EndpointSelector) parseSecurityPolicy() (string, error) {
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
func (ses *EndpointSelector) getSecurityPolicyURI() string {
	policy, _ := ses.parseSecurityPolicy()
	return policy
}

// isAuthModeSupported checks if the endpoint supports the required authentication mode
func (ses *EndpointSelector) isAuthModeSupported(ep *ua.EndpointDescription) bool {
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

// CreateInsecureClientOptions creates OPC UA client options for insecure connections
func CreateInsecureClientOptions(authConfig AuthConfig, timeouts config.ConnectionTimeouts, debug bool) ([]opcua.Option, error) {
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

	// For insecure connections, use no security
	options = append(options, opcua.SecurityMode(ua.MessageSecurityModeNone))

	// Add timeout options
	options = append(options, opcua.DialTimeout(timeouts.DialTimeout))
	options = append(options, opcua.RequestTimeout(timeouts.RequestTimeout))
	options = append(options, opcua.SessionTimeout(timeouts.SessionTimeout))

	if debug {
		log.Printf("Using insecure connection (no encryption)")
		log.Printf("Timeout configuration: dial=%v, request=%v, session=%v",
			timeouts.DialTimeout, timeouts.RequestTimeout, timeouts.SessionTimeout)
	}

	return options, nil
}

// CreateSecureClientOptions creates OPC UA client options for secure connections with selected endpoint
func CreateSecureClientOptions(authConfig AuthConfig, selectedEndpoint *ua.EndpointDescription, timeouts config.ConnectionTimeouts, debug bool) ([]opcua.Option, error) {
	var options []opcua.Option

	// Get the user token type for this authentication mode
	userTokenType, err := getUserTokenTypeForAuth(authConfig.Mode, selectedEndpoint)
	if err != nil {
		return nil, fmt.Errorf("authentication mode not supported by endpoint: %w", err)
	}

	// Use security settings from selected endpoint
	options = append(options, opcua.SecurityFromEndpoint(selectedEndpoint, userTokenType))

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

	// Auto-trust functionality is not directly supported in current gopcua API
	// The connection will follow standard certificate validation rules
	if authConfig.AutoTrust {
		if debug {
			log.Printf("Auto-trust enabled (WARNING: This is insecure for production use)")
			log.Printf("Note: Auto-trust requires manual certificate acceptance in current gopcua version")
		}
	}

	// Add timeout options
	options = append(options, opcua.DialTimeout(timeouts.DialTimeout))
	options = append(options, opcua.RequestTimeout(timeouts.RequestTimeout))
	options = append(options, opcua.SessionTimeout(timeouts.SessionTimeout))

	if debug {
		log.Printf("Using secure connection with endpoint: %s", selectedEndpoint.EndpointURL)
		log.Printf("Timeout configuration: dial=%v, request=%v, session=%v",
			timeouts.DialTimeout, timeouts.RequestTimeout, timeouts.SessionTimeout)
	}

	return options, nil
}

// getUserTokenTypeForAuth determines the appropriate user token type for the authentication mode
func getUserTokenTypeForAuth(authMode string, endpoint *ua.EndpointDescription) (ua.UserTokenType, error) {
	switch authMode {
	case AuthModeAnonymous:
		// Check if anonymous authentication is supported
		for _, token := range endpoint.UserIdentityTokens {
			if token.TokenType == ua.UserTokenTypeAnonymous {
				return ua.UserTokenTypeAnonymous, nil
			}
		}
		return 0, fmt.Errorf("anonymous authentication not supported by endpoint")
	case AuthModeUsername:
		// Check if username authentication is supported
		for _, token := range endpoint.UserIdentityTokens {
			if token.TokenType == ua.UserTokenTypeUserName {
				return ua.UserTokenTypeUserName, nil
			}
		}
		return 0, fmt.Errorf("username authentication not supported by endpoint")
	case AuthModeCertificate:
		// Check if certificate authentication is supported
		for _, token := range endpoint.UserIdentityTokens {
			if token.TokenType == ua.UserTokenTypeCertificate {
				return ua.UserTokenTypeCertificate, nil
			}
		}
		return 0, fmt.Errorf("certificate authentication not supported by endpoint")
	default:
		return 0, fmt.Errorf("unsupported authentication mode: %s", authMode)
	}
}
