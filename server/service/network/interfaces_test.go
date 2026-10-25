package network

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"NanoKVM-Server/proto"
)

func TestValidateIfaceStatic(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		gw      string
		wantErr bool
	}{
		{"plain", "192.168.1.10/24", "192.168.1.1", false},
		{"no gateway", "192.168.1.10/24", "", false},
		{"link-local direct cable", "169.254.50.1/16", "", false},
		{"slash 30", "10.0.0.1/30", "10.0.0.2", false},
		{"slash 1", "10.1.2.3/1", "", false},
		{"no prefix", "192.168.1.10", "", true},
		{"prefix 0", "192.168.1.10/0", "", true},
		{"prefix 31", "192.168.1.10/31", "", true},
		{"prefix 32", "192.168.1.10/32", "", true},
		{"prefix 33", "192.168.1.10/33", "", true},
		{"ipv6", "fd00::1/64", "", true},
		{"garbage", "hello/24", "", true},
		{"empty", "", "", true},
		{"network address", "192.168.1.0/24", "", true},
		{"broadcast address", "192.168.1.255/24", "", true},
		{"loopback", "127.0.0.1/8", "", true},
		{"unspecified", "0.0.0.0/8", "", true},
		{"multicast", "224.0.0.5/24", "", true},
		{"reserved", "240.0.0.5/24", "", true},
		{"gateway outside", "192.168.1.10/24", "192.168.2.1", true},
		{"gateway equals address", "192.168.1.10/24", "192.168.1.10", true},
		{"gateway network address", "192.168.1.10/24", "192.168.1.0", true},
		{"gateway broadcast", "192.168.1.10/24", "192.168.1.255", true},
		{"gateway garbage", "192.168.1.10/24", "gw", true},
		{"gateway ipv6", "192.168.1.10/24", "fd00::1", true},
		{"shell metacharacters", "1.2.3.4/24; reboot", "", true},
	}
	for _, tc := range cases {
		_, err := validateIfaceStatic(tc.addr, tc.gw)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err=%v, wantErr=%t", tc.name, err, tc.wantErr)
		}
	}
}

func TestRenderIfaceConfig(t *testing.T) {
	cases := []struct {
		req     proto.SetInterfaceReq
		want    string
		remove  bool
		wantErr bool
	}{
		{proto.SetInterfaceReq{Mode: "static", Address: "192.168.1.10/24", Gateway: "192.168.1.1"}, "192.168.1.10/24 192.168.1.1\n", false, false},
		{proto.SetInterfaceReq{Mode: "static", Address: "192.168.1.10/24"}, "192.168.1.10/24\n", false, false},
		{proto.SetInterfaceReq{Mode: "dhcp"}, "", true, false},
		{proto.SetInterfaceReq{Mode: "dhcp", Address: "ignored"}, "", true, false},
		{proto.SetInterfaceReq{Mode: "bridge"}, "", false, true},
		{proto.SetInterfaceReq{}, "", false, true},
	}
	for _, tc := range cases {
		got, err := renderIfaceConfig(tc.req)
		if (err != nil) != tc.wantErr {
			t.Errorf("%+v: err=%v", tc.req, err)
			continue
		}
		if tc.wantErr {
			continue
		}
		if tc.remove != (got == nil) || string(got) != tc.want {
			t.Errorf("%+v: got %q (nil=%t)", tc.req, got, got == nil)
		}
	}
}

// testSpec points one interface at a temporary directory and stubs the script.
func testSpec(t *testing.T, name, kind string) (ifaceSpec, *int) {
	t.Helper()
	dir := t.TempDir()
	spec := ifaceSpec{
		Name:       name,
		Kind:       kind,
		ConfigFile: filepath.Join(dir, "boot", name+".nodhcp"),
		PrevFile:   filepath.Join(dir, "kvm", name+".nodhcp.prev"),
	}
	if err := os.MkdirAll(filepath.Dir(spec.ConfigFile), 0o755); err != nil {
		t.Fatal(err)
	}

	runs := 0
	oldApply := applyIfaceScript
	applyIfaceScript = func(ifaceSpec) error { runs++; return nil }
	oldResolv := ifaceResolv
	ifaceResolv = filepath.Join(dir, "resolv.conf")

	t.Cleanup(func() {
		applyIfaceScript, ifaceResolv = oldApply, oldResolv
		st := stateFor(name)
		st.mu.Lock()
		if st.timer != nil {
			st.timer.Stop()
		}
		st.timer, st.pending = nil, nil
		st.mu.Unlock()
	})
	return spec, &runs
}

