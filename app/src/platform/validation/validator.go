package validation

import "github.com/go-playground/validator/v10"

var Instance = validator.New(validator.WithRequiredStructEnabled())
