package vm

import (
	"errors"
	"os"
	"os/exec"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"NanoKVM-Server/proto"
	"NanoKVM-Server/service/hid"
	"NanoKVM-Server/service/storage"
)

const (
	virtualNetwork = "/boot/usb.rndis0"
	virtualDisk    = "/boot/usb.disk0"
)

// deviceStep is one action of a gadget change, done with os calls or an argv
// exec, never through a shell
type deviceStep func() error

func touchFile(path string) deviceStep {
	return func() error {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o666)
		if err != nil {
			return err
		}
		return f.Close()
	}
}

func removeFile(path string) deviceStep {
	return func() error { return os.Remove(path) }
}

func removeTree(path string) deviceStep {
	return func() error { return os.RemoveAll(path) }
}

func gadgetService(action string) deviceStep {
	return func() error { return exec.Command("/bin/sh", hid.USBDevScript, action).Run() }
}

var (
	mountNetworkCommands = []deviceStep{
		touchFile(virtualNetwork),
		gadgetService("stop"),
		gadgetService("start"),
	}

	unmountNetworkCommands = []deviceStep{
		gadgetService("stop"),
		removeTree("/sys/kernel/config/usb_gadget/g0/configs/c.1/rndis.usb0"),
		removeFile(virtualNetwork),
		gadgetService("start"),
	}

	mountDiskCommands = []deviceStep{
		touchFile(virtualDisk),
		gadgetService("stop"),
		gadgetService("start"),
	}

	unmountDiskCommands = []deviceStep{
		gadgetService("stop"),
		removeTree("/sys/kernel/config/usb_gadget/g0/configs/c.1/mass_storage.disk0"),
		removeFile(virtualDisk),
		gadgetService("start"),
	}
)

func (s *Service) GetVirtualDevice(c *gin.Context) {
	var rsp proto.Response

	network, _ := isDeviceExist(virtualNetwork)

	rsp.OkRspWithData(c, &proto.GetVirtualDeviceRsp{
		Network: network,
		Disk:    diskAttached(),
	})
	log.Debugf("get virtual device success")
}

func (s *Service) UpdateVirtualDevice(c *gin.Context) {
	var req proto.UpdateVirtualDeviceReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid argument")
		return
	}

	var device string
	var commands []deviceStep

	switch req.Device {
	case "network":
		device = virtualNetwork

		exist, _ := isDeviceExist(device)
		if !exist {
			commands = mountNetworkCommands
		} else {
			commands = unmountNetworkCommands
		}
	case "disk":
		device = virtualDisk

		if diskAttached() {
			commands = unmountDiskCommands
		} else {
			// the host writes the partition from now on. A mounted image is released by
			// the restarted gadget, which attaches the raw partition again.
			if err := storage.SetDataWritable(false); err != nil {
				log.Errorf("make /data read-only failed: %s", err)
				rsp.ErrRsp(c, -3, "make /data read-only failed")
				return
			}

			commands = mountDiskCommands
		}
	default:
		rsp.ErrRsp(c, -2, "invalid arguments")
		return
	}

	h := hid.GetHid()
	h.Lock()
	h.CloseNoLock()
	defer func() {
		h.OpenNoLock()
		h.Unlock()
	}()

	for _, command := range commands {
		if err := command(); err != nil {
			rsp.ErrRsp(c, -3, "operation failed")
			return
		}
	}

	// the disk is gone from the host, the KVM writes /data itself again
	if device == virtualDisk && !diskAttached() {
		if err := storage.SetDataWritable(true); err != nil {
			log.Errorf("make /data writable failed: %s", err)
		}
	}

	on, _ := isDeviceExist(device)
	rsp.OkRspWithData(c, &proto.UpdateVirtualDeviceRsp{
		On: on,
	})

	log.Debugf("update virtual device %s success", req.Device)
}

// diskAttached reports whether the virtual disk is enabled and the /data
// partition is currently shared with the host. A virtual disk that serves an
// image, or nothing at all, counts as disabled.
func diskAttached() bool {
	enabled, _ := isDeviceExist(virtualDisk)
	return enabled && storage.DataDiskAttached()
}

func isDeviceExist(device string) (bool, error) {
	_, err := os.Stat(device)

	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	log.Errorf("check file %s err: %s", device, err)
	return false, err
}
