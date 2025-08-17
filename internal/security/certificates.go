package security

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
)

// CertificateManager handles loading and validating certificates for OPC UA connections
type CertificateManager struct {
	certificateFile string
	privateKeyFile  string
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager(certificateFile, privateKeyFile string) *CertificateManager {
	return &CertificateManager{
		certificateFile: certificateFile,
		privateKeyFile:  privateKeyFile,
	}
}

// LoadCertificate loads and validates a certificate and private key pair
func (cm *CertificateManager) LoadCertificate() (tls.Certificate, error) {
	if cm.certificateFile == "" || cm.privateKeyFile == "" {
		return tls.Certificate{}, fmt.Errorf("certificate file and private key file must both be specified")
	}

	// Check if certificate file exists
	if _, err := os.Stat(cm.certificateFile); os.IsNotExist(err) {
		return tls.Certificate{}, fmt.Errorf("certificate file does not exist: %s", cm.certificateFile)
	}

	// Check if private key file exists
	if _, err := os.Stat(cm.privateKeyFile); os.IsNotExist(err) {
		return tls.Certificate{}, fmt.Errorf("private key file does not exist: %s", cm.privateKeyFile)
	}

	// Load certificate and private key
	cert, err := tls.LoadX509KeyPair(cm.certificateFile, cm.privateKeyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to load certificate and key pair: %w", err)
	}

	// Validate certificate
	if err := cm.validateCertificate(cert); err != nil {
		return tls.Certificate{}, fmt.Errorf("certificate validation failed: %w", err)
	}

	log.Printf("Successfully loaded certificate from %s", cm.certificateFile)
	return cert, nil
}

// validateCertificate performs basic validation on the loaded certificate
func (cm *CertificateManager) validateCertificate(cert tls.Certificate) error {
	if len(cert.Certificate) == 0 {
		return fmt.Errorf("certificate chain is empty")
	}

	// Parse the leaf certificate
	leafCert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed to parse leaf certificate: %w", err)
	}

	// Check if certificate is valid for client authentication
	if !cm.isValidForClientAuth(leafCert) {
		log.Printf("Warning: certificate may not be valid for client authentication")
	}

	// Check if certificate has expired
	if leafCert.NotAfter.Before(leafCert.NotBefore) {
		return fmt.Errorf("certificate has invalid validity period")
	}

	// Check key usage if present
	if leafCert.KeyUsage != 0 {
		requiredUsage := x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
		if leafCert.KeyUsage&requiredUsage == 0 {
			log.Printf("Warning: certificate key usage may not be suitable for OPC UA client authentication")
		}
	}

	return nil
}

// isValidForClientAuth checks if the certificate is suitable for client authentication
func (cm *CertificateManager) isValidForClientAuth(cert *x509.Certificate) bool {
	// Check extended key usage
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageClientAuth {
			return true
		}
	}
	
	// If no specific EKU is set, assume it's valid
	return len(cert.ExtKeyUsage) == 0
}

// LoadCertificateFromPEM loads a certificate from PEM data
func LoadCertificateFromPEM(certPEM, keyPEM []byte) (tls.Certificate, error) {
	// Decode certificate
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return tls.Certificate{}, fmt.Errorf("failed to decode certificate PEM")
	}

	// Decode private key
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return tls.Certificate{}, fmt.Errorf("failed to decode private key PEM")
	}

	// Parse private key
	var privateKey interface{}
	var err error

	switch keyBlock.Type {
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	case "PRIVATE KEY":
		privateKey, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	default:
		return tls.Certificate{}, fmt.Errorf("unsupported private key type: %s", keyBlock.Type)
	}

	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Validate that it's an RSA key (OPC UA typically uses RSA)
	if _, ok := privateKey.(*rsa.PrivateKey); !ok {
		return tls.Certificate{}, fmt.Errorf("private key is not RSA")
	}

	return tls.Certificate{
		Certificate: [][]byte{certBlock.Bytes},
		PrivateKey:  privateKey,
	}, nil
}

// ValidateCertificateFiles performs basic file validation without loading the certificates
func ValidateCertificateFiles(certificateFile, privateKeyFile string) error {
	if certificateFile == "" && privateKeyFile == "" {
		return nil // No certificates configured, which is fine
	}

	if certificateFile == "" {
		return fmt.Errorf("private key file specified but certificate file is empty")
	}

	if privateKeyFile == "" {
		return fmt.Errorf("certificate file specified but private key file is empty")
	}

	// Check if files exist
	if _, err := os.Stat(certificateFile); os.IsNotExist(err) {
		return fmt.Errorf("certificate file does not exist: %s", certificateFile)
	}

	if _, err := os.Stat(privateKeyFile); os.IsNotExist(err) {
		return fmt.Errorf("private key file does not exist: %s", privateKeyFile)
	}

	return nil
}