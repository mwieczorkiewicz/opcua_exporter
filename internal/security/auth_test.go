package security

import (
	"testing"

	"github.com/gopcua/opcua/ua"
	"github.com/stretchr/testify/assert"
)

func TestValidateAuthConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        AuthConfig
		expectedError string
	}{
		{
			name: "valid anonymous config",
			config: AuthConfig{
				Mode: AuthModeAnonymous,
			},
			expectedError: "",
		},
		{
			name: "valid username config",
			config: AuthConfig{
				Mode:     AuthModeUsername,
				Username: "testuser",
				Password: "testpass",
			},
			expectedError: "",
		},
		{
			name: "username config missing username",
			config: AuthConfig{
				Mode:     AuthModeUsername,
				Password: "testpass",
			},
			expectedError: "username is required for username authentication",
		},
		{
			name: "invalid auth mode",
			config: AuthConfig{
				Mode: "InvalidMode",
			},
			expectedError: "unsupported authentication mode: InvalidMode",
		},
		{
			name: "certificate config with missing files",
			config: AuthConfig{
				Mode:            AuthModeCertificate,
				CertificateFile: "",
				PrivateKeyFile:  "",
			},
			expectedError: "", // ValidateCertificateFiles should handle this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAuthConfig(tt.config)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestValidateSecurityConfig(t *testing.T) {
	tests := []struct {
		name           string
		securityMode   string
		securityPolicy string
		expectedError  string
	}{
		{
			name:           "valid none/none config",
			securityMode:   SecurityModeNone,
			securityPolicy: SecurityPolicyNone,
			expectedError:  "",
		},
		{
			name:           "valid sign with policy",
			securityMode:   SecurityModeSign,
			securityPolicy: SecurityPolicyBasic256Sha256,
			expectedError:  "",
		},
		{
			name:           "invalid security mode",
			securityMode:   "InvalidMode",
			securityPolicy: SecurityPolicyNone,
			expectedError:  "unsupported security mode: InvalidMode",
		},
		{
			name:           "invalid security policy",
			securityMode:   SecurityModeNone,
			securityPolicy: "InvalidPolicy",
			expectedError:  "unsupported security policy: InvalidPolicy",
		},
		{
			name:           "none mode with non-none policy",
			securityMode:   SecurityModeNone,
			securityPolicy: SecurityPolicyBasic256,
			expectedError:  "security mode 'None' requires security policy 'None'",
		},
		{
			name:           "sign mode with none policy",
			securityMode:   SecurityModeSign,
			securityPolicy: SecurityPolicyNone,
			expectedError:  "security mode 'Sign' requires a security policy other than 'None'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecurityConfig(tt.securityMode, tt.securityPolicy)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestSecurityEndpointSelector_parseSecurityMode(t *testing.T) {
	tests := []struct {
		name         string
		securityMode string
		expected     ua.MessageSecurityMode
		expectError  bool
	}{
		{
			name:         "none mode",
			securityMode: SecurityModeNone,
			expected:     ua.MessageSecurityModeNone,
			expectError:  false,
		},
		{
			name:         "sign mode",
			securityMode: SecurityModeSign,
			expected:     ua.MessageSecurityModeSign,
			expectError:  false,
		},
		{
			name:         "sign and encrypt mode",
			securityMode: SecurityModeSignAndEncrypt,
			expected:     ua.MessageSecurityModeSignAndEncrypt,
			expectError:  false,
		},
		{
			name:         "invalid mode",
			securityMode: "InvalidMode",
			expected:     0,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewSecurityEndpointSelector(tt.securityMode, SecurityPolicyNone, AuthModeAnonymous, false)
			result, err := selector.parseSecurityMode()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSecurityEndpointSelector_parseSecurityPolicy(t *testing.T) {
	tests := []struct {
		name           string
		securityPolicy string
		expected       string
		expectError    bool
	}{
		{
			name:           "none policy",
			securityPolicy: SecurityPolicyNone,
			expected:       "None",
			expectError:    false,
		},
		{
			name:           "basic128rsa15 policy",
			securityPolicy: SecurityPolicyBasic128Rsa15,
			expected:       "Basic128Rsa15",
			expectError:    false,
		},
		{
			name:           "basic256 policy",
			securityPolicy: SecurityPolicyBasic256,
			expected:       "Basic256",
			expectError:    false,
		},
		{
			name:           "basic256sha256 policy",
			securityPolicy: SecurityPolicyBasic256Sha256,
			expected:       "Basic256Sha256",
			expectError:    false,
		},
		{
			name:           "invalid policy",
			securityPolicy: "InvalidPolicy",
			expected:       "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewSecurityEndpointSelector(SecurityModeNone, tt.securityPolicy, AuthModeAnonymous, false)
			result, err := selector.parseSecurityPolicy()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestSecurityEndpointSelector_isAuthModeSupported(t *testing.T) {
	// Create test endpoints with different authentication tokens
	anonymousEndpoint := &ua.EndpointDescription{
		UserIdentityTokens: []*ua.UserTokenPolicy{
			{TokenType: ua.UserTokenTypeAnonymous},
		},
	}

	usernameEndpoint := &ua.EndpointDescription{
		UserIdentityTokens: []*ua.UserTokenPolicy{
			{TokenType: ua.UserTokenTypeUserName},
		},
	}

	certificateEndpoint := &ua.EndpointDescription{
		UserIdentityTokens: []*ua.UserTokenPolicy{
			{TokenType: ua.UserTokenTypeCertificate},
		},
	}

	multiAuthEndpoint := &ua.EndpointDescription{
		UserIdentityTokens: []*ua.UserTokenPolicy{
			{TokenType: ua.UserTokenTypeAnonymous},
			{TokenType: ua.UserTokenTypeUserName},
		},
	}

	tests := []struct {
		name     string
		authMode string
		endpoint *ua.EndpointDescription
		expected bool
	}{
		{
			name:     "anonymous auth supported",
			authMode: AuthModeAnonymous,
			endpoint: anonymousEndpoint,
			expected: true,
		},
		{
			name:     "anonymous auth not supported",
			authMode: AuthModeAnonymous,
			endpoint: usernameEndpoint,
			expected: false,
		},
		{
			name:     "username auth supported",
			authMode: AuthModeUsername,
			endpoint: usernameEndpoint,
			expected: true,
		},
		{
			name:     "username auth not supported",
			authMode: AuthModeUsername,
			endpoint: certificateEndpoint,
			expected: false,
		},
		{
			name:     "certificate auth supported",
			authMode: AuthModeCertificate,
			endpoint: certificateEndpoint,
			expected: true,
		},
		{
			name:     "certificate auth not supported",
			authMode: AuthModeCertificate,
			endpoint: anonymousEndpoint,
			expected: false,
		},
		{
			name:     "anonymous auth in multi-auth endpoint",
			authMode: AuthModeAnonymous,
			endpoint: multiAuthEndpoint,
			expected: true,
		},
		{
			name:     "username auth in multi-auth endpoint",
			authMode: AuthModeUsername,
			endpoint: multiAuthEndpoint,
			expected: true,
		},
		{
			name:     "certificate auth in multi-auth endpoint",
			authMode: AuthModeCertificate,
			endpoint: multiAuthEndpoint,
			expected: false,
		},
		{
			name:     "invalid auth mode",
			authMode: "InvalidMode",
			endpoint: anonymousEndpoint,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selector := NewSecurityEndpointSelector(SecurityModeNone, SecurityPolicyNone, tt.authMode, false)
			result := selector.isAuthModeSupported(tt.endpoint)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateSecurityConfigWithCertificates(t *testing.T) {
	tests := []struct {
		name            string
		securityMode    string
		securityPolicy  string
		certificateFile string
		privateKeyFile  string
		expectedError   string
	}{
		{
			name:            "none security mode - no certificates required",
			securityMode:    SecurityModeNone,
			securityPolicy:  SecurityPolicyNone,
			certificateFile: "",
			privateKeyFile:  "",
			expectedError:   "",
		},
		{
			name:            "sign mode with Basic256Sha256 - certificates required but missing",
			securityMode:    SecurityModeSign,
			securityPolicy:  SecurityPolicyBasic256Sha256,
			certificateFile: "",
			privateKeyFile:  "",
			expectedError:   "security policy 'Basic256Sha256' with mode 'Sign' requires both certificate and private key files to be specified",
		},
		{
			name:            "signandencrypt mode with Basic256 - certificates required but missing",
			securityMode:    SecurityModeSignAndEncrypt,
			securityPolicy:  SecurityPolicyBasic256,
			certificateFile: "",
			privateKeyFile:  "",
			expectedError:   "security policy 'Basic256' with mode 'SignAndEncrypt' requires both certificate and private key files to be specified",
		},
		{
			name:            "sign mode with Basic128Rsa15 - certificates required but missing",
			securityMode:    SecurityModeSign,
			securityPolicy:  SecurityPolicyBasic128Rsa15,
			certificateFile: "",
			privateKeyFile:  "",
			expectedError:   "security policy 'Basic128Rsa15' with mode 'Sign' requires both certificate and private key files to be specified",
		},
		{
			name:            "secure policy with missing certificate file only",
			securityMode:    SecurityModeSign,
			securityPolicy:  SecurityPolicyBasic256Sha256,
			certificateFile: "",
			privateKeyFile:  "key.pem",
			expectedError:   "security policy 'Basic256Sha256' with mode 'Sign' requires both certificate and private key files to be specified",
		},
		{
			name:            "secure policy with missing private key file only",
			securityMode:    SecurityModeSign,
			securityPolicy:  SecurityPolicyBasic256Sha256,
			certificateFile: "cert.pem",
			privateKeyFile:  "",
			expectedError:   "security policy 'Basic256Sha256' with mode 'Sign' requires both certificate and private key files to be specified",
		},
		{
			name:            "secure policy with nonexistent certificate files",
			securityMode:    SecurityModeSign,
			securityPolicy:  SecurityPolicyBasic256Sha256,
			certificateFile: "nonexistent_cert.pem",
			privateKeyFile:  "nonexistent_key.pem",
			expectedError:   "certificate validation failed for security policy 'Basic256Sha256'",
		},
		{
			name:            "none policy with sign mode - should fail basic validation first",
			securityMode:    SecurityModeSign,
			securityPolicy:  SecurityPolicyNone,
			certificateFile: "",
			privateKeyFile:  "",
			expectedError:   "security mode 'Sign' requires a security policy other than 'None'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecurityConfigWithCertificates(tt.securityMode, tt.securityPolicy, tt.certificateFile, tt.privateKeyFile)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestRequiresCertificatesForSecurity(t *testing.T) {
	tests := []struct {
		name           string
		securityMode   string
		securityPolicy string
		expected       bool
	}{
		{
			name:           "none mode never requires certificates",
			securityMode:   SecurityModeNone,
			securityPolicy: SecurityPolicyNone,
			expected:       false,
		},
		{
			name:           "none mode with any policy never requires certificates",
			securityMode:   SecurityModeNone,
			securityPolicy: SecurityPolicyBasic256Sha256,
			expected:       false,
		},
		{
			name:           "sign mode with none policy does not require certificates",
			securityMode:   SecurityModeSign,
			securityPolicy: SecurityPolicyNone,
			expected:       false,
		},
		{
			name:           "sign mode with Basic256Sha256 requires certificates",
			securityMode:   SecurityModeSign,
			securityPolicy: SecurityPolicyBasic256Sha256,
			expected:       true,
		},
		{
			name:           "signandencrypt mode with Basic256 requires certificates",
			securityMode:   SecurityModeSignAndEncrypt,
			securityPolicy: SecurityPolicyBasic256,
			expected:       true,
		},
		{
			name:           "sign mode with Basic128Rsa15 requires certificates",
			securityMode:   SecurityModeSign,
			securityPolicy: SecurityPolicyBasic128Rsa15,
			expected:       true,
		},
		{
			name:           "unknown security policy with sign mode requires certificates",
			securityMode:   SecurityModeSign,
			securityPolicy: "UnknownPolicy",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := requiresCertificatesForSecurity(tt.securityMode, tt.securityPolicy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateInsecureClientOptions(t *testing.T) {
	t.Run("anonymous authentication", func(t *testing.T) {
		authConfig := AuthConfig{
			Mode: AuthModeAnonymous,
		}
		
		options, err := CreateInsecureClientOptions(authConfig, false)
		
		assert.NoError(t, err)
		assert.NotEmpty(t, options)
	})

	t.Run("username authentication", func(t *testing.T) {
		authConfig := AuthConfig{
			Mode:     AuthModeUsername,
			Username: "testuser",
			Password: "testpass",
		}
		
		options, err := CreateInsecureClientOptions(authConfig, false)
		
		assert.NoError(t, err)
		assert.NotEmpty(t, options)
	})

	t.Run("certificate authentication with missing files", func(t *testing.T) {
		authConfig := AuthConfig{
			Mode:            AuthModeCertificate,
			CertificateFile: "nonexistent.pem",
			PrivateKeyFile:  "nonexistent.pem",
		}
		
		options, err := CreateInsecureClientOptions(authConfig, false)
		
		assert.Error(t, err)
		assert.Nil(t, options)
		assert.Contains(t, err.Error(), "failed to load certificate")
	})
}

func TestGetUserTokenTypeForAuth(t *testing.T) {
	// Create test endpoints with different authentication tokens
	anonymousEndpoint := &ua.EndpointDescription{
		UserIdentityTokens: []*ua.UserTokenPolicy{
			{TokenType: ua.UserTokenTypeAnonymous},
		},
	}

	usernameEndpoint := &ua.EndpointDescription{
		UserIdentityTokens: []*ua.UserTokenPolicy{
			{TokenType: ua.UserTokenTypeUserName},
		},
	}

	certificateEndpoint := &ua.EndpointDescription{
		UserIdentityTokens: []*ua.UserTokenPolicy{
			{TokenType: ua.UserTokenTypeCertificate},
		},
	}

	multiAuthEndpoint := &ua.EndpointDescription{
		UserIdentityTokens: []*ua.UserTokenPolicy{
			{TokenType: ua.UserTokenTypeAnonymous},
			{TokenType: ua.UserTokenTypeUserName},
		},
	}

	tests := []struct {
		name          string
		authMode      string
		endpoint      *ua.EndpointDescription
		expectedType  ua.UserTokenType
		expectedError string
	}{
		{
			name:         "anonymous auth supported",
			authMode:     AuthModeAnonymous,
			endpoint:     anonymousEndpoint,
			expectedType: ua.UserTokenTypeAnonymous,
		},
		{
			name:          "anonymous auth not supported",
			authMode:      AuthModeAnonymous,
			endpoint:      usernameEndpoint,
			expectedError: "anonymous authentication not supported by endpoint",
		},
		{
			name:         "username auth supported",
			authMode:     AuthModeUsername,
			endpoint:     usernameEndpoint,
			expectedType: ua.UserTokenTypeUserName,
		},
		{
			name:          "username auth not supported",
			authMode:      AuthModeUsername,
			endpoint:      certificateEndpoint,
			expectedError: "username authentication not supported by endpoint",
		},
		{
			name:         "certificate auth supported",
			authMode:     AuthModeCertificate,
			endpoint:     certificateEndpoint,
			expectedType: ua.UserTokenTypeCertificate,
		},
		{
			name:          "certificate auth not supported",
			authMode:      AuthModeCertificate,
			endpoint:      anonymousEndpoint,
			expectedError: "certificate authentication not supported by endpoint",
		},
		{
			name:         "anonymous auth in multi-auth endpoint",
			authMode:     AuthModeAnonymous,
			endpoint:     multiAuthEndpoint,
			expectedType: ua.UserTokenTypeAnonymous,
		},
		{
			name:         "username auth in multi-auth endpoint",
			authMode:     AuthModeUsername,
			endpoint:     multiAuthEndpoint,
			expectedType: ua.UserTokenTypeUserName,
		},
		{
			name:          "unsupported auth mode",
			authMode:      "InvalidMode",
			endpoint:      anonymousEndpoint,
			expectedError: "unsupported authentication mode: InvalidMode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getUserTokenTypeForAuth(tt.authMode, tt.endpoint)

			if tt.expectedError == "" {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedType, result)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}