func TestIfaceChangeRevertRoundTrip(t *testing.T) {
	for _, kind := range []struct{ name, kind string }{{"eth0", ifaceKindEthernet}, {"wlan0", ifaceKindWifi}} {
		spec, _ := testSpec(t, kind.name, kind.kind)

		// no file: DHCP. Switch to static.
		req := proto.SetInterfaceReq{Mode: "static", Address: "10.0.0.5/24", Gateway: "10.0.0.1"}
		data, err := renderIfaceConfig(req)
		if err != nil {
			t.Fatal(err)
		}
		pending, _, err := startIfaceChange(spec, req, data)
		if err != nil {
			t.Fatalf("%s: %v", kind.name, err)
		}
		if pending.DeadlineUnix == 0 || pending.Address != "10.0.0.5/24" {
			t.Errorf("%s: pending %+v", kind.name, pending)
		}
		got, _ := os.ReadFile(spec.ConfigFile)
		if string(got) != "10.0.0.5/24 10.0.0.1\n" {
			t.Errorf("%s: file %q", kind.name, got)
		}
		info, err := os.Stat(spec.PrevFile)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: prev file %v %v", kind.name, info, err)
		}
		if mode, addr, gw := readIfaceConfig(spec); mode != "static" || addr != "10.0.0.5/24" || gw != "10.0.0.1" {
			t.Errorf("%s: read back %s %s %s", kind.name, mode, addr, gw)
		}

		// a second change while pending is refused
		if _, _, err := startIfaceChange(spec, req, data); err == nil {
			t.Errorf("%s: second change accepted while pending", kind.name)
		}

		// dead-man: revert restores "absent"
		revertIface(spec)
		if _, err := os.Stat(spec.ConfigFile); !os.IsNotExist(err) {
			t.Errorf("%s: config file survived the revert: %v", kind.name, err)
		}
		if _, err := os.Stat(spec.PrevFile); !os.IsNotExist(err) {
			t.Errorf("%s: prev file survived the revert", kind.name)
		}

		// existing static file, change to DHCP, then confirm
		if err := os.WriteFile(spec.ConfigFile, []byte("192.168.9.9/24\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		dhcp := proto.SetInterfaceReq{Mode: "dhcp"}
		if _, _, err := startIfaceChange(spec, dhcp, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(spec.ConfigFile); !os.IsNotExist(err) {
			t.Errorf("%s: dhcp left the file", kind.name)
		}
		local := netip.MustParseAddr("192.168.9.10")
		if err := confirmIfaceChange(spec, local); err == nil {
			t.Errorf("%s: confirm before the apply succeeded", kind.name)
		}
		applyIface(spec, "apply", "")
		oldAssigned := ifaceLocalAddrAssigned
		ifaceLocalAddrAssigned = func(a netip.Addr) bool { return false }
		if err := confirmIfaceChange(spec, local); err == nil {
			t.Errorf("%s: confirm on an unassigned address succeeded", kind.name)
		}
		ifaceLocalAddrAssigned = func(a netip.Addr) bool { return a == local }
		if err := confirmIfaceChange(spec, local); err != nil {
			t.Fatal(err)
		}
		ifaceLocalAddrAssigned = oldAssigned
		if _, err := os.Stat(spec.PrevFile); !os.IsNotExist(err) {
			t.Errorf("%s: confirm left the prev file", kind.name)
		}
		if err := confirmIfaceChange(spec, local); err == nil {
			t.Errorf("%s: confirm without a pending change succeeded", kind.name)
		}
	}
}

func TestIfaceCrashRecovery(t *testing.T) {
	spec, runs := testSpec(t, "eth0", ifaceKindEthernet)

	if err := os.WriteFile(spec.ConfigFile, []byte("192.168.1.10/24 192.168.1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := proto.SetInterfaceReq{Mode: "static", Address: "10.0.0.5/24"}
	data, _ := renderIfaceConfig(req)
	if _, _, err := startIfaceChange(spec, req, data); err != nil {
		t.Fatal(err)
	}

	// the server dies here: the pending state is lost, the files remain
	st := stateFor(spec.Name)
	st.mu.Lock()
	st.timer.Stop()
	st.timer, st.pending = nil, nil
	st.mu.Unlock()

	// the restart is asynchronous in recoverIface; the file restore is not
	recoverIface(spec)
	got, _ := os.ReadFile(spec.ConfigFile)
	if string(got) != "192.168.1.10/24 192.168.1.1\n" {
		t.Errorf("not restored: %q", got)
	}
	if _, err := os.Stat(spec.PrevFile); !os.IsNotExist(err) {
		t.Errorf("prev file survived recovery")
	}
	_ = runs
}

func TestIfacePresence(t *testing.T) {
	oldPresent, oldAP := ifacePresent, ifaceAPMode
	t.Cleanup(func() { ifacePresent, ifaceAPMode = oldPresent, oldAP })

	eth, _ := testSpec(t, "eth0", ifaceKindEthernet)
	wlan, _ := testSpec(t, "wlan0", ifaceKindWifi)

	// not present: described as such and refused
	ifacePresent = func(ifaceSpec) bool { return false }
	ifaceAPMode = func() bool { return false }
	info := describeIface(wlan)
	if info.Present || info.Name != "wlan0" || info.Kind != ifaceKindWifi || info.Pending != nil {
		t.Errorf("absent iface described as %+v", info)
	}
	if err := checkIfaceApplicable(wlan); err == nil {
		t.Error("POST accepted for a not-present interface")
	}
	if err := checkIfaceApplicable(eth); err == nil {
		t.Error("POST accepted for a not-present ethernet")
	}

	// present, station mode: allowed
	ifacePresent = func(ifaceSpec) bool { return true }
	if err := checkIfaceApplicable(wlan); err != nil {
		t.Errorf("station mode refused: %v", err)
	}

	// wlan0 in AP mode: refused, eth0 unaffected
	ifaceAPMode = func() bool { return true }
	if err := checkIfaceApplicable(wlan); err == nil {
		t.Error("wlan0 change accepted in AP mode")
	}
	if err := checkIfaceApplicable(eth); err != nil {
		t.Errorf("eth0 affected by AP mode: %v", err)
	}
}

func TestIfaceSpecsAreFixed(t *testing.T) {
	for _, name := range []string{"eth0", "wlan0"} {
		if _, ok := findIfaceSpec(name); !ok {
			t.Errorf("%s missing", name)
		}
	}
	for _, name := range []string{"", "eth1", "../etc/passwd", "lo", "wlan0 "} {
		if _, ok := findIfaceSpec(name); ok {
			t.Errorf("%q accepted", name)
		}
	}
}

func TestGatewayOf(t *testing.T) {
	if gatewayOf("1.2.3.4/24 1.2.3.1\n") != "1.2.3.1" || gatewayOf("1.2.3.4/24\n") != "" || gatewayOf("") != "" {
		t.Error("gatewayOf")
	}
}
