package utils

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"software.sslmate.com/src/go-pkcs12"
)

// CertificateManager handles OPC-UA certificate extraction and conversion using Docker Go client
type CertificateManager struct {
	dockerClient *client.Client
	workingDir   string
}

// NewCertificateManager creates a new Docker-based certificate manager
func NewCertificateManager(workingDir string) (*CertificateManager, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &CertificateManager{
		dockerClient: dockerClient,
		workingDir:   workingDir,
	}, nil
}

// Close closes the Docker client connection
func (cm *CertificateManager) Close() error {
	if cm.dockerClient != nil {
		return cm.dockerClient.Close()
	}
	return nil
}

// ExtractAndConvertCertificates extracts certificates from the OPC-UA server container and converts them to PEM format
func (cm *CertificateManager) ExtractAndConvertCertificates(ctx context.Context, containerID string) (*CertificateInfo, error) {
	// Extract the raw certificate files from the container using Docker API
	rawCertInfo, err := cm.extractRawCertificatesWithDockerAPI(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to extract raw certificates: %w", err)
	}

	// Convert PKCS12 to PEM format
	pemCertInfo, err := cm.convertPKCS12ToPEM(rawCertInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to convert PKCS12 to PEM: %w", err)
	}

	return pemCertInfo, nil
}

// extractRawCertificatesWithDockerAPI extracts certificate files from the container using Docker Go client
func (cm *CertificateManager) extractRawCertificatesWithDockerAPI(ctx context.Context, containerID string) (*CertificateInfo, error) {
	// Define the certificate paths within the container
	// First, let's explore the full PKI structure to find all available certificates
	certPaths := map[string]string{
		"server_cert":  "/app/pki/own/certs",
		"server_key":   "/app/pki/own/private",
		"trusted_cert": "/app/pki/trusted/certs",
	}

	// We'll explore additional paths in the exploreContainerPKIStructure function

	extractedFiles := make(map[string]string)

	// Optional: explore PKI structure for debugging (can be removed for production)
	// Uncomment the next 4 lines if you need to debug certificate structure:
	// if err := cm.exploreContainerPKIStructure(ctx, containerID); err != nil {
	//     // Don't fail if exploration fails, just log and continue
	//     fmt.Printf("Warning: Could not explore PKI structure: %v\n", err)
	// }

	for certType, containerPath := range certPaths {
		// First, find the actual filename in the directory using Docker API
		actualFile, err := cm.findFileInContainerWithAPI(ctx, containerID, containerPath)
		if err != nil {
			return nil, fmt.Errorf("failed to find %s file in %s: %w", certType, containerPath, err)
		}

		// Extract the file to the working directory using Docker API
		actualFile = sanitizePath(actualFile) // Ensure the path is sanitized
		destPath := filepath.Join(cm.workingDir, fmt.Sprintf("raw_%s_%s", certType, filepath.Base(actualFile)))
		if err := cm.copyFileFromContainerWithAPI(ctx, containerID, actualFile, destPath); err != nil {
			return nil, fmt.Errorf("failed to copy %s from container: %w", certType, err)
		}

		extractedFiles[certType] = destPath
	}

	return &CertificateInfo{
		CertificateFile: extractedFiles["server_cert"],
		PrivateKeyFile:  extractedFiles["server_key"],
		TrustedCertFile: extractedFiles["trusted_cert"],
	}, nil
}

func sanitizePath(p string) string {
	// strip control chars
	p = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, p)

	// normalize and use forward slashes
	return filepath.ToSlash(filepath.Clean(p))
}

