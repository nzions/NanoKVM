package proto

type LoginReq struct {
	Username string `validate:"required"`
	Password string `validate:"required"`
}

type GetAccountRsp struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type ChangePasswordReq struct {
	CurrentPassword string `json:"currentPassword"`
	Password        string `json:"password" validate:"required"`
}

type IsPasswordUpdatedRsp struct {
	IsUpdated bool `json:"isUpdated"`
}

// Codes of the first-connection password setup. Like every response of this
// API they travel in the body of an HTTP 200.
const (
	// ErrCodePasswordUnset is the code of POST /api/auth/login while root has no
	// password: the client must call POST /api/auth/setup.
	ErrCodePasswordUnset = -6
	// ErrCodeSetupDone is the code of POST /api/auth/setup once a password exists.
	ErrCodeSetupDone = -7
)

type SetupPasswordReq struct {
	Password string `json:"password" validate:"required"`
}

// AuthStateRsp is the unauthenticated answer of GET /api/auth/state.
type AuthStateRsp struct {
	PasswordSet bool `json:"password_set"`
}
