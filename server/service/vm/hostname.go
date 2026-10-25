package vm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"NanoKVM-Server/proto"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

const (
	maxHostnameLength = 253
	maxLabelLength    = 63
)

// the files are variables so tests can point them at a temporary directory
var (
	BootHostnameFile = "/boot/hostname"
	EtcHostname      = "/etc/hostname"
	EtcHosts         = "/etc/hosts"
)

// validateHostname checks an RFC 1123 hostname: dot separated labels of 1-63
// characters from [a-z0-9-], none starting or ending with a hyphen, 253
// characters at most.
func validateHostname(hostname string) error {
	if hostname == "" || len(hostname) > maxHostnameLength {
		return errors.New("invalid hostname length")
	}

	for _, label := range strings.Split(hostname, ".") {
		if label == "" || len(label) > maxLabelLength {
			return errors.New("invalid hostname label length")
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("hostname label starts or ends with a hyphen")
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return errors.New("hostname contains an invalid character")
			}
		}
	}

	return nil
}

// writeFileAtomic writes data to a temporary file in the target directory,
// syncs it and renames it over path, so a reader never sees a partial file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	_, err = tmp.Write(data)
	if err == nil {
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

func (s *Service) SetHostname(c *gin.Context) {
	var req proto.SetHostnameReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	if err := validateHostname(req.Hostname); err != nil {
		log.Warnf("reject hostname %q: %s", req.Hostname, err)
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	dataRead, err := os.ReadFile(EtcHostname)
	if err != nil {
		rsp.ErrRsp(c, -1, "read Hostname failed")
		return
	}

	oldHostname := strings.Replace(string(dataRead), "\n", "", -1)

	// an empty old name would match between every character of the file
	if oldHostname != "" && oldHostname != req.Hostname {
		dataRead, err = os.ReadFile(EtcHosts)
		if err != nil {
			rsp.ErrRsp(c, -1, "read Hosts failed")
			return
		}

		data := []byte(strings.Replace(string(dataRead), oldHostname, req.Hostname, -1))

		if err := writeFileAtomic(EtcHosts, data, 0o644); err != nil {
			rsp.ErrRsp(c, -2, "failed to write data")
			return
		}
	}

	data := []byte(req.Hostname)

	if err := writeFileAtomic(BootHostnameFile, data, 0o644); err != nil {
		rsp.ErrRsp(c, -2, "failed to write data")
		return
	}

	if err := writeFileAtomic(EtcHostname, data, 0o644); err != nil {
		rsp.ErrRsp(c, -3, "failed to write data")
		return
	}

	rsp.OkRsp(c)
	log.Debugf("set Hostname: %s", req.Hostname)

	_ = exec.Command("hostname", "-F", EtcHostname).Run()
}

func (s *Service) GetHostname(c *gin.Context) {
	var rsp proto.Response

	data, err := os.ReadFile(EtcHostname)
	if err != nil {
		rsp.ErrRsp(c, -1, "read Hostname failed")
		return
	}

	rsp.OkRspWithData(c, &proto.GetHostnameRsp{
		Hostname: strings.Replace(string(data), "\n", "", -1),
	})
	log.Debugf("get Hostname successful")
}
