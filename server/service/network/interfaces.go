package network

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"NanoKVM-Server/proto"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

const (
	ifaceModeDHCP   = "dhcp"
	ifaceModeStatic = "static"

	ifaceKindEthernet = "ethernet"
	ifaceKindWifi     = "wifi"

	// ifaceConfirmWindow is how long a change stays revertable before the old
	// settings come back on their own.
	ifaceConfirmWindow = 120 * time.Second
	// ifaceRestartTimeout bounds one apply (arping and udhcpc are slow).
	ifaceRestartTimeout = 90 * time.Second
)

// ifaceSpec describes one configurable interface. The set is fixed: a request
// names an interface, and only these names are ever used for a path or argv.
type ifaceSpec struct {
	Name       string
	Kind       string
	ConfigFile string   // one-line "addr/nn [gw]" file read by the init script
	PrevFile   string   // snapshot of ConfigFile while a change is unconfirmed
	Script     string   // init script that applies the configuration
	Args       []string // its arguments
}

// the specs and the hooks below are variables so tests can redirect them
var (
	ifaceSpecs = []ifaceSpec{
		{
			Name:       "eth0",
			Kind:       ifaceKindEthernet,
			ConfigFile: "/boot/eth.nodhcp",
			PrevFile:   "/etc/kvm/eth.nodhcp.prev",
			Script:     "/etc/init.d/S30eth",
			Args:       []string{"restart"},
		},
		{
			Name:       "wlan0",
			Kind:       ifaceKindWifi,
			ConfigFile: "/boot/wlan.nodhcp",
			PrevFile:   "/etc/kvm/wlan.nodhcp.prev",
			// "ip" re-applies the address only; "restart" would also bounce
			// wpa_supplicant and drop the Wi-Fi join
			Script: "/etc/init.d/S30wifi",
			Args:   []string{"ip"},
		},
	}

	ifaceRouteFile = "/proc/net/route"
	ifaceResolv    = etcResolvFile

	// ifacePresent decides whether the hardware is there: the interface exists
	// and, for Wi-Fi, the same /etc/kvm/wifi_exist test the Wi-Fi panel uses.
	ifacePresent = func(spec ifaceSpec) bool {
		if _, err := net.InterfaceByName(spec.Name); err != nil {
			return false
		}
		if spec.Kind == ifaceKindWifi {
			return isSupported()
		}
		return true
	}
	ifaceAPMode = isAPMode
	// applyIfaceScript runs the init script; tests replace it.
	applyIfaceScript = runIfaceScript

	// ifaceLocalAddrAssigned reports whether a is assigned to an interface now:
	// a confirm that arrives on such an address reached the box after the change.
	ifaceLocalAddrAssigned = func(a netip.Addr) bool {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			return false
		}
		for _, addr := range addrs {
			if p, err := netip.ParsePrefix(addr.String()); err == nil && p.Addr().Unmap() == a {
				return true
			}
		}
		return false
	}
)

// ifaceSnapshot is the previous config file, kept on disk so the revert
// survives a crash or a reboot while a change is pending.
type ifaceSnapshot struct {
	Absent  bool   `json:"absent"`
	Content string `json:"content"`
}

type ifaceState struct {
	mu      sync.Mutex
	timer   *time.Timer
	pending *proto.InterfacePending
	// applied: the init script has run for the pending change, so a confirm
	// can prove the new settings work; before that it proves nothing
	applied bool
}

var (
	ifaceStatesMu sync.Mutex
	ifaceStates   = map[string]*ifaceState{}
)

func stateFor(name string) *ifaceState {
	ifaceStatesMu.Lock()
	defer ifaceStatesMu.Unlock()
	st, ok := ifaceStates[name]
	if !ok {
		st = &ifaceState{}
		ifaceStates[name] = st
	}
	return st
}

func findIfaceSpec(name string) (ifaceSpec, bool) {
	for _, spec := range ifaceSpecs {
		if spec.Name == name {
			return spec, true
		}
	}
	return ifaceSpec{}, false
}

// ifaceConfig is a validated static configuration.
type ifaceConfig struct {
	Prefix  netip.Prefix
	Address netip.Addr
	Gateway netip.Addr // invalid when absent
}

func (s *Service) GetInterfaces(c *gin.Context) {
	var rsp proto.Response

	out := &proto.GetInterfacesRsp{Interfaces: []proto.InterfaceInfo{}}
	for _, spec := range ifaceSpecs {
		out.Interfaces = append(out.Interfaces, describeIface(spec))
	}

	rsp.OkRspWithData(c, out)
}