// findFileInContainerWithAPI finds the first file in the specified container directory using Docker API
func (cm *CertificateManager) findFileInContainerWithAPI(ctx context.Context, containerID, dirPath string) (string, error) {
	// Create an exec configuration to list files in the directory
	execConfig := container.ExecOptions{
		Cmd:          []string{"find", dirPath, "-type", "f", "-name", "*"},
		AttachStdout: true,
		AttachStderr: true,
	}

	// Create the exec instance
	execIDResp, err := cm.dockerClient.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec for file listing: %w", err)
	}

	// Attach to the exec instance
	attachResp, err := cm.dockerClient.ContainerExecAttach(ctx, execIDResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer attachResp.Close()

	// Read the output to get the file path
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, io.Discard, attachResp.Reader); err != nil {
		return "", fmt.Errorf("failed to read exec output: %w", err)
	}

	outputStr := strings.TrimSpace(buf.String())
	if outputStr == "" {
		return "", fmt.Errorf("no files found in directory %s", dirPath)
	}

	// Return the first file found
	lines := strings.Split(outputStr, "\n")
	if len(lines) > 0 && len(lines[0]) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}

	return "", fmt.Errorf("no valid file paths found in directory %s", dirPath)
}

// copyFileFromContainerWithAPI copies a single file from the container to the host using Docker API
func (cm *CertificateManager) copyFileFromContainerWithAPI(ctx context.Context, containerID, srcPath, destPath string) error {
	// Get the file content from the container using Docker API
	reader, _, err := cm.dockerClient.CopyFromContainer(ctx, containerID, srcPath)
	if err != nil {
		return fmt.Errorf("failed to copy from container: %w", err)
	}
	defer reader.Close()

	// Create the destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Extract the file from the tar stream
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Skip directories, we only want the file content
		if header.Typeflag == tar.TypeReg {
			if _, err := io.Copy(destFile, tarReader); err != nil {
				return fmt.Errorf("failed to write file content: %w", err)
			}
			break
		}
	}

	return nil
}

// convertPKCS12ToPEM converts PKCS12 certificates to PEM format
func (cm *CertificateManager) convertPKCS12ToPEM(rawCertInfo *CertificateInfo) (*CertificateInfo, error) {
	// The private key file is in PKCS12 format and needs conversion
	pemCertPath, pemKeyPath, err := cm.convertPKCS12File(rawCertInfo.PrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to convert PKCS12 file: %w", err)
	}

	// The trusted certificate might also be in DER format
	pemTrustedCertPath, err := cm.convertDERToPEM(rawCertInfo.TrustedCertFile, "trusted_certificate")
	if err != nil {
		return nil, fmt.Errorf("failed to convert trusted certificate: %w", err)
	}

	return &CertificateInfo{
		CertificateFile: pemCertPath,        // Use the certificate from PKCS12 (more reliable)
		PrivateKeyFile:  pemKeyPath,         // Private key from PKCS12
		TrustedCertFile: pemTrustedCertPath, // Server's public cert for trust
	}, nil
}

// convertPKCS12File converts a PKCS12 file to PEM certificate and private key
func (cm *CertificateManager) convertPKCS12File(pkcs12Path string) (certPath, keyPath string, err error) {
	// Read the PKCS12 file
	pkcs12Data, err := os.ReadFile(pkcs12Path)
	if err != nil {
		return "", "", fmt.Errorf("failed to read PKCS12 file: %w", err)
	}

	// Try to decode with empty password first (common for test certificates)
	privateKey, cert, caCerts, err := pkcs12.DecodeChain(pkcs12Data, "")
	if err != nil {
		// If that fails, try with "password" as the password
		privateKey, cert, caCerts, err = pkcs12.DecodeChain(pkcs12Data, "password")
		if err != nil {
			// Try other common passwords
			for _, password := range []string{"123456", "test", "opcua"} {
				privateKey, cert, caCerts, err = pkcs12.DecodeChain(pkcs12Data, password)
				if err == nil {
					break
				}
			}
			if err != nil {
				return "", "", fmt.Errorf("failed to decode PKCS12 with any common password: %w", err)
			}
		}
	}

	// Convert certificate to PEM
	certPath = filepath.Join(cm.workingDir, "client-cert.pem")
	certFile, err := os.Create(certPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create certificate file: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}); err != nil {
		return "", "", fmt.Errorf("failed to encode certificate to PEM: %w", err)
	}

	// Add any CA certificates to the chain
	for _, caCert := range caCerts {
		if err := pem.Encode(certFile, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: caCert.Raw,
		}); err != nil {
			return "", "", fmt.Errorf("failed to encode CA certificate to PEM: %w", err)
		}
	}

	// Convert private key to PEM
	keyPath = filepath.Join(cm.workingDir, "client-key.pem")
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create private key file: %w", err)
	}
	defer keyFile.Close()

	var keyBytes []byte
	var keyType string

	switch key := privateKey.(type) {
	case *rsa.PrivateKey:
		keyBytes = x509.MarshalPKCS1PrivateKey(key)
		keyType = "RSA PRIVATE KEY"
	default:
		// Try PKCS8 encoding for unknown key types
		keyBytes, err = x509.MarshalPKCS8PrivateKey(privateKey)
		if err != nil {
			return "", "", fmt.Errorf("failed to marshal private key (unknown type): %w", err)
		}
		keyType = "PRIVATE KEY"
	}

	if err := pem.Encode(keyFile, &pem.Block{
		Type:  keyType,
		Bytes: keyBytes,
	}); err != nil {
		return "", "", fmt.Errorf("failed to encode private key to PEM: %w", err)
	}

	return certPath, keyPath, nil
}

