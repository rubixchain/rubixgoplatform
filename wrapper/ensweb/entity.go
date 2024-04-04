package ensweb

import (
	"fmt"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/jinzhu/gorm"
	"github.com/rubixchain/rubixgoplatform/crypto"
	"github.com/rubixchain/rubixgoplatform/wrapper/uuid"
)

const (
	UserEntity     string = "UserEntity"
	RoleEntity     string = "RoleEntity"
	UserRoleEntity string = "UserRoleEntity"
	TenantEntity   string = "TenantEntity"
	DefaultEntity  string = "DefaultEntity"
)

type Entity struct {
	EntityName  string
	EntityModel interface{}
}

type EntityConfig struct {
	DefaultTenantName    string
	DefaultAdminName     string
	DefaultAdminPassword string
	TenantTableName      string
	UserTableName        string
	RoleTableName        string
	UserRoleTableName    string
}

// Base contains common columns for all tables.
type Base struct {
	ID                   uuid.UUID `gorm:"column:Id;primary_key;type:uniqueidentifier"`
	CreationTime         time.Time `gorm:"column:CreationTime;not null"`
	CreatorID            uuid.UUID `gorm:"column:CreatorId;type:uniqueidentifier"`
	LastModificationTime time.Time `gorm:"column:LastModificationTime"`
	LastModifierID       uuid.UUID `gorm:"column:LastModifierId;type:uniqueidentifier"`
	TenantID             uuid.UUID `gorm:"column:TenantId;type:uniqueidentifier"`
}

// Tenant List
type Tenant struct {
	Base
	ExtraProperties  string    `gorm:"column:ExtraProperties"`
	ConcurrencyStamp string    `gorm:"column:ConcurrencyStamp"`
	IsDeleted        bool      `gorm:"column:IsDeleted;not null;type:bit"`
	DeleterID        uuid.UUID `gorm:"column:DeleterId;type:uniqueidentifier"`
	DeletionTime     time.Time `gorm:"column:DeletionTime"`
	Name             string    `gorm:"column:Name;not null" json:"Name"`
	EditionID        uuid.UUID `gorm:"column:EditionId;type:uniqueidentifier"`
}

// User user table
type User struct {
	Base
	ConcurrencyStamp     string    `gorm:"column:ConcurrencyStamp"`
	IsDeleted            bool      `gorm:"column:IsDeleted;not null;type:bit"`
	DeleterID            uuid.UUID `gorm:"column:DeleterId;type:uniqueidentifier"`
	DeletionTime         time.Time `gorm:"column:DeletionTime"`
	UserName             string    `gorm:"column:UserName;not null" json:"UserName"`
	NormalizedUserName   string    `gorm:"column:NormalizedUserName;not null" json:"NormalizedUserName"`
	Name                 string    `gorm:"column:Name;size:64;not null" json:"Name"`
	Surname              string    `gorm:"column:Surname;size:64;not null" json:"Surname"`
	Email                string    `gorm:"column:Email;size:256;not null" json:"Email"`
	NormalizedEmail      string    `gorm:"column:NormalizedEmail;size:256;not null" json:"NormalizedEmail"`
	EmailConfirmed       bool      `gorm:"column:EmailConfirmed;not null;type:bit"`
	PasswordHash         string    `gorm:"column:PasswordHash;not null" json:"PasswordHash"`
	SecurityStamp        string    `gorm:"column:SecurityStamp"`
	IsExternal           bool      `gorm:"column:IsExternal;type:bit"`
	PhoneNumber          string    `gorm:"column:PhoneNumber"`
	PhoneNumberConfirmed bool      `gorm:"column:PhoneNumberConfirmed;not null;type:bit"`
	TwoFactorEnabled     bool      `gorm:"column:TwoFactorEnabled;not null;type:bit"`
	LockoutEnd           time.Time `gorm:"column:LockoutEnd"`
	LockoutEnabled       bool      `gorm:"column:LockoutEnabled;not null;type:bit"`
	AccessFailedCount    int       `gorm:"column:AccessFailedCount;not null"`
	ExtraProperties      string    `gorm:"column:ExtraProperties;not null"`
	Roles                []Role
}

