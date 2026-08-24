package discovery

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type fixtureRunner map[string]string

func (runner fixtureRunner) Run(_ context.Context, name string, arguments ...string) ([]byte, error) {
	key := name + " " + strings.Join(arguments, " ")
	value, ok := runner[key]
	if !ok {
		return nil, fmt.Errorf("missing fixture %s", key)
	}
	return []byte(value), nil
}

type fixtureFiles map[string]string

func (files fixtureFiles) ReadFile(path string) ([]byte, error) {
	value, ok := files[path]
	if !ok {
		return nil, fmt.Errorf("missing fixture %s", path)
	}
	return []byte(value), nil
}

func TestParseLeasesAndNeighbors(t *testing.T) {
	leases := parseLeases([]byte("1710000000 aa:bb:cc:dd:ee:ff 192.168.8.20 nas *\n"))
	if leases["aa:bb:cc:dd:ee:ff"].Name != "nas" {
		t.Fatalf("unexpected leases: %#v", leases)
	}
	neighbors := parseNeighbors([]byte("192.168.8.20 dev br-lan lladdr aa:bb:cc:dd:ee:ff REACHABLE\n192.168.8.30 dev br-lan FAILED\n"))
	if len(neighbors) != 1 || neighbors[0].IPv4 != "192.168.8.20" {
		t.Fatalf("unexpected neighbors: %#v", neighbors)
	}
}

func TestValidateObservationRejectsIPv6(t *testing.T) {
	err := ValidateObservation(Observation{Domain: "lan", MAC: "aa:bb:cc:dd:ee:ff", IPv4: "fe80::1"})
	if err == nil {
		t.Fatal("expected IPv6 to be rejected in V1")
	}
}

func TestWirelessClientsStayInTheirConfiguredNetworkDomain(t *testing.T) {
	raw := []byte(`{
      "radio0": {"interfaces": [
        {"ifname":"wlan0","config":{"network":["lan"]}},
        {"ifname":"wlan0-1","config":{"network":["guest"]}}
      ]}
    }`)
	domains := parseWirelessDomains(raw)
	if len(domains["lan"]) != 1 || domains["lan"][0] != "wlan0" {
		t.Fatalf("unexpected LAN mapping: %#v", domains)
	}
	if len(domains["guest"]) != 1 || domains["guest"][0] != "wlan0-1" {
		t.Fatalf("unexpected guest mapping: %#v", domains)
	}
	clients := parseHostapdClients([]byte(`{"clients":{"aa:bb:cc:dd:ee:ff":{"auth":true},"invalid":{}}}`))
	if len(clients) != 1 || clients[0] != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("unexpected clients: %#v", clients)
	}
}

func TestSystemSourceUsesOnlyAllowedLogicalInterfaces(t *testing.T) {
	runner := fixtureRunner{
		"ubus call network.wireless status":      `{"radio0":{"interfaces":[{"ifname":"wlan0","config":{"network":["lan"]}},{"ifname":"wlan1","config":{"network":["guest"]}}]}}`,
		"ubus call network.interface.lan status": `{"up":true,"l3_device":"br-lan","ipv4-address":[{"address":"192.168.8.1"}]}`,
		"ip -4 neigh show dev br-lan":            "192.168.8.20 dev br-lan lladdr aa:bb:cc:dd:ee:20 REACHABLE\n",
		"ubus call hostapd.wlan0 get_clients":    `{"clients":{"aa:bb:cc:dd:ee:30":{"auth":true}}}`,
	}
	files := fixtureFiles{
		"/tmp/dhcp.leases":              "1 aa:bb:cc:dd:ee:20 192.168.8.20 nas *\n1 aa:bb:cc:dd:ee:30 192.168.8.30 phone *\n",
		"/sys/class/net/br-lan/address": "aa:bb:cc:dd:ee:01\n",
	}
	source := NewSystemSource(runner, []string{"lan"})
	source.Files = files
	observations, err := source.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 3 {
		t.Fatalf("expected router, wired neighbor, and LAN Wi-Fi client: %#v", observations)
	}
	for _, observation := range observations {
		if observation.Domain != "lan" {
			t.Fatalf("unexpected non-LAN observation: %#v", observation)
		}
		if observation.IPv4 == "192.168.8.30" && observation.Name != "phone" {
			t.Fatalf("hostapd client did not inherit DHCP identity: %#v", observation)
		}
	}
}
