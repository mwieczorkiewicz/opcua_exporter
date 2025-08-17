package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCertificateFiles(t *testing.T) {
	tests := []struct {
		name            string
		certificateFile string
		privateKeyFile  string
		setupFiles      func(t *testing.T) (string, string)
		expectedError   string
	}{
		{
			name:            "both files empty should be valid",
			certificateFile: "",
			privateKeyFile:  "",
			expectedError:   "",
		},
		{
			name:            "certificate file specified but private key missing",
			certificateFile: "cert.pem",
			privateKeyFile:  "",
			expectedError:   "certificate file specified but private key file is empty",
		},
		{
			name:            "private key file specified but certificate missing",
			certificateFile: "",
			privateKeyFile:  "key.pem",
			expectedError:   "private key file specified but certificate file is empty",
		},
		{
			name: "certificate file does not exist",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				keyFile := filepath.Join(tempDir, "key.pem")
				require.NoError(t, os.WriteFile(keyFile, []byte("fake key"), 0600))
				return filepath.Join(tempDir, "nonexistent.pem"), keyFile
			},
			expectedError: "certificate file does not exist",
		},
		{
			name: "private key file does not exist",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				certFile := filepath.Join(tempDir, "cert.pem")
				require.NoError(t, os.WriteFile(certFile, []byte("fake cert"), 0644))
				return certFile, filepath.Join(tempDir, "nonexistent.pem")
			},
			expectedError: "private key file does not exist",
		},
		{
			name: "both files exist should be valid",
			setupFiles: func(t *testing.T) (string, string) {
				tempDir := t.TempDir()
				certFile := filepath.Join(tempDir, "cert.pem")
				keyFile := filepath.Join(tempDir, "key.pem")
				require.NoError(t, os.WriteFile(certFile, []byte("fake cert"), 0644))
				require.NoError(t, os.WriteFile(keyFile, []byte("fake key"), 0600))
				return certFile, keyFile
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certFile, keyFile := tt.certificateFile, tt.privateKeyFile
			if tt.setupFiles != nil {
				certFile, keyFile = tt.setupFiles(t)
			}

			err := ValidateCertificateFiles(certFile, keyFile)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestCertificateManager_LoadCertificate(t *testing.T) {
	t.Run("missing certificate file", func(t *testing.T) {
		cm := NewCertificateManager("", "key.pem")
		_, err := cm.LoadCertificate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "certificate file and private key file must both be specified")
	})

	t.Run("missing private key file", func(t *testing.T) {
		cm := NewCertificateManager("cert.pem", "")
		_, err := cm.LoadCertificate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "certificate file and private key file must both be specified")
	})

	t.Run("valid certificate and key", func(t *testing.T) {
		// Create a temporary certificate and key
		certFile, keyFile := createTestCertificateFiles(t)
		
		cm := NewCertificateManager(certFile, keyFile)
		cert, err := cm.LoadCertificate()
		
		assert.NoError(t, err)
		assert.NotEmpty(t, cert.Certificate)
		assert.NotNil(t, cert.PrivateKey)
	})
}

func TestLoadCertificateFromPEM(t *testing.T) {
	t.Run("invalid certificate PEM", func(t *testing.T) {
		_, err := LoadCertificateFromPEM([]byte("invalid cert"), []byte("invalid key"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode certificate PEM")
	})

	t.Run("invalid private key PEM", func(t *testing.T) {
		certPEM := []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCAS+gAwIBAgIJAKnL4UEDMN/FMA0GCSqGSIb3DQEBBQUAMFkxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIEwpTb21lLVN0YXRlMSEwHwYDVQQKExhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQxEjAQBgNVBAMTCWxvY2FsaG9zdDAeFw0yMDA0MDIwNjMy
MzhaFw0zMDA0MDIwNjMyMzhaMFkxCzAJBgNVBAYTAkFVMRMwEQYDVQQIEwpTb21l
LVN0YXRlMSEwHwYDVQQKExhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQxEjAQBgNV
BAMTCWxvY2FsaG9zdDBJMAoGCCqGSM49BAMCQQDeAyLhzqKt4xWMg7Y=
-----END CERTIFICATE-----`)
		
		_, err := LoadCertificateFromPEM(certPEM, []byte("invalid key"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode private key PEM")
	})
}

// createTestCertificateFiles creates temporary certificate and key files for testing
func createTestCertificateFiles(t *testing.T) (string, string) {
	// Generate a test RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create a test certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test Org"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Write certificate to temp file
	tempDir := t.TempDir()
	certFile := filepath.Join(tempDir, "test_cert.pem")
	keyFile := filepath.Join(tempDir, "test_key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, os.WriteFile(certFile, certPEM, 0644))

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER})
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0600))

	return certFile, keyFile
}