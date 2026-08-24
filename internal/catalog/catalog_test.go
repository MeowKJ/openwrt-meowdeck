package catalog

import "testing"

func TestDeviceIDIncludesNetworkDomain(t *testing.T) {
	a, err := DeviceID("lan", "AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatal(err)
	}
	b, err := DeviceID("guest", "aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("network domain must participate in device identity")
	}
}

func TestServiceIDIncludesPath(t *testing.T) {
	a := ServiceID("router", "http", 80, "/")
	b := ServiceID("router", "http", 80, "/cgi-bin/luci/")
	if a == b {
		t.Fatal("service path must participate in service identity")
	}
}
