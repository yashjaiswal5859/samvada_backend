package user

import "errors"

var (
	ErrUserNotFound          = errors.New("user not found")
	ErrEmailAlreadyExists    = errors.New("email already exists")
	ErrKratosIDAlreadyExists = errors.New("kratos id already exists")
)
