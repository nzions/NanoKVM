package auth

import (
	"errors"
	"time"

	"NanoKVM-Server/authn"
	"NanoKVM-Server/config"
	"NanoKVM-Server/middleware"
	"NanoKVM-Server/proto"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// GetState is the unauthenticated GET /api/auth/state: whether root has a
// password. password_set is false only on a device that booted without
// /boot/root.pass (or was button-reset) and is waiting for its first connection.
func (s *Service) GetState(c *gin.Context) {
	var rsp proto.Response
	if config.GetInstance().Authentication == "disable" {
		rsp.OkRspWithData(c, &proto.AuthStateRsp{PasswordSet: true})
		return
	}
	set, err := authn.DefaultStore.PasswordSet()
	if err != nil {
		log.Errorf("load account state: %s", err)
		rsp.ErrRsp(c, -3, "authentication unavailable")
		return
	}
	rsp.OkRspWithData(c, &proto.AuthStateRsp{PasswordSet: set})
}

// Setup is the unauthenticated POST /api/auth/setup {"password": "..."}: the
// first connection chooses root's password. It is accepted only while the
// password is unset; it then answers exactly like a successful login (the
// nano-kvm-token cookie), and every later call is ErrCodeSetupDone.
func (s *Service) Setup(c *gin.Context) {
	var req proto.SetupPasswordReq
	var rsp proto.Response

	if config.GetInstance().Authentication == "disable" {
		rsp.ErrRsp(c, proto.ErrCodeSetupDone, "password already set")
		return
	}
	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid parameters")
		return
	}
	user, err := authn.DefaultStore.SetupPassword(req.Password)
	if err != nil {
		if errors.Is(err, authn.ErrPasswordAlreadySet) {
			rsp.ErrRsp(c, proto.ErrCodeSetupDone, "password already set")
			return
		}
		if verr := authn.ValidatePassword(req.Password); verr != nil {
			rsp.ErrRsp(c, -2, verr.Error())
			return
		}
		log.Errorf("failed to set the root password: %s", err)
		rsp.ErrRsp(c, -3, "authentication unavailable")
		return
	}

	token, err := middleware.GenerateJWT(user.Username, user.TokenVersion)
	if err != nil {
		time.Sleep(time.Second)
		rsp.ErrRsp(c, -3, "generate token failed")
		return
	}
	setSessionCookie(c, token)
	rsp.OkRsp(c)
	log.Infof("root password set by the first connection; user logged in: %s", user.Username)
}
