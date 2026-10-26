package vm

import (
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"NanoKVM-Server/config"
	"NanoKVM-Server/gpio"
	"NanoKVM-Server/proto"
)

func (s *Service) SetGpio(c *gin.Context) {
	var req proto.SetGpioReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, fmt.Sprintf("invalid arguments: %s", err))
		return
	}

	line, device := "", ""
	conf := config.GetInstance().Hardware

	switch req.Type {
	case "power":
		line, device = conf.LinePower, conf.GPIOPower
	case "reset":
		line, device = conf.LineReset, conf.GPIOReset
	default:
		rsp.ErrRsp(c, -2, fmt.Sprintf("invalid power event: %s", req.Type))
		return
	}

	var duration time.Duration
	if req.Duration > 0 {
		duration = time.Duration(req.Duration) * time.Millisecond
	} else {
		duration = 800 * time.Millisecond
	}

	if err := writeGpio(line, device, duration); err != nil {
		rsp.ErrRsp(c, -3, fmt.Sprintf("operation failed: %s", err))
		return
	}

	log.Debugf("gpio %s (%s) set successfully", line, device)
	rsp.OkRsp(c)
}

func (s *Service) GetGpio(c *gin.Context) {
	var rsp proto.Response

	conf := config.GetInstance().Hardware

	pwr, err := readGpio(conf.LinePowerLED, conf.GPIOPowerLED)
	if err != nil {
		rsp.ErrRsp(c, -2, fmt.Sprintf("failed to read power led: %s", err))
		return
	}

	hdd := false
	if conf.Version == config.HWVersionAlpha {
		hdd, err = readGpio(conf.LineHDDLed, conf.GPIOHDDLed)
		if err != nil {
			rsp.ErrRsp(c, -2, fmt.Sprintf("failed to read hdd led: %s", err))
			return
		}
	}

	data := &proto.GetGpioRsp{
		PWR: pwr,
		HDD: hdd,
	}
	rsp.OkRspWithData(c, data)
}

// writeGpio presses a key line for duration. The line is found by its DT name
// over the GPIO chardev; the vendor sysfs file is used only when no line
// carries the name. A missing line is an error, never a guessed number.
func writeGpio(line string, sysfs string, duration time.Duration) error {
	err := gpio.Pulse(line, duration)
	if err == nil {
		return nil
	}
	if !errors.Is(err, gpio.ErrNoLine) || sysfs == "" {
		log.Errorf("pulse gpio line %q failed: %s", line, err)
		return err
	}
	if err := gpio.SysfsPulse(sysfs, duration); err != nil {
		log.Errorf("write gpio %s failed: %s", sysfs, err)
		return err
	}
	return nil
}

// readGpio returns true when the (active-low) LED line is lit.
func readGpio(line string, sysfs string) (bool, error) {
	high, err := gpio.Read(line)
	if err != nil {
		if !errors.Is(err, gpio.ErrNoLine) || sysfs == "" {
			log.Errorf("read gpio line %q failed: %s", line, err)
			return false, err
		}
		high, err = gpio.SysfsRead(sysfs)
		if err != nil {
			log.Errorf("read gpio %s failed: %s", sysfs, err)
			return false, err
		}
	}
	return !high, nil
}