type Role struct {
	ID               uuid.UUID `gorm:"column:Id;primary_key;type:uniqueidentifier"`
	ConcurrencyStamp string    `gorm:"column:ConcurrencyStamp"`
	ExtraProperties  string    `gorm:"column:ExtraProperties"`
	TenantID         uuid.UUID `gorm:"column:TenantId;type:uniqueidentifier"`
	Name             string    `gorm:"column:Name;not null" json:"Name"`
	NormalizedName   string    `gorm:"column:NormalizedName;not null" json:"NormalizedName"`
	IsDefault        bool      `gorm:"column:IsDefault;type:bit;not null"`
	IsStatic         bool      `gorm:"column:IsStatic;type:bit;not null"`
	IsPublic         bool      `gorm:"column:IsPublic;type:bit;not null"`
	DeleterUserID    int64     `gorm:"column:DeleterUserID"`
}

// UserRole user role table
type UserRole struct {
	UserID   uuid.UUID `gorm:"column:UserId;not null;type:uniqueidentifier"`
	RoleID   uuid.UUID `gorm:"column:RoleId;not null;type:uniqueidentifier"`
	TenantID uuid.UUID `gorm:"column:TenantId;type:uniqueidentifier"`
}

type BasicToken struct {
	UserName string `json:"username"`
	UserID   string `json:"user_id"`
	Role     string `json:"role"`
	jwt.StandardClaims
}

