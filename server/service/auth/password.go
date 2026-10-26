package auth

import (
	"NanoKVM-Server/authn"
	"NanoKVM-Server/config"
	"NanoKVM-Server/middleware"
	"NanoKVM-Server/proto"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func (s *Service) ChangePassword(c *gin.Context) {
	var req proto.ChangePasswordReq
	var rsp proto.Response
	if err := proto.ParseFormRequest(c, &req); err != nil {
		rsp.ErrRsp(c, -1, "invalid parameters")
		return
	}
	principal, ok := middleware.CurrentPrincipal(c)
	if !ok {
		rsp.ErrRsp(c, -2, "invalid session")
		return
	}
	currentPassword := req.CurrentPassword
	if currentPassword == "" {
		rsp.ErrRsp(c, -3, "current password is required")
		return
	}
	if _, authenticated, authErr := authn.DefaultStore.Authenticate(principal.Username, currentPassword); authErr != nil {
		rsp.ErrRsp(c, -4, "authentication unavailable")
		return
	} else if !authenticated {
		rsp.ErrRsp(c, -3, "current password is incorrect")
		return
	}
	// the web password is root's Linux password: one store, one change
	if _, err := authn.DefaultStore.SetPassword(req.Password); err != nil {
		log.Errorf("failed to change the root password: %s", err)
		rsp.ErrRsp(c, -5, err.Error())
		return
	}

	middleware.RevokeUserSessions(principal.Username)
	clearSessionCookie(c)
	rsp.OkRsp(c)
	log.Infof("password changed for user: %s", principal.Username)
}

func (s *Service) IsPasswordUpdated(c *gin.Context) {
	var rsp proto.Response
	if config.GetInstance().Authentication == "disable" {
		rsp.OkRspWithData(c, &proto.IsPasswordUpdatedRsp{IsUpdated: true})
		return
	}
	principal, ok := middleware.CurrentPrincipal(c)
	if !ok {
		rsp.ErrRsp(c, -1, "invalid session")
		return
	}
	user, err := authn.DefaultStore.Get(principal.Username)
	if err != nil {
		rsp.ErrRsp(c, -2, "failed to get password state")
		return
	}
	rsp.OkRspWithData(c, &proto.IsPasswordUpdatedRsp{IsUpdated: !user.MustChangePassword})
}
