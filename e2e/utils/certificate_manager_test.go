package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"software.sslmate.com/src/go-pkcs12"
)

func TestCertificateManager(t *testing.T) {
	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "cert-manager-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create certificate manager
	manager, err := NewCertificateManager(tempDir)
	require.NoError(t, err)
	defer manager.Close()

	t.Run("convertPKCS12ToPEM", func(t *testing.T) {
		// Generate a test certificate and private key
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

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
			NotBefore:    time.Now(),
			NotAfter:     time.Now().Add(365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses:  nil,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
		require.NoError(t, err)

		cert, err := x509.ParseCertificate(certDER)
		require.NoError(t, err)

		// Encode as PKCS12 with empty password
		pfxData, err := pkcs12.Encode(rand.Reader, privateKey, cert, nil, "")
		require.NoError(t, err)

		// Write PKCS12 data to temp file
		pkcs12Path := filepath.Join(tempDir, "test.pfx")
		err = os.WriteFile(pkcs12Path, pfxData, 0600)
		require.NoError(t, err)

		// Convert PKCS12 to PEM
		certPath, keyPath, err := manager.convertPKCS12File(pkcs12Path)
		require.NoError(t, err)

		// Verify PEM files exist and are readable
		assert.FileExists(t, certPath, "Certificate PEM file should exist")
		assert.FileExists(t, keyPath, "Private key PEM file should exist")

		// Verify PEM files contain valid data
		certData, err := os.ReadFile(certPath)
		require.NoError(t, err)
		assert.Contains(t, string(certData), "BEGIN CERTIFICATE", "Certificate file should contain PEM certificate")

		keyData, err := os.ReadFile(keyPath)
		require.NoError(t, err)
		assert.True(t, 
			string(keyData) != "" && (
				assert.Contains(t, string(keyData), "BEGIN RSA PRIVATE KEY") ||
				assert.Contains(t, string(keyData), "BEGIN PRIVATE KEY")),
			"Private key file should contain PEM private key")
	})

	t.Run("convertDERToPEM", func(t *testing.T) {
		// Generate a test certificate
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		template := x509.Certificate{
			SerialNumber: big.NewInt(1),
			Subject: pkix.Name{
				Organization: []string{"Test Org"},
				Country:      []string{"US"},
			},
			NotBefore: time.Now(),
			NotAfter:  time.Now().Add(365 * 24 * time.Hour),
			KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
		require.NoError(t, err)

		// Write DER data to temp file
		derPath := filepath.Join(tempDir, "test.der")
		err = os.WriteFile(derPath, certDER, 0600)
		require.NoError(t, err)

		// Convert DER to PEM
		pemPath, err := manager.convertDERToPEM(derPath, "test_cert")
		require.NoError(t, err)

		// Verify PEM file exists and contains valid data
		assert.FileExists(t, pemPath, "PEM file should exist")
		
		pemData, err := os.ReadFile(pemPath)
		require.NoError(t, err)
		assert.Contains(t, string(pemData), "BEGIN CERTIFICATE", "PEM file should contain certificate")
	})
}

func TestCertificateManagerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temporary directory for test files
	tempDir, err := os.MkdirTemp("", "cert-manager-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create certificate manager
	manager, err := NewCertificateManager(tempDir)
	require.NoError(t, err)
	defer manager.Close()

	t.Run("findFileInContainer", func(t *testing.T) {
		// This test would require a running container, so we skip it for now
		// In a real integration test, you would:
		// 1. Start a test container with known certificate files
		// 2. Call findFileInContainer to locate them
		// 3. Verify the correct paths are returned
		t.Skip("Requires running container - implement when needed")
	})

	t.Run("copyFileFromContainer", func(t *testing.T) {
		// This test would require a running container, so we skip it for now
		// In a real integration test, you would:
		// 1. Start a test container with known files
		// 2. Call copyFileFromContainer to extract them
		// 3. Verify the files are correctly copied
		t.Skip("Requires running container - implement when needed")
	})
}

func TestCertificateManagerErrorHandling(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cert-manager-error-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	manager, err := NewCertificateManager(tempDir)
	require.NoError(t, err)
	defer manager.Close()

	t.Run("convertPKCS12File with invalid file", func(t *testing.T) {
		invalidPath := filepath.Join(tempDir, "nonexistent.pfx")
		_, _, err := manager.convertPKCS12File(invalidPath)
		assert.Error(t, err, "Should fail with nonexistent file")
	})

	t.Run("convertPKCS12File with invalid PKCS12 data", func(t *testing.T) {
		invalidPath := filepath.Join(tempDir, "invalid.pfx")
		err := os.WriteFile(invalidPath, []byte("not a valid PKCS12 file"), 0600)
		require.NoError(t, err)

		_, _, err = manager.convertPKCS12File(invalidPath)
		assert.Error(t, err, "Should fail with invalid PKCS12 data")
	})

	t.Run("convertDERToPEM with invalid file", func(t *testing.T) {
		invalidPath := filepath.Join(tempDir, "nonexistent.der")
		_, err := manager.convertDERToPEM(invalidPath, "test")
		assert.Error(t, err, "Should fail with nonexistent file")
	})

	t.Run("convertDERToPEM with invalid DER data", func(t *testing.T) {
		invalidPath := filepath.Join(tempDir, "invalid.der")
		err := os.WriteFile(invalidPath, []byte("not a valid DER file"), 0600)
		require.NoError(t, err)

		_, err = manager.convertDERToPEM(invalidPath, "test")
		assert.Error(t, err, "Should fail with invalid DER data")
	})
}