func (s *Service) SetInterface(c *gin.Context) {
	var req proto.SetInterfaceReq
	var rsp proto.Response

	spec, ok := findIfaceSpec(c.Param("name"))
	if !ok {
		rsp.ErrRsp(c, -1, "unknown interface")
		return
	}

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	if err := checkIfaceApplicable(spec); err != nil {
		rsp.ErrRsp(c, -4, err.Error())
		return
	}

	data, err := renderIfaceConfig(req)
	if err != nil {
		rsp.ErrRsp(c, -2, err.Error())
		return
	}

	pending, oldGw, err := startIfaceChange(spec, req, data)
	if err != nil {
		log.Errorf("%s: change refused: %s", spec.Name, err)
		rsp.ErrRsp(c, -3, err.Error())
		return
	}

	rsp.OkRspWithData(c, pending)

	// the address may be about to change under this connection: send the
	// response out first, then apply
	c.Writer.Flush()
	go applyIface(spec, "apply", oldGw)
}

func (s *Service) ConfirmInterface(c *gin.Context) {
	var rsp proto.Response

	spec, ok := findIfaceSpec(c.Param("name"))
	if !ok {
		rsp.ErrRsp(c, -1, "unknown interface")
		return
	}

	var local netip.Addr
	if la, ok := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		if ap, err := netip.ParseAddrPort(la.String()); err == nil {
			local = ap.Addr().Unmap()
		}
	}

	if err := confirmIfaceChange(spec, local); err != nil {
		rsp.ErrRsp(c, -1, err.Error())
		return
	}

	rsp.OkRsp(c)
}

// checkIfaceApplicable refuses a change to hardware that is absent, or to a
// Wi-Fi interface that is not in station mode (the AP keeps its own address).
func checkIfaceApplicable(spec ifaceSpec) error {
	if !ifacePresent(spec) {
		return fmt.Errorf("%s is not present on this device", spec.Name)
	}
	if spec.Kind == ifaceKindWifi && ifaceAPMode() {
		return errors.New("Wi-Fi is in access-point mode; static addressing applies to station mode only")
	}
	return nil
}

func describeIface(spec ifaceSpec) proto.InterfaceInfo {
	info := proto.InterfaceInfo{
		Name:    spec.Name,
		Kind:    spec.Kind,
		Present: ifacePresent(spec),
		Mode:    ifaceModeDHCP,
		Current: proto.InterfaceCurrent{Addresses: []string{}},
	}
	if !info.Present {
		return info
	}

	info.Mode, info.Address, info.Gateway = readIfaceConfig(spec)
	info.Current = readIfaceCurrent(spec)

	if spec.Kind == ifaceKindWifi && isConnected() {
		info.Ssid = getWiFiSsid()
		if info.Ssid == "" {
			info.Ssid = "Wi-Fi"
		}
	}

	st := stateFor(spec.Name)
	st.mu.Lock()
	if st.pending != nil {
		p := *st.pending
		p.NowUnix = time.Now().Unix()
		info.Pending = &p
	}
	st.mu.Unlock()

	return info
}

// renderIfaceConfig validates the request and returns the new file contents; nil
// means "remove the file" (DHCP).
func renderIfaceConfig(req proto.SetInterfaceReq) ([]byte, error) {
	switch req.Mode {
	case ifaceModeDHCP:
		return nil, nil
	case ifaceModeStatic:
		cfg, err := validateIfaceStatic(req.Address, req.Gateway)
		if err != nil {
			return nil, err
		}
		// keep the host address, not the masked network, in the file
		line := fmt.Sprintf("%s/%d", cfg.Address, cfg.Prefix.Bits())
		if cfg.Gateway.IsValid() {
			line += " " + cfg.Gateway.String()
		}
		return []byte(line + "\n"), nil
	default:
		return nil, errors.New("invalid mode")
	}
}

