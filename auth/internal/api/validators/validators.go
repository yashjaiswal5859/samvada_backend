package validators

import (
	"regexp"

	"github.com/go-playground/validator/v10"
)

var (
	deviceIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

var ValidateDeviceID validator.Func = func(fl validator.FieldLevel) bool {
	return deviceIDRegex.MatchString(fl.Field().String())
}