type LoginRequest struct {
	UserName string `json:"userName"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Token   string `json:"token"`
	Role    string `json:"role"`
	User    *User  `json:"user"`
}

// BeforeCreate will set a UUID rather than numeric ID.
func (b *Base) BeforeCreate(scope *gorm.Scope) error {
	uuid := uuid.New()

	err := scope.SetColumn("CreationTime", time.Now())
	if err != nil {
		return err
	}
	return scope.SetColumn("ID", uuid)
}

// BeforeCreate will set a UUID rather than numeric ID.
func (b *Base) BeforeUpdate(scope *gorm.Scope) error {
	return scope.SetColumn("LastModificationTime", time.Now())
}

// BeforeCreate will set a UUID rather than numeric ID.
func (b *Base) BeforeSave(scope *gorm.Scope) error {
	return scope.SetColumn("LastModificationTime", time.Now())
}

func (s *Server) SetupEntity(cfg EntityConfig) error {
	s.entityConfig = cfg
	if s.db == nil {
		return fmt.Errorf("db is not confgured")
	}
	err := s.db.InitTable(cfg.TenantTableName, &Tenant{}, true)
	if err != nil {
		return err
	}
	err = s.db.InitTable(cfg.UserTableName, &User{}, true)
	if err != nil {
		return err
	}
	err = s.db.InitTable(cfg.RoleTableName, &Role{}, true)
	if err != nil {
		return err
	}
	err = s.db.InitTable(cfg.UserRoleTableName, &UserRole{}, true)
	if err != nil {
		return err
	}
	if s.cfg.DBType != "Sqlite3" {
		err = s.AddForienKey(cfg.UserRoleTableName, &UserRole{}, "UserId", cfg.UserTableName, "Id")
		if err != nil {
			return err
		}
		err = s.AddForienKey(cfg.UserRoleTableName, &UserRole{}, "RoleId", cfg.RoleTableName, "Id")
		if err != nil {
			return err
		}
	}
	t, err := s.GetTenant(cfg.DefaultTenantName)
	if err != nil {
		t = &Tenant{
			Name: cfg.DefaultTenantName,
		}
		err = s.CreateTenant(t)
		if err != nil {
			return err
		}
	}
	s.SetDefaultTenant(t.ID)
	return nil
}

func (s *Server) AddEntity(entitytName string, entityModel interface{}) error {
	if s.db == nil {
		return fmt.Errorf("db is not initialised")
	}
	return s.db.InitTable(entitytName, entityModel, true)
}

// DropTable drop the table
func (s *Server) AddForienKey(entitytName string, entityModel interface{}, column string, forienEntityName string, forienEntityColumn string) error {
	tableStr := forienEntityName + "(" + forienEntityColumn + ")"
	return s.db.AddForienKey(entitytName, entityModel, column, tableStr)
}

func (s *Server) CreateEntity(entitytName string, value interface{}) error {
	if s.db == nil {
		return fmt.Errorf("db is not initialised")
	}
	return s.db.Create(entitytName, value)
}

func (s *Server) GetEntity(entitytName string, tenantID interface{}, format string, item interface{}, value ...interface{}) error {
	if s.db == nil {
		return fmt.Errorf("db is not initialised")
	}
	return s.db.FindNew(tenantID, entitytName, format, item, value...)
}

func (s *Server) UpdateEntity(entitytName string, tenantID interface{}, item interface{}, format string, value ...interface{}) error {
	if s.db == nil {
		return fmt.Errorf("db is not initialised")
	}
	return s.db.UpdateNew(tenantID, entitytName, format, item, value...)
}

func (s *Server) SaveEntity(entitytName string, tenantID interface{}, item interface{}, format string, value ...interface{}) error {
	if s.db == nil {
		return fmt.Errorf("db is not initialised")
	}
	return s.db.SaveNew(tenantID, entitytName, format, item, value...)
}

func (s *Server) DeleteEntity(entitytName string, tenantID interface{}, format string, item interface{}, value ...interface{}) error {
	if s.db == nil {
		return fmt.Errorf("db is not initialised")
	}
	return s.db.DeleteNew(tenantID, entitytName, format, item, value)
}

// GetTenant will get tenant
func (s *Server) GetTenant(name string) (*Tenant, error) {
	var t Tenant
	err := s.db.FindNew(uuid.Nil, s.entityConfig.TenantTableName, "Name=?", &t, name)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateRole will create role
func (s *Server) CreateRole(r *Role) error {
	return s.db.Create(s.entityConfig.RoleTableName, r)
}

// GetRole will get role
func (s *Server) GetRole(name string) (*Role, error) {
	var r Role
	err := s.db.FindNew(uuid.Nil, s.entityConfig.RoleTableName, "NormalizedName=?", &r, strings.ToUpper(name))
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateUserRole will create user role
func (s *Server) CreateUserRole(ur *UserRole) error {
	return s.db.Create(s.entityConfig.UserRoleTableName, ur)
}

// GetUserRole will get user role
func (s *Server) GetUserRole(u *User) ([]UserRole, error) {
	var ur []UserRole
	err := s.db.FindNew(u.TenantID, s.entityConfig.UserRoleTableName, "UserID=?", &ur, u.ID)
	if err != nil {
		return nil, err
	}
	return ur, nil
}

// CreateTenant will create tenant
func (s *Server) CreateTenant(t *Tenant) error {
	err := s.db.Create(s.entityConfig.TenantTableName, t)
	if err != nil {
		return err
	}
	r := &Role{
		ID:             uuid.New(),
		TenantID:       t.ID,
		Name:           "admin",
		NormalizedName: strings.ToUpper("admin"),
		IsDefault:      false,
		IsStatic:       true,
		IsPublic:       true,
	}
	err = s.CreateRole(r)
	if err != nil {
		return err
	}
	r = &Role{
		ID:             uuid.New(),
		TenantID:       t.ID,
		Name:           "user",
		NormalizedName: strings.ToUpper("user"),
		IsDefault:      true,
		IsStatic:       true,
		IsPublic:       true,
	}
	err = s.CreateRole(r)
	if err != nil {
		return err
	}
	u := &User{
		Base: Base{
			TenantID: t.ID,
		},
		UserName:           s.entityConfig.DefaultAdminName,
		NormalizedUserName: strings.ToUpper(s.entityConfig.DefaultAdminName),
		Name:               "Administrator",
		PasswordHash:       crypto.HashPassword(s.entityConfig.DefaultAdminPassword, 3, 1, 1000),
		Roles: []Role{
			{
				Name:           "admin",
				NormalizedName: strings.ToUpper("admin"),
			},
		},
	}
	err = s.CreateUser(u)
	return err
}

func (s *Server) GetUser(tenantID interface{}, userName string) (*User, error) {
	var u User
	value := make([]interface{}, 0)
	value = append(value, strings.ToUpper(userName))
	value = append(value, false)
	err := s.db.FindNew(tenantID, s.entityConfig.UserTableName, "NormalizedUserName=? AND IsDeleted=?", &u, value...)
	if err != nil {
		return nil, err
	}
	ur, err := s.GetUserRole(&u)
	if err != nil {
		return nil, err
	}
	u.Roles = make([]Role, 0)
	for i := range ur {
		var r Role
		err = s.db.FindNew(tenantID, s.entityConfig.RoleTableName, "Id=?", &r, ur[i].RoleID)
		if err != nil {
			return nil, err
		}
		u.Roles = append(u.Roles, r)
	}
	return &u, nil
}

func (s *Server) GetUserByID(tenantID interface{}, id uuid.UUID) (*User, error) {
	var u User
	value := make([]interface{}, 0)
	value = append(value, id)
	value = append(value, false)
	err := s.db.FindNew(tenantID, s.entityConfig.UserTableName, "Id=? AND IsDeleted=?", &u, value...)
	if err != nil {
		return nil, err
	}
	ur, err := s.GetUserRole(&u)
	if err != nil {
		return nil, err
	}
	u.Roles = make([]Role, 0)
	for i := range ur {
		var r Role
		err = s.db.FindNew(tenantID, s.entityConfig.RoleTableName, "Id=?", &r, ur[i].RoleID)
		if err != nil {
			return nil, err
		}
		u.Roles = append(u.Roles, r)
	}
	return &u, nil
}

func (s *Server) GetUsers(tenantID interface{}, format string, value ...interface{}) ([]User, error) {
	var us []User
	if format == "*" {
		format = "IsDeleted=?"
	} else {
		format = format + " AND isDeleted=?"
	}
	value = append(value, false)
	err := s.db.FindNew(tenantID, s.entityConfig.UserTableName, format, &us, value...)
	if err != nil {
		return nil, err
	}
	for i := range us {
		ur, err := s.GetUserRole(&us[i])
		if err != nil {
			return nil, err
		}
		us[i].Roles = make([]Role, 0)
		for i := range ur {
			var r Role
			err = s.db.FindNew(tenantID, s.entityConfig.RoleTableName, "Id=?", &r, ur[i].RoleID)
			if err != nil {
				return nil, err
			}
			us[i].Roles = append(us[i].Roles, r)
		}
	}
	return us, nil
}

func (s *Server) CreateUser(u *User) error {
	err := s.db.Create(s.entityConfig.UserTableName, u)
	if err != nil {
		return err
	}
	for i := range u.Roles {
		r, err := s.GetRole(u.Roles[i].Name)
		if err != nil {
			return err
		}
		ur := &UserRole{
			UserID:   u.ID,
			RoleID:   r.ID,
			TenantID: u.TenantID,
		}
		err = s.CreateUserRole(ur)
		if err != nil {
			return err
		}
	}
	var roles []Role
	err = s.db.FindNew(u.TenantID, s.entityConfig.RoleTableName, "IsDefault=?", &roles, true)
	if err == nil {
		for _, r := range roles {
			ur := &UserRole{
				UserID:   u.ID,
				RoleID:   r.ID,
				TenantID: u.TenantID,
			}
			err = s.CreateUserRole(ur)
			if err != nil {
				return err
			}
		}
	}
	return err
}

func (s *Server) DeleteUser(tenantID interface{}, id uuid.UUID) error {
	u, err := s.GetUserByID(tenantID, id)
	if err != nil {
		return err
	}
	u.IsDeleted = true
	u.Roles = nil
	return s.UpdateUser(u)
}

func (s *Server) UpdateUser(u *User) error {
	return s.db.UpdateNew(u.TenantID, s.entityConfig.UserTableName, "Id=?", u, u.ID)
}

func (s *Server) LoginUser(tenantID interface{}, req *LoginRequest) *LoginResponse {
	u, err := s.GetUser(tenantID, req.UserName)
	resp := &LoginResponse{
		Status: false,
	}
	if err != nil {
		resp.Message = "User not found"
		return resp
	}
	if crypto.VerifyPassword(req.Password, u.PasswordHash) {
		role := "user"
		for _, r := range u.Roles {
			if r.Name == "admin" {
				role = "admin"
			}
		}
		expiresAt := time.Now().Add(time.Minute * 60).Unix()
		claims := BasicToken{
			u.Name,
			u.ID.String(),
			role,
			jwt.StandardClaims{
				ExpiresAt: expiresAt,
			},
		}

		token := s.GenerateJWTToken(claims)
		resp.Status = true
		resp.Message = "User logged in successfully"
		resp.Token = token
		resp.Role = role
		resp.User = u
		return resp
	} else {
		resp.Message = "Password mismatch"
		u.AccessFailedCount++
		s.UpdateUser(u)
		return resp
	}
}