// convertDERToPEM converts a DER-encoded certificate to PEM format
func (cm *CertificateManager) convertDERToPEM(derPath, certType string) (string, error) {
	// Read the DER file
	derData, err := os.ReadFile(derPath)
	if err != nil {
		return "", fmt.Errorf("failed to read DER file: %w", err)
	}

	// Try to parse as DER first
	cert, err := x509.ParseCertificate(derData)
	if err != nil {
		// Maybe it's already in PEM format, try parsing as PEM
		block, _ := pem.Decode(derData)
		if block == nil {
			return "", fmt.Errorf("failed to decode certificate as DER or PEM: %w", err)
		}
		cert, err = x509.ParseCertificate(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("failed to parse certificate from PEM block: %w", err)
		}
		// Already in PEM format, just copy it
		return derPath, nil
	}

	// Convert to PEM
	pemPath := filepath.Join(cm.workingDir, fmt.Sprintf("%s.pem", certType))
	pemFile, err := os.Create(pemPath)
	if err != nil {
		return "", fmt.Errorf("failed to create PEM file: %w", err)
	}
	defer pemFile.Close()

	if err := pem.Encode(pemFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}); err != nil {
		return "", fmt.Errorf("failed to encode certificate to PEM: %w", err)
	}

	return pemPath, nil
}

// exploreContainerPKIStructure explores the container's PKI directory structure to find available certificates
func (cm *CertificateManager) exploreContainerPKIStructure(ctx context.Context, containerID string) error {
	// Explore the main PKI directory structure
	pkiPaths := []string{
		"/app/pki",
		"/app/pki/own", 
		"/app/pki/own/certs",
		"/app/pki/own/private",
		"/app/pki/trusted",
		"/app/pki/trusted/certs",
		"/app/pki/rejected",
		"/app/pki/rejected/certs",
		"/app/pki/issuer",
		"/app/pki/issuer/certs",
		"/app/pki/issuer/private",
	}

	fmt.Printf("Exploring container PKI structure:\n")
	for _, path := range pkiPaths {
		if err := cm.listDirectoryContents(ctx, containerID, path); err != nil {
			fmt.Printf("  %s: Not accessible or empty\n", path)
		}
	}

	return nil
}

// listDirectoryContents lists the contents of a directory in the container
func (cm *CertificateManager) listDirectoryContents(ctx context.Context, containerID, dirPath string) error {
	// Create an exec configuration to list directory contents
	execConfig := container.ExecOptions{
		Cmd:          []string{"ls", "-la", dirPath},
		AttachStdout: true,
		AttachStderr: true,
	}

	// Create the exec instance
	execIDResp, err := cm.dockerClient.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return fmt.Errorf("failed to create exec for directory listing: %w", err)
	}

	// Attach to the exec instance
	attachResp, err := cm.dockerClient.ContainerExecAttach(ctx, execIDResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer attachResp.Close()

	// Read the output to get the directory listing
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, io.Discard, attachResp.Reader); err != nil {
		return fmt.Errorf("failed to read exec output: %w", err)
	}

	output := strings.TrimSpace(buf.String())
	if output != "" {
		fmt.Printf("  %s:\n", dirPath)
		for _, line := range strings.Split(output, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}

	return nil
}
