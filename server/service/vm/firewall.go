package vm

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"NanoKVM-Server/config"
	"NanoKVM-Server/proto"
)

const (
	// FirewallScript is the script S95nanokvm runs for `firewall`. It is run
	// directly (not through S95nanokvm) because S95nanokvm swallows its exit
	// status so a boot never stops on it; here the status is the verification.
	FirewallScript = "/kvmapp/system/nanokvm-firewall"

	firewallApplyTimeout = 30 * time.Second
)

var firewallMu sync.Mutex

// parseFirewallMode maps the stored value to a mode the way the script does:
// empty means open, anything else unknown is rejected.
func parseFirewallMode(v string) (proto.FirewallMode, error) {
	if v == "" {
		return proto.FirewallOpen, nil
	}
	m := proto.FirewallMode(v)
	if !m.Valid() {
		return "", fmt.Errorf("unknown firewall mode %q", v)
	}
	return m, nil
}

func (s *Service) GetFirewall(c *gin.Context) {
	var rsp proto.Response

	conf, err := config.Read()
	if err != nil {
		rsp.ErrRsp(c, -1, "failed to read configuration")
		return
	}

	mode, err := parseFirewallMode(conf.Firewall)
	if err != nil {
		log.Errorf("firewall: %s", err)
		rsp.ErrRsp(c, -2, "configuration holds an unknown firewall mode")
		return
	}

	rsp.OkRspWithData(c, &proto.GetFirewallRsp{Mode: mode})
}

func (s *Service) SetFirewall(c *gin.Context) {
	var req proto.SetFirewallReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil || !req.Mode.Valid() {
		rsp.ErrRsp(c, -1, "invalid arguments: mode must be open or drop")
		return
	}

	firewallMu.Lock()
	defer firewallMu.Unlock()

	conf, err := config.Read()
	if err != nil {
		rsp.ErrRsp(c, -2, "failed to read configuration")
		return
	}
	previous := conf.Firewall

	conf.Firewall = string(req.Mode)
	if err := config.Write(conf); err != nil {
		rsp.ErrRsp(c, -3, "failed to save configuration")
		return
	}

	if err := applyFirewall(context.Background()); err != nil {
		log.Errorf("firewall: applying %s failed: %s", req.Mode, err)

		// put the file back so it matches the (unchanged) inbound state
		conf.Firewall = previous
		if werr := config.Write(conf); werr != nil {
			log.Errorf("firewall: restoring server.yaml failed: %s", werr)
		}
		rsp.ErrRsp(c, -4, "applying the firewall failed; the previous mode is kept")
		return
	}

	rsp.OkRspWithData(c, &proto.GetFirewallRsp{Mode: req.Mode})
	log.Infof("firewall mode set to %s", req.Mode)
}

// applyFirewall runs the firewall script: fixed path, no shell interpolation.
// Success is the script's exit status 0, nothing else.
func applyFirewall(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, firewallApplyTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "/bin/sh", FirewallScript).CombinedOutput()
	if err != nil {
		return errors.Join(err, fmt.Errorf("script output: %s", out))
	}
	return nil
}
