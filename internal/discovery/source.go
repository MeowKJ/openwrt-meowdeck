package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type FileReader interface {
	ReadFile(string) ([]byte, error)
}

type Observation struct {
	Domain    string
	Interface string
	MAC       string
	IPv4      string
	Name      string
	Router    bool
}

type DeviceSource interface {
	Discover(context.Context) ([]Observation, error)
}

type SystemSource struct {
	Runner     CommandRunner
	Files      FileReader
	Domains    []string
	LeasesPath string
}

type osFiles struct{}

func (osFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, arguments...).Output()
}

func NewSystemSource(runner CommandRunner, domains []string) *SystemSource {
	if len(domains) == 0 {
		domains = []string{"lan"}
	}
	return &SystemSource{Runner: runner, Files: osFiles{}, Domains: domains, LeasesPath: "/tmp/dhcp.leases"}
}

type interfaceStatus struct {
	Up       bool   `json:"up"`
	Device   string `json:"device"`
	L3Device string `json:"l3_device"`
	IPv4     []struct {
		Address string `json:"address"`
	} `json:"ipv4-address"`
}

func (s *SystemSource) Discover(ctx context.Context) ([]Observation, error) {
	if s.Runner == nil {
		return nil, fmt.Errorf("command runner is required")
	}
	leases := map[string]Observation{}
	if raw, err := s.Files.ReadFile(s.LeasesPath); err == nil {
		leases = parseLeases(raw)
	}
	wirelessDomains := map[string][]string{}
	if raw, err := s.Runner.Run(ctx, "ubus", "call", "network.wireless", "status"); err == nil {
		wirelessDomains = parseWirelessDomains(raw)
	}
	seen := map[string]Observation{}
	for _, domain := range s.Domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		raw, err := s.Runner.Run(ctx, "ubus", "call", "network.interface."+domain, "status")
		if err != nil {
			return nil, fmt.Errorf("read interface %s: %w", domain, err)
		}
		var status interfaceStatus
		if err := json.Unmarshal(raw, &status); err != nil {
			return nil, fmt.Errorf("decode interface %s: %w", domain, err)
		}
		if !status.Up {
			continue
		}
		device := status.L3Device
		if device == "" {
			device = status.Device
		}
		if device == "" {
			continue
		}
		if len(status.IPv4) > 0 && net.ParseIP(status.IPv4[0].Address).To4() != nil {
			if macRaw, err := s.Files.ReadFile(filepath.Join("/sys/class/net", device, "address")); err == nil {
				mac := strings.TrimSpace(string(macRaw))
				if _, err := net.ParseMAC(mac); err == nil {
					observation := Observation{Domain: domain, Interface: device, MAC: mac, IPv4: status.IPv4[0].Address, Name: "OpenWrt", Router: true}
					seen[domain+"\x00"+strings.ToLower(mac)] = observation
				}
			}
		}
		neighbors, err := s.Runner.Run(ctx, "ip", "-4", "neigh", "show", "dev", device)
		if err != nil {
			continue
		}
		for _, neighbor := range parseNeighbors(neighbors) {
			if lease, ok := leases[strings.ToLower(neighbor.MAC)]; ok {
				if lease.IPv4 == neighbor.IPv4 || lease.IPv4 == "" {
					neighbor.Name = lease.Name
				}
			}
			neighbor.Domain = domain
			neighbor.Interface = device
			seen[domain+"\x00"+strings.ToLower(neighbor.MAC)] = neighbor
		}
		for _, ifname := range wirelessDomains[domain] {
			clientsRaw, err := s.Runner.Run(ctx, "ubus", "call", "hostapd."+ifname, "get_clients")
			if err != nil {
				continue
			}
			for _, mac := range parseHostapdClients(clientsRaw) {
				lease, ok := leases[strings.ToLower(mac)]
				if !ok || net.ParseIP(lease.IPv4).To4() == nil {
					continue
				}
				observation := Observation{
					Domain: domain, Interface: device, MAC: mac, IPv4: lease.IPv4, Name: lease.Name,
				}
				seen[domain+"\x00"+strings.ToLower(mac)] = observation
			}
		}
	}
	result := make([]Observation, 0, len(seen))
	for _, observation := range seen {
		result = append(result, observation)
	}
	return result, nil
}

func parseWirelessDomains(raw []byte) map[string][]string {
	var radios map[string]struct {
		Interfaces []struct {
			Ifname string `json:"ifname"`
			Config struct {
				Network []string `json:"network"`
			} `json:"config"`
		} `json:"interfaces"`
	}
	result := map[string][]string{}
	if json.Unmarshal(raw, &radios) != nil {
		return result
	}
	for _, radio := range radios {
		for _, wireless := range radio.Interfaces {
			if wireless.Ifname == "" {
				continue
			}
			for _, domain := range wireless.Config.Network {
				result[domain] = append(result[domain], wireless.Ifname)
			}
		}
	}
	return result
}

func parseHostapdClients(raw []byte) []string {
	var response struct {
		Clients map[string]json.RawMessage `json:"clients"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return nil
	}
	result := make([]string, 0, len(response.Clients))
	for mac := range response.Clients {
		if _, err := net.ParseMAC(mac); err == nil {
			result = append(result, mac)
		}
	}
	return result
}

func parseLeases(raw []byte) map[string]Observation {
	result := map[string]Observation{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || net.ParseIP(fields[2]).To4() == nil {
			continue
		}
		if _, err := net.ParseMAC(fields[1]); err != nil {
			continue
		}
		name := fields[3]
		if name == "*" {
			name = ""
		}
		result[strings.ToLower(fields[1])] = Observation{MAC: fields[1], IPv4: fields[2], Name: name}
	}
	return result
}

func parseNeighbors(raw []byte) []Observation {
	result := make([]Observation, 0)
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || net.ParseIP(fields[0]).To4() == nil {
			continue
		}
		state := strings.ToUpper(fields[len(fields)-1])
		if state == "FAILED" || state == "INCOMPLETE" {
			continue
		}
		macIndex := -1
		for index, field := range fields {
			if field == "lladdr" && index+1 < len(fields) {
				macIndex = index + 1
				break
			}
		}
		if macIndex < 0 {
			continue
		}
		if _, err := net.ParseMAC(fields[macIndex]); err != nil {
			continue
		}
		result = append(result, Observation{MAC: fields[macIndex], IPv4: fields[0]})
	}
	return result
}
