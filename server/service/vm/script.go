package vm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"

	"NanoKVM-Server/proto"
	"NanoKVM-Server/utils"
)

// ScriptDirectory is a variable so tests can point it at a temporary directory
var ScriptDirectory = "/etc/kvm/scripts"

var validScriptName = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

func (s *Service) GetScripts(c *gin.Context) {
	var rsp proto.Response

	var files []string
	err := filepath.Walk(ScriptDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && isScript(info.Name()) {
			files = append(files, info.Name())
		}

		return nil
	})
	if err != nil {
		rsp.ErrRsp(c, -1, "get scripts failed")
		return
	}

	rsp.OkRspWithData(c, &proto.GetScriptsRsp{
		Files: files,
	})

	log.Debugf("get scripts total %d", len(files))
}

func (s *Service) UploadScript(c *gin.Context) {
	var rsp proto.Response

	_, header, err := c.Request.FormFile("file")
	if err != nil {
		rsp.ErrRsp(c, -1, "bad request")
		return
	}

	if _, err = os.Stat(ScriptDirectory); err != nil {
		_ = os.MkdirAll(ScriptDirectory, 0o755)
	}

	target, err := scriptPath(header.Filename)
	if err != nil {
		log.Warnf("reject upload of script %q: %s", header.Filename, err)
		rsp.ErrRsp(c, -2, "invalid arguments")
		return
	}

	err = c.SaveUploadedFile(header, target)
	if err != nil {
		rsp.ErrRsp(c, -2, "save failed")
		return
	}

	_ = utils.EnsurePermission(target, 0o100)

	data := &proto.UploadScriptRsp{
		File: header.Filename,
	}
	rsp.OkRspWithData(c, data)

	log.Debugf("upload script %s success", header.Filename)
}

func (s *Service) RunScript(c *gin.Context) {
	var req proto.RunScriptReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	script, err := scriptPath(req.Name)
	if err != nil {
		log.Warnf("reject run of script %q: %s", req.Name, err)
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	// argv only, the name never reaches a shell
	var cmd *exec.Cmd
	if strings.HasSuffix(strings.ToLower(script), ".py") {
		cmd = exec.Command("python", script)
	} else {
		cmd = exec.Command("/bin/sh", script)
	}

	var output []byte

	if req.Type == "foreground" {
		output, err = cmd.CombinedOutput()
	} else {
		cmd.Stdout = nil
		cmd.Stderr = nil
		go func() {
			err := cmd.Run()
			if err != nil {
				log.Errorf("run script %s in background failed: %s", req.Name, err)
			}
		}()
	}

	if err != nil {
		log.Errorf("run script %s failed: %s", req.Name, err.Error())
		rsp.ErrRsp(c, -2, "run script failed")
		return
	}

	rsp.OkRspWithData(c, &proto.RunScriptRsp{
		Log: string(output),
	})

	log.Debugf("run script %s success", req.Name)
}

func (s *Service) DeleteScript(c *gin.Context) {
	var req proto.DeleteScriptReq
	var rsp proto.Response

	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	file, err := scriptPath(req.Name)
	if err != nil {
		log.Warnf("reject delete of script %q: %s", req.Name, err)
		rsp.ErrRsp(c, -1, "invalid arguments")
		return
	}

	if err := os.Remove(file); err != nil {
		log.Errorf("delete script %s failed: %s", file, err)
		rsp.ErrRsp(c, -3, "delete failed")
		return
	}

	rsp.OkRsp(c)
	log.Debugf("delete script %s success", file)
}

// scriptPath maps a client supplied script name to its path inside
// ScriptDirectory. The name must be a plain file name with a script extension,
// and an existing file must not be a symlink that leads out of the directory.
func scriptPath(name string) (string, error) {
	if !validScriptName.MatchString(name) || !isScript(name) {
		return "", errors.New("invalid script name")
	}

	root, err := filepath.EvalSymlinks(ScriptDirectory)
	if err != nil {
		return "", err
	}

	path := filepath.Join(root, name)

	if _, err := os.Lstat(path); err == nil {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		// the separator keeps /scripts-other from matching /scripts
		if !strings.HasPrefix(resolved, root+"/") {
			return "", errors.New("script is outside " + ScriptDirectory)
		}
		path = resolved
	}

	return path, nil
}

func isScript(name string) bool {
	nameLower := strings.ToLower(name)
	if strings.HasSuffix(nameLower, ".sh") || strings.HasSuffix(nameLower, ".py") {
		return true
	}

	return false
}