// validateIfaceStatic is the authority on what may be written to the file.
func validateIfaceStatic(address, gateway string) (ifaceConfig, error) {
	var cfg ifaceConfig

	address = strings.TrimSpace(address)
	gateway = strings.TrimSpace(gateway)

	if !strings.Contains(address, "/") {
		return cfg, errors.New("address must be written a.b.c.d/nn")
	}
	prefix, err := netip.ParsePrefix(address)
	if err != nil {
		return cfg, errors.New("invalid address")
	}
	addr := prefix.Addr()
	if !addr.Is4() {
		return cfg, errors.New("only IPv4 is supported")
	}
	if prefix.Bits() < 1 || prefix.Bits() > 32 {
		return cfg, errors.New("prefix length must be between 1 and 32")
	}
	if err := checkIfaceHost(addr, "address"); err != nil {
		return cfg, err
	}

	// A /31 is a point-to-point link (RFC 3021): both addresses are hosts. A /32
	// is a lone host whose gateway lies outside it (S30eth adds a host route).
	network := prefix.Masked()
	p2p := prefix.Bits() >= 31
	host := prefix.Bits() == 32
	if !p2p && addr == network.Addr() {
		return cfg, errors.New("address is the network address")
	}
	if !p2p && addr == ifaceBroadcast(network) {
		return cfg, errors.New("address is the broadcast address")
	}

	cfg.Prefix = prefix
	cfg.Address = addr

	if gateway != "" {
		gw, err := netip.ParseAddr(gateway)
		if err != nil || !gw.Is4() {
			return cfg, errors.New("invalid gateway")
		}
		if err := checkIfaceHost(gw, "gateway"); err != nil {
			return cfg, err
		}
		if !host && !network.Contains(gw) {
			return cfg, errors.New("gateway is not inside the address's network")
		}
		if gw == addr {
			return cfg, errors.New("gateway equals the address")
		}
		if !p2p && (gw == network.Addr() || gw == ifaceBroadcast(network)) {
			return cfg, errors.New("gateway is the network or broadcast address")
		}
		cfg.Gateway = gw
	}

	return cfg, nil
}

// checkIfaceHost refuses addresses that cannot belong to a LAN host. Link-local
// (169.254/16) is allowed on purpose: a direct cable has used 169.254.50.1/16.
func checkIfaceHost(a netip.Addr, what string) error {
	switch {
	case a.IsUnspecified():
		return fmt.Errorf("%s is unspecified", what)
	case a.IsLoopback():
		return fmt.Errorf("%s is a loopback address", what)
	case a.IsMulticast():
		return fmt.Errorf("%s is a multicast address", what)
	case a.As4()[0] >= 240:
		return fmt.Errorf("%s is a reserved or broadcast address", what)
	}
	return nil
}

func ifaceBroadcast(p netip.Prefix) netip.Addr {
	b := p.Masked().Addr().As4()
	host := 32 - p.Bits()
	for i := 3; i >= 0 && host > 0; i-- {
		n := min(host, 8)
		b[i] |= byte(0xff >> (8 - n))
		host -= n
	}
	return netip.AddrFrom4(b)
}

// startIfaceChange snapshots the current file, writes the new one and arms the
// dead-man timer. It refuses while a change is already waiting for confirm. It
// returns the gateway of the configuration being replaced.
func startIfaceChange(spec ifaceSpec, req proto.SetInterfaceReq, data []byte) (*proto.InterfacePending, string, error) {
	st := stateFor(spec.Name)
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.pending != nil {
		return nil, "", errors.New("a network change is already waiting for confirmation; confirm it or wait for it to revert")
	}

	snap := readIfaceSnapshot(spec)
	if err := writeIfacePrev(spec, snap); err != nil {
		return nil, "", fmt.Errorf("failed to save the previous configuration: %w", err)
	}

	if err := writeIfaceConfig(spec, data); err != nil {
		_ = os.Remove(spec.PrevFile)
		return nil, "", fmt.Errorf("failed to write %s: %w", spec.ConfigFile, err)
	}

	deadline := time.Now().Add(ifaceConfirmWindow)
	pending := &proto.InterfacePending{
		DeadlineUnix: deadline.Unix(),
		NowUnix:      time.Now().Unix(),
		Mode:         req.Mode,
	}
	if req.Mode == ifaceModeStatic {
		pending.Address = strings.TrimSpace(req.Address)
		pending.Gateway = strings.TrimSpace(req.Gateway)
	}
	st.pending = pending
	st.applied = false
	st.timer = time.AfterFunc(ifaceConfirmWindow, func() { revertIface(spec) })

	log.Infof("%s: change written mode=%s address=%s gateway=%s, revert at %s unless confirmed",
		spec.Name, req.Mode, pending.Address, pending.Gateway, deadline.Format(time.RFC3339))

	return pending, gatewayOf(snap.Content), nil
}

