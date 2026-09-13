//go:build linux

package network

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// fallbackInterfaces avoids rtnetlink so the interface inventory still works
// under the lumonasd systemd sandbox, which intentionally does not permit
// AF_NETLINK. The IPv4 ioctl is allowed by the service's AF_INET policy.
func fallbackInterfaces(primaryErr error) ([]Interface, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, fmt.Errorf("%w; sysfs fallback: %v", primaryErr, err)
	}
	result := make([]Interface, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		flags := readSysfsUint(filepath.Join("/sys/class/net", name, "flags"))
		mtu := int(readSysfsUint(filepath.Join("/sys/class/net", name, "mtu")))
		mac := strings.TrimSpace(readSysfs(filepath.Join("/sys/class/net", name, "address")))
		addresses := interfaceIPv4(name)
		result = append(result, Interface{
			Name:      name,
			MAC:       mac,
			MTU:       mtu,
			Up:        flags&uint64(net.FlagUp) != 0,
			Loopback:  name == "lo",
			Wireless:  isWireless(name),
			Addresses: addresses,
		})
	}
	sortInterfaces(result)
	return result, nil
}

func interfaceIPv4(name string) []string {
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		return nil
	}
	defer unix.Close(fd)
	request, err := unix.NewIfreq(name)
	if err != nil || unix.IoctlIfreq(fd, unix.SIOCGIFADDR, request) != nil {
		return nil
	}
	address, err := request.Inet4Addr()
	if err != nil {
		return nil
	}
	return []string{net.IP(address).String()}
}

func readSysfs(path string) string {
	value, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(value)
}

func readSysfsUint(path string) uint64 {
	value := strings.TrimSpace(readSysfs(path))
	if strings.HasPrefix(value, "0x") {
		parsed, _ := strconv.ParseUint(value[2:], 16, 64)
		return parsed
	}
	parsed, _ := strconv.ParseUint(value, 10, 64)
	return parsed
}

func sortInterfaces(items []Interface) {
	for left := 0; left < len(items); left++ {
		for right := left + 1; right < len(items); right++ {
			if items[right].Name < items[left].Name {
				items[left], items[right] = items[right], items[left]
			}
		}
	}
}
