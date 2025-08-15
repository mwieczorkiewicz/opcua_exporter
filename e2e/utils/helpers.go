package utils

import (
	"fmt"
	"net"
	"time"
)


// waitForTCPPort waits for a TCP port to be available
func waitForTCPPort(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for TCP port %s", address)
}