// confirmIfaceChange keeps a pending change. It is accepted only once the
// change has been applied, and only on a connection whose local address is
// assigned now: a confirm sent before the apply, or over a connection to an
// address the change removed, proves nothing about the new settings.
func confirmIfaceChange(spec ifaceSpec, local netip.Addr) error {
	st := stateFor(spec.Name)
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.pending == nil {
		return errors.New("no network change is waiting for confirmation")
	}
	if !st.applied {
		return errors.New("the change is still being applied; confirm once the page is reachable on the new settings")
	}
	if !local.IsValid() || !ifaceLocalAddrAssigned(local) {
		return errors.New("confirm from the device's current address")
	}

	if st.timer != nil {
		st.timer.Stop()
		st.timer = nil
	}
	st.pending = nil

	if err := os.Remove(spec.PrevFile); err != nil && !os.IsNotExist(err) {
		log.Errorf("%s: failed to delete %s: %s", spec.Name, spec.PrevFile, err)
	}

	log.Infof("%s: change confirmed", spec.Name)
	return nil
}

// revertIface is the dead-man: it runs when the confirm window passes.
func revertIface(spec ifaceSpec) {
	st := stateFor(spec.Name)
	st.mu.Lock()
	if st.pending == nil {
		st.mu.Unlock()
		return
	}
	st.timer = nil
	replacedGw := st.pending.Gateway
	st.mu.Unlock()

	log.Warnf("%s: no confirmation, restoring the previous configuration", spec.Name)

	// a failed restore leaves the snapshot in place, so the next start retries
	if err := restoreIfacePrev(spec); err != nil {
		log.Errorf("%s: revert failed: %s", spec.Name, err)
	}

	st.mu.Lock()
	st.pending = nil
	st.mu.Unlock()

	applyIface(spec, "revert", replacedGw)
}

// RecoverInterfaces restores every change that was never confirmed (the server
// or the box went down inside the window). Called once at start.
func RecoverInterfaces() {
	for _, spec := range ifaceSpecs {
		recoverIface(spec)
	}
}

func recoverIface(spec ifaceSpec) {
	if _, err := os.Stat(spec.PrevFile); err != nil {
		if !os.IsNotExist(err) {
			log.Errorf("%s: cannot inspect %s: %s", spec.Name, spec.PrevFile, err)
		}
		return
	}

	log.Warnf("%s: found unconfirmed change at start, restoring the previous configuration", spec.Name)
	if err := restoreIfacePrev(spec); err != nil {
		log.Errorf("%s: crash revert failed: %s", spec.Name, err)
		return
	}
	go applyIface(spec, "crash revert", "")
}

// applyIface re-applies the configuration. replacedGw is the gateway of the
// static configuration being replaced, whose resolv.conf line the init script
// left behind.
func applyIface(spec ifaceSpec, why, replacedGw string) {
	log.Infof("%s: %s: running %s %s", spec.Name, why, spec.Script, strings.Join(spec.Args, " "))
	dropStaleResolvNameservers(replacedGw)
	if err := applyIfaceScript(spec); err != nil {
		log.Errorf("%s: %s: apply failed: %s", spec.Name, why, err)
	}
	dedupeResolvNameservers()

	if why == "apply" {
		st := stateFor(spec.Name)
		st.mu.Lock()
		if st.pending != nil {
			st.applied = true
		}
		st.mu.Unlock()
	}
}

func runIfaceScript(spec ifaceSpec) error {
	ctx, cancel := context.WithTimeout(context.Background(), ifaceRestartTimeout)
	defer cancel()

	// no output pipes: the script leaves udhcpc running in the background, which
	// would keep a pipe open and block Run until the timeout
	cmd := exec.CommandContext(ctx, spec.Script, spec.Args...)
	return cmd.Run()
}

// restoreIfacePrev puts the snapshot back and deletes it.
func restoreIfacePrev(spec ifaceSpec) error {
	data, err := os.ReadFile(spec.PrevFile)
	if err != nil {
		return err
	}
	var snap ifaceSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("corrupt %s: %w", spec.PrevFile, err)
	}

	if snap.Absent {
		if err := os.Remove(spec.ConfigFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	} else if err := writeFileAtomicPerm(spec.ConfigFile, []byte(snap.Content), 0); err != nil {
		return err
	}

	if err := os.Remove(spec.PrevFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	log.Infof("%s: previous configuration restored (absent=%t)", spec.Name, snap.Absent)
	return nil
}

func readIfaceSnapshot(spec ifaceSpec) ifaceSnapshot {
	data, err := os.ReadFile(spec.ConfigFile)
	if err != nil {
		return ifaceSnapshot{Absent: true}
	}
	return ifaceSnapshot{Content: string(data)}
}

func writeIfacePrev(spec ifaceSpec, snap ifaceSnapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(spec.PrevFile), 0o755); err != nil {
		return err
	}
	return writeFileAtomicPerm(spec.PrevFile, data, 0o600)
}

