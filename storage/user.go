package storage

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"goshorty/models"
	"goshorty/utils"
)

const defaultAdmin = "admin"

// ErrUserNotFound is returned when a user lookup fails
var ErrUserNotFound = errors.New("user not found")

// ErrUsernameExists is returned when a username is already registered.
var ErrUsernameExists = errors.New("username already exists")

// ErrInvalidRole is returned when a role is not supported.
var ErrInvalidRole = errors.New("invalid role")

// ErrLastAdmin is returned when an operation would remove the final admin.
var ErrLastAdmin = errors.New("cannot remove the last admin")

// UserStorage manages user data
type UserStorage struct {
	mu    sync.RWMutex
	users map[string]*models.User // map by username
	byID  map[string]*models.User // map by ID
}

// NewUserStorage creates a new user storage instance.
// adminPassword and adminEmail are used to create the initial admin user.
func NewUserStorage(adminPassword, adminEmail string) (*UserStorage, error) {
	storage := &UserStorage{
		users: make(map[string]*models.User),
		byID:  make(map[string]*models.User),
	}

	adminPass, err := hashPassword(adminPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to hash admin password: %w", err)
	}

	adminUser := &models.User{
		ID:        utils.GenerateID(),
		Username:  defaultAdmin,
		Password:  adminPass,
		Email:     adminEmail,
		Role:      models.RoleAdmin,
		CreatedAt: time.Now().Unix(),
	}
	storage.users[defaultAdmin] = adminUser
	storage.byID[adminUser.ID] = adminUser

	return storage, nil
}

// CreateUser creates a new user
func (us *UserStorage) CreateUser(username, password, email string) (*models.User, error) {
	us.mu.Lock()
	defer us.mu.Unlock()

	// Check if user exists
	if _, exists := us.users[username]; exists {
		return nil, ErrUsernameExists
	}

	passHash, err := hashPassword(password)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user := &models.User{
		ID:        utils.GenerateID(),
		Username:  username,
		Password:  passHash,
		Email:     email,
		Role:      models.RoleUser,
		CreatedAt: time.Now().Unix(),
	}

	us.users[username] = user
	us.byID[user.ID] = user

	return cloneUser(user), nil
}

// GetUser retrieves a user by username
func (us *UserStorage) GetUser(username string) (*models.User, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	user, exists := us.users[username]
	if !exists {
		return nil, ErrUserNotFound
	}

	return cloneUser(user), nil
}

// GetUserByID retrieves a user by ID
func (us *UserStorage) GetUserByID(id string) (*models.User, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	user, exists := us.byID[id]
	if !exists {
		return nil, ErrUserNotFound
	}

	return cloneUser(user), nil
}

// VerifyPassword checks if password matches using bcrypt
func (us *UserStorage) VerifyPassword(username, password string) (bool, error) {
	user, err := us.GetUser(username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Run a real bcrypt comparison against a fixed dummy hash so a
			// missing username does not reveal itself through faster response
			// times (timing-based username enumeration resistance).
			_ = bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash()), []byte(password))
		}
		return false, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return false, nil
	}
	return true, nil
}

// UpdatePassword replaces a user's bcrypt password hash.
func (us *UserStorage) UpdatePassword(userID, password string) error {
	passwordHash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	us.mu.Lock()
	defer us.mu.Unlock()
	user, exists := us.byID[userID]
	if !exists {
		return ErrUserNotFound
	}
	user.Password = passwordHash
	user.TokenVersion++
	return nil
}

// RevokeTokens invalidates every token previously issued for a user.
func (us *UserStorage) RevokeTokens(userID string) error {
	us.mu.Lock()
	defer us.mu.Unlock()
	user, exists := us.byID[userID]
	if !exists {
		return ErrUserNotFound
	}
	user.TokenVersion++
	return nil
}

// UpdateUserRole updates a user's role (admin only)
func (us *UserStorage) UpdateUserRole(username, role string) error {
	us.mu.Lock()
	defer us.mu.Unlock()

	user, exists := us.users[username]
	if !exists {
		return ErrUserNotFound
	}

	if role != models.RoleAdmin && role != models.RoleUser {
		return ErrInvalidRole
	}

	if user.Role == models.RoleAdmin && role != models.RoleAdmin && us.adminCountLocked() <= 1 {
		return ErrLastAdmin
	}

	user.Role = role
	return nil
}

// GetAllUsers returns all users (admin only)
func (us *UserStorage) GetAllUsers() ([]*models.User, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	users := make([]*models.User, 0, len(us.byID))
	for _, user := range us.byID {
		users = append(users, cloneUser(user))
	}

	return users, nil
}

// ListUsers returns a single keyset page (username ASC) of all users. The
// in-memory store scans the map, but keeps the same keyset semantics as the
// Postgres backend so cursors are portable between stores.
func (us *UserStorage) ListUsers(cursor models.UserCursor, limit int) (UserListResult, error) {
	us.mu.RLock()
	defer us.mu.RUnlock()

	all := make([]*models.User, 0, len(us.byID))
	for _, user := range us.byID {
		all = append(all, cloneUser(user))
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Username < all[j].Username })

	total := len(all)
	start := 0
	if cursor.Username != "" {
		start = sort.Search(total, func(i int) bool { return all[i].Username > cursor.Username })
	}
	if start >= total {
		return UserListResult{Items: []*models.User{}, Total: total, HasMore: false}, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	return UserListResult{Items: all[start:end], Total: total, HasMore: end < total}, nil
}

// DeleteUser deletes a user
func (us *UserStorage) DeleteUser(username string) error {
	us.mu.Lock()
	defer us.mu.Unlock()

	user, exists := us.users[username]
	if !exists {
		return ErrUserNotFound
	}

	if user.Role == models.RoleAdmin && us.adminCountLocked() <= 1 {
		return ErrLastAdmin
	}

	delete(us.users, username)
	delete(us.byID, user.ID)

	return nil
}

// hashPassword hashes a password using bcrypt
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// dummyPasswordHash is a pre-computed bcrypt hash used for timing equalization
// when a username does not exist (see VerifyPassword).
var dummyPasswordHash = sync.OnceValue(func() string {
	hash, err := bcrypt.GenerateFromPassword(
		[]byte("goshorty-dummy-password-placeholder"),
		bcrypt.DefaultCost,
	)
	if err != nil {
		panic("failed to precompute dummy password hash: " + err.Error())
	}
	return string(hash)
})

func (us *UserStorage) adminCountLocked() int {
	count := 0
	for _, user := range us.users {
		if user.Role == models.RoleAdmin {
			count++
		}
	}
	return count
}

func cloneUser(user *models.User) *models.User {
	if user == nil {
		return nil
	}
	cloned := *user
	return &cloned
}
