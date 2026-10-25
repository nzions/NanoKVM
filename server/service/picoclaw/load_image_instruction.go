package picoclaw

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// There is deliberately no load-image route: it told picoclaw to read an
// arbitrary file path, a file-read lever the locked-down agent must not have.

func (s *Service) requireActiveGatewaySession(c *gin.Context) (string, *GatewaySession, *PicoclawError) {
	sessionID := strings.TrimSpace(c.GetHeader(sessionIDHeader))
	if sessionID == "" {
		sessionID = strings.TrimSpace(s.lock.Owner())
	}
	if sessionID == "" {
		return "", nil, newPicoclawError(CodeSessionIDMissing, "missing X-PicoClaw-Session-ID")
	}

	session, ok := GetSessionManager().Get(sessionID)
	if !ok || session == nil {
		err := newPicoclawError(CodeRuntimeUnavailable, "picoclaw session is not connected")
		err.SessionID = sessionID
		return "", nil, err
	}
	if session.State != SessionStateActive || session.Upstream == nil {
		err := newPicoclawError(CodeRuntimeUnavailable, "picoclaw session is not active")
		err.SessionID = sessionID
		return "", nil, err
	}

	return sessionID, session, nil
}
