package domain

import "time"

type Role string

const (
	RoleUser         Role = "user"
	RoleStoreManager Role = "company_store_manager"
	RoleSuperAdmin   Role = "superadmin"
)

func (r Role) Valid() bool {
	return r == RoleUser || r == RoleStoreManager || r == RoleSuperAdmin
}

type Actor struct {
	UserID    int64
	Role      Role
	SessionID string
}

func (a Actor) CanAccessCompanyStore() bool {
	return a.Role == RoleStoreManager || a.Role == RoleSuperAdmin
}

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type RegisterInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     Role   `json:"role"`
}

type Session struct {
	ID          string
	UserID      int64
	RefreshHash [32]byte
	ExpiresAt   time.Time
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}
