package translator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/yl2chen/cidranger"
)

type internalAddressType int

const (
	internalAddressTypeIP internalAddressType = iota
	internalAddressTypeHost
	internalAddressTypeCIDR
)
const optionalPort uint16 = 0

var ErrPortOutOfRange = errors.New("port out of range")

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
		return 0, fmt.Errorf("invalid port %s: %w", portStr, err)
	}
	// Ports 0-1023 are well-known/privileged, but still technically valid.
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %d: %w", port, ErrPortOutOfRange)
	}

	return uint16(port), nil
}

func parseInternalAddress(address string) (parsedInternalAddress, error) {
	port := optionalPort
	main := address

	// Split optional port
	if strings.Contains(address, ":") {
		parts := strings.Split(address, ":")
		main = parts[0]
		p, err := parsePort(parts[1])
		if err != nil {
			return parsedInternalAddress{}, fmt.Errorf("invalid port in internal address %s: %w", address, err)
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

func parseExternalAddress(address string, logger *zerolog.Logger) (parsedExternalAddress, error) {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return parsedExternalAddress{}, fmt.Errorf("failed to split external address '%s': %w", address, err)
	}

	port, err := parsePort(portStr)
	if err != nil {
		return parsedExternalAddress{}, fmt.Errorf("failed to parse port from external address '%s': %w", address, err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		logger.Warn().Msgf("failed to parse IP from host '%s' of external address '%s'", host, address)
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
	logger      zerolog.Logger
}

func NewStaticAddressTranslator(translations map[string]string, logger *zerolog.Logger) *StaticAddressTranslator {
	translator := &StaticAddressTranslator{
		hostMapping: make(map[string]addressMapping),
		cidrRanger:  cidranger.NewPCTrieRanger(),
		logger:      *logger,
	}

	seenCIDRs := make(map[string]bool)

	for internal, external := range translations {
		internalAddress, err := parseInternalAddress(internal)
		if err != nil {
			translator.logger.Warn().Msgf("Failed to parse internal translation address '%s': %v", internal, err)
			continue
		}
		externalAddress, err := parseExternalAddress(external, &translator.logger)
		if err != nil {
			translator.logger.Warn().Msgf("Failed to parse external translation address '%s' associated with internal address '%s': %v",
				external, internal, err)
			continue
		}

		switch internalAddress.addressType {
		case internalAddressTypeIP, internalAddressTypeHost:
			var ip net.IP
			if internalAddress.addressType == internalAddressTypeIP {
				ip = net.ParseIP(internalAddress.main)
			}

			lookupKey := fmt.Sprintf("%s:%d", internalAddress.main, internalAddress.port)
			if _, exists := translator.hostMapping[lookupKey]; exists {
				translator.logger.Debug().Msgf("Skipping duplicate lookup key '%s' associated with mapping: '%s' -> '%s'", lookupKey, internal, external)
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

		case internalAddressTypeCIDR:
			if _, seen := seenCIDRs[internalAddress.main]; seen {
				translator.logger.Debug().Msgf("Skipping duplicate CIDR entry '%s' associated with mapping: '%s' -> '%s'", internalAddress.main, internal, external)
				continue
			}
			seenCIDRs[internalAddress.main] = true

			ip, network, err := net.ParseCIDR(internalAddress.main)
			if err != nil {
				translator.logger.Warn().Msgf("Failed to parse CIDR '%s' for internal address '%s': %v", internalAddress.main, internal, err)
				continue
			}

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
				translator.logger.Warn().Msgf("Failed to insert CIDR entry for CIDR '%s': %v", internalAddress.main, err)
				continue
			}

		default:
			translator.logger.Warn().Msgf("Unknown internal address type for address '%s'", internal)
		}
	}

	return translator
}

func (s *StaticAddressTranslator) Translate(originalIP net.IP, originalPort uint16) (translatedIP net.IP, translatedPort uint16) {
	// Attempt to find a direct match
	hostnames := []string{originalIP.String()}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var r net.Resolver
	addresses, err := r.LookupAddr(ctx, hostnames[0])
	if err == nil && len(addresses) > 0 {
		for _, n := range addresses {
			hostnames = append(hostnames, strings.Trim(n, "."))
		}
	}

	lookupPorts := []uint16{originalPort, optionalPort}
	for _, hostname := range hostnames {
		for _, lookupPort := range lookupPorts {
			lookupKey := fmt.Sprintf("%s:%d", hostname, lookupPort)
			if translation, ok := s.hostMapping[lookupKey]; ok {
				return translation.external.ip, translation.external.port
			}
		}
	}

	// Attempt to find a CIDR match
	entries, err := s.cidrRanger.ContainingNetworks(originalIP)
	if err == nil {
		for _, entry := range entries {
			translation, ok := entry.(*cidrRangerHostEntry)
			if !ok {
				s.logger.Warn().Msgf("entry type assertion to 'cidrRangerHostEntry' failed for IP '%s'", originalIP)
				continue
			}

			if translation.internal.port == optionalPort || translation.internal.port == originalPort {
				return translation.external.ip, translation.external.port
			}
		}
	} else {
		s.logger.Warn().Msgf("CIDR ranger lookup error for IP '%s': %v", originalIP, err)
	}

	// No translation found, return original
	return originalIP, originalPort
}
