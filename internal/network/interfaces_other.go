//go:build !linux

package network

import "fmt"

func fallbackInterfaces(primaryErr error) ([]Interface, error) {
	return nil, fmt.Errorf("network interface enumeration failed: %w", primaryErr)
}
