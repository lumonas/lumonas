package network

import (
	"net"
	"sort"
)

type Interface struct {
	Name      string   `json:"name"`
	MAC       string   `json:"mac,omitempty"`
	MTU       int      `json:"mtu"`
	Up        bool     `json:"up"`
	Loopback  bool     `json:"loopback"`
	Addresses []string `json:"addresses"`
}

func Interfaces() ([]Interface, error) {
	items, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]Interface, 0, len(items))
	for _, item := range items {
		addresses, _ := item.Addrs()
		values := make([]string, 0, len(addresses))
		for _, address := range addresses {
			values = append(values, address.String())
		}
		sort.Strings(values)
		result = append(result, Interface{Name: item.Name, MAC: item.HardwareAddr.String(), MTU: item.MTU, Up: item.Flags&net.FlagUp != 0, Loopback: item.Flags&net.FlagLoopback != 0, Addresses: values})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Name < result[right].Name })
	return result, nil
}
