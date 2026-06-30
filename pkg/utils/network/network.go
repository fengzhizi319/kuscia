// Copyright 2023 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package network

import (
	"errors"
	"net"
)

// GetHostIP gets IPv4 address of network interface eth0.
func GetHostIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	var iface *net.Interface
	for i := range ifaces {
		// eth0 for linux and en0 for mac
		if ifaces[i].Name == "eth0" || ifaces[i].Name == "en0" {
			iface = &ifaces[i]
			break
		}
	}

	if iface != nil {
		if addr, err := getIPv4Addr(iface); err == nil {
			return addr, nil
		}
	}

	// Fallback: use the first non-loopback interface that has an IPv4 address.
	// This makes local tests / WSL / container environments with non-standard
	// interface names (e.g. eth2) work without requiring explicit host IP config.
	for i := range ifaces {
		if ifaces[i].Flags&net.FlagUp == 0 || ifaces[i].Flags&net.FlagLoopback != 0 {
			continue
		}
		if addr, err := getIPv4Addr(&ifaces[i]); err == nil {
			return addr, nil
		}
	}

	return "", errors.New("host IP unknown")
}

func getIPv4Addr(iface *net.Interface) (string, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}

	var address string
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		if ip == nil {
			continue
		}

		address = ip.String()
		if ip.To4() != nil {
			return address, nil
		}
	}

	if address == "" {
		return address, errors.New("host IP unknown")
	}

	return address, nil
}