// writeIfaceConfig writes the new file, or removes it for DHCP (data == nil).
func writeIfaceConfig(spec ifaceSpec, data []byte) error {
	if data == nil {
		if err := os.Remove(spec.ConfigFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeFileAtomicPerm(spec.ConfigFile, data, 0)
}

// writeFileAtomicPerm writes a temporary file next to path, syncs it and renames
// it over path. perm 0 leaves the mode alone (/boot is FAT and has none).
func writeFileAtomicPerm(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	_, err = tmp.Write(data)
	if err == nil && perm != 0 {
		err = tmp.Chmod(perm)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpPath, path)
	}
	if err != nil {
		_ = os.Remove(tmpPath)
	}
	return err
}

// readIfaceConfig reports what the config file asks for, the way the init
// scripts read it: the first line with an address.
func readIfaceConfig(spec ifaceSpec) (mode, address, gateway string) {
	file, err := os.Open(spec.ConfigFile)
	if err != nil {
		return ifaceModeDHCP, "", ""
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		address = fields[0]
		if !strings.Contains(address, "/") {
			address += "/16" // the init scripts' default
		}
		if len(fields) > 1 {
			gateway = fields[1]
		}
		return ifaceModeStatic, address, gateway
	}

	return ifaceModeDHCP, "", ""
}

func readIfaceCurrent(spec ifaceSpec) proto.InterfaceCurrent {
	cur := proto.InterfaceCurrent{Addresses: []string{}}

	iface, err := net.InterfaceByName(spec.Name)
	if err != nil {
		return cur
	}
	cur.Mac = iface.HardwareAddr.String()

	addrs, err := iface.Addrs()
	if err == nil {
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.To4() == nil {
				continue
			}
			ones, _ := ipNet.Mask.Size()
			cur.Addresses = append(cur.Addresses, fmt.Sprintf("%s/%d", ipNet.IP.To4(), ones))
		}
	}

	cur.Gateway = ifaceDefaultGateway(spec.Name)
	return cur
}

// ifaceDefaultGateway reads the default route of one interface from /proc/net/route.
func ifaceDefaultGateway(name string) string {
	file, err := os.Open(ifaceRouteFile)
	if err != nil {
		return ""
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 || fields[0] != name || fields[1] != "00000000" {
			continue
		}
		return parseRouteGateway(fields[2])
	}
	return ""
}

// gatewayOf returns the gateway named by the first line of a config file body.
func gatewayOf(content string) string {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) > 1 {
			return fields[1]
		}
		return ""
	}
	return ""
}

// The init scripts append "nameserver <gateway>" to /etc/resolv.conf on every
// static start and never remove it. dropStaleResolvNameservers removes the one
// a replaced static configuration added (unless a DNS setting of dns.go also
// names it), and dedupeResolvNameservers collapses the repeats a restart leaves.

func dropStaleResolvNameservers(oldGw string) {
	if oldGw == "" {
		return
	}
	keep := map[string]bool{}
	for _, s := range readManualDNSServers() {
		keep[s] = true
	}
	if cfg, err := parseResolvConfig(dhcpResolvFile); err == nil {
		for _, s := range cfg.Servers {
			keep[s] = true
		}
	}
	if keep[oldGw] {
		return
	}
	rewriteResolv(func(server string) bool { return server != oldGw })
}

func dedupeResolvNameservers() {
	seen := map[string]bool{}
	rewriteResolv(func(server string) bool {
		if seen[server] {
			return false
		}
		seen[server] = true
		return true
	})
}

// rewriteResolv drops the nameserver lines of /etc/resolv.conf for which keep
// returns false; every other line stays as it is.
func rewriteResolv(keep func(server string) bool) {
	data, err := os.ReadFile(ifaceResolv)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" && !keep(fields[1]) {
			changed = true
			continue
		}
		out = append(out, line)
	}
	if !changed {
		return
	}
	if err := os.WriteFile(ifaceResolv, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		log.Warnf("failed to tidy %s: %s", ifaceResolv, err)
	}
}
