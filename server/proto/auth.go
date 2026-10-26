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
