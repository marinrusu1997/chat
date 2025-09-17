package scylla

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/yl2chen/cidranger"
)

type internalAddressType int

const (
	internalAddressTypeIP internalAddressType = iota
	internalAddressTypeHost
	internalAddressTypeCIDR
)
const optionalPort = 0

type parsedInternalAddress struct {
	addressType internalAddressType
	main        string // IP string, Hostname, or CIDR string
	port        uint16 // optional, 0 if not provided
}

type parsedExternalAddress struct {
	ip   net.IP
	port uint16
}

func parsePort(portStr string) (uint16, error) {
	port, err := strconv.ParseUint(portStr, 10, 16) // Parse as unsigned 16-bit integer
	if err != nil {
		return 0, fmt.Errorf("invalid port %s: %v", portStr, err)
	}
	// Ports 0-1023 are well-known/privileged, but still technically valid.
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port out of range: %d", port)
	}

	return uint16(port), nil
}

func parseInternalAddress(address string) (parsedInternalAddress, error) {
	port := uint16(optionalPort)
	main := address

	// Split optional port
	if strings.Contains(address, ":") {
		parts := strings.Split(address, ":")
		main = parts[0]
		p, err := parsePort(parts[1])
		if err != nil {
			return parsedInternalAddress{}, fmt.Errorf("invalid port in internal address %s: %v", address, err)
		}
		port = p
	}

	if _, _, err := net.ParseCIDR(main); err == nil {
		return parsedInternalAddress{addressType: internalAddressTypeCIDR, main: main, port: port}, nil
	}
	if ip := net.ParseIP(main); ip != nil {
		return parsedInternalAddress{addressType: internalAddressTypeIP, main: main, port: port}, nil
	}
	return parsedInternalAddress{addressType: internalAddressTypeHost, main: main, port: port}, nil
}

func parseExternalAddress(address string) (parsedExternalAddress, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return parsedExternalAddress{}, fmt.Errorf("failed to split external address '%s': %v", address, err)
	}

	port, err := parsePort(portStr)
	if err != nil {
		return parsedExternalAddress{}, fmt.Errorf("failed to parse port from external address '%s': %v", address, err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		log.Printf("failed to parse IP from host '%s' of external address '%s'", host, address)
	}

	return parsedExternalAddress{ip: ip, port: port}, nil
}

type networkEndpoint struct {
	host string // hostname or CIDR
	ip   net.IP // nil if host is a hostname or CIDR
	port uint16
}

type addressMapping struct {
	internal networkEndpoint
	external networkEndpoint
}

type cidrRangerHostEntry struct {
	addressMapping
	network net.IPNet
}

func (e *cidrRangerHostEntry) Network() net.IPNet {
	return e.network
}

type StaticAddressTranslator struct {
	hostMapping map[string]addressMapping
	cidrRanger  cidranger.Ranger
}

func NewStaticAddressTranslator(translations map[string]string) *StaticAddressTranslator {
	translator := &StaticAddressTranslator{
		hostMapping: map[string]addressMapping{},
		cidrRanger:  cidranger.NewPCTrieRanger(),
	}

	seenCIDRs := map[string]bool{}

	for internal, external := range translations {
		internalAddress, err := parseInternalAddress(internal)
		if err != nil {
			log.Printf("Failed to parse internal translation address '%s': %v", internal, err)
			continue
		}
		externalAddress, err := parseExternalAddress(external)
		if err != nil {
			log.Printf("Failed to parse external translation address '%s' associated with internal address '%s': %v",
				external, internal, err)
			continue
		}

		switch internalAddress.addressType {
		case internalAddressTypeIP:
			fallthrough
		case internalAddressTypeHost:
			var ip net.IP
			if internalAddress.addressType == internalAddressTypeIP {
				ip = net.ParseIP(internalAddress.main)
			}

			lookupKey := fmt.Sprintf("%s:%d", internalAddress.main, internalAddress.port)
			if _, exists := translator.hostMapping[lookupKey]; exists {
				log.Printf("Skipping duplicate lookup key '%s' associated with mapping: '%s' -> '%s'", lookupKey, internal, external)
				continue
			}

			translator.hostMapping[lookupKey] = addressMapping{
				internal: networkEndpoint{
					host: internalAddress.main,
					ip:   ip,
					port: internalAddress.port,
				},
				external: networkEndpoint{
					host: externalAddress.ip.String(),
					ip:   externalAddress.ip,
					port: externalAddress.port,
				},
			}
			break

		case internalAddressTypeCIDR:
			if _, seen := seenCIDRs[internalAddress.main]; seen {
				log.Printf("Skipping duplicate CIDR entry '%s' associated with mapping: '%s' -> '%s'", internalAddress.main, internal, external)
				continue
			}
			seenCIDRs[internalAddress.main] = true

			ip, network, _ := net.ParseCIDR(internalAddress.main)

			err = translator.cidrRanger.Insert(&cidrRangerHostEntry{
				network: *network,
				addressMapping: addressMapping{
					internal: networkEndpoint{
						host: internalAddress.main,
						ip:   ip,
						port: internalAddress.port,
					},
					external: networkEndpoint{
						host: externalAddress.ip.String(),
						ip:   externalAddress.ip,
						port: externalAddress.port,
					},
				},
			})
			if err != nil {
				log.Printf("Failed to insert CIDR entry for CIDR '%s': %v", internalAddress.main, err)
				continue
			}

			break
		}
	}

	return translator
}

func (s *StaticAddressTranslator) Translate(originalIP net.IP, originalPort int) (net.IP, int) {
	// Attempt to find a direct match
	hostnames := []string{originalIP.String()}
	addresses, err := net.LookupAddr(hostnames[0])
	if err == nil && len(addresses) > 0 {
		for _, n := range addresses {
			hostnames = append(hostnames, strings.Trim(n, "."))
		}
	}

	lookupPorts := []int{originalPort, optionalPort}
	for _, hostname := range hostnames {
		for _, lookupPort := range lookupPorts {
			lookupKey := fmt.Sprintf("%s:%d", hostname, lookupPort)
			if translation, ok := s.hostMapping[lookupKey]; ok {
				return translation.external.ip, int(translation.external.port)
			}
		}
	}

	// Attempt to find a CIDR match
	entries, err := s.cidrRanger.ContainingNetworks(originalIP)
	if err == nil {
		for _, entry := range entries {
			translation := entry.(*cidrRangerHostEntry)
			if translation.internal.port == optionalPort || translation.internal.port == uint16(originalPort) {
				return translation.external.ip, int(translation.external.port)
			}
		}
	} else {
		log.Printf("CIDR ranger lookup error for IP '%s': %v", originalIP, err)
	}

	// No translation found, return original
	return originalIP, originalPort
}
