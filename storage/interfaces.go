package storage

import (
	"goshorty/models"
)

// URLStore defines persistence operations required by URLService.
type URLStore interface {
	SetIfAbsent(shortCode string, urlData *models.URLData) error
	Get(shortCode string) (*models.URLData, error)
	Delete(shortCode string) error
	GetAndIncrement(shortCode string) (*models.URLData, error)
	GetAllFor(userID, role string) ([]*models.URLData, error)
	StatsFor(userID, role string) (map[string]interface{}, error)
}

// UserStore defines persistence operations required by authentication handlers.
type UserStore interface {
	CreateUser(username, password, email string) (*models.User, error)
	GetUser(username string) (*models.User, error)
	GetUserByID(id string) (*models.User, error)
	VerifyPassword(username, password string) (bool, error)
	UpdateUserRole(username, role string) error
	GetAllUsers() ([]*models.User, error)
	DeleteUser(username string) error
}

var (
	_ URLStore  = (*Storage)(nil)
	_ UserStore = (*UserStorage)(nil)
	_ URLStore  = (*PostgresStore)(nil)
	_ UserStore = (*PostgresStore)(nil)
)
