package util

import (
	"net"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

func ValidateUnique(fl validator.FieldLevel) bool {
	field := fl.Field()
	kind := field.Kind()

	if kind != reflect.Slice && kind != reflect.Array {
		// This tag should only be used on slices/arrays
		return false
	}

	seen := make(map[interface{}]struct{})

	for i := 0; i < field.Len(); i++ {
		element := field.Index(i).Interface()

		// Check if we've already seen this element
		if _, ok := seen[element]; ok {
			return false // Found a duplicate
		}

		// Mark the element as seen. Since map keys must be comparable,
		// this works for strings, integers, and other basic types.
		seen[element] = struct{}{}
	}

	return true // No duplicates found
}

func ValidateEnum(fl validator.FieldLevel) bool {
	// 1. Prepare the set of allowed values from the tag parameter
	param := fl.Param()
	if param == "" {
		return false // Parameter is required for 'enum'
	}
	// Create a map where keys are the allowed string values.
	allowedValues := make(map[string]struct{})
	for _, val := range strings.Split(param, "#") {
		allowedValues[val] = struct{}{}
	}

	// 2. Determine the field type and value
	field := fl.Field()
	kind := field.Kind()

	// --- Helper function to check a single value ---
	checkSingleValue := func(v reflect.Value) bool {
		switch v.Kind() {
		case reflect.String:
			// Case: string field
			if _, exists := allowedValues[v.String()]; exists {
				return true
			}
			return false

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			// Case: int-like field
			valStr := strconv.FormatInt(v.Int(), 10) // Convert int value to its string representation
			if _, exists := allowedValues[valStr]; exists {
				return true
			}
			return false
		default:
			// Unsupported scalar type
			return false
		}
	}

	// --- Check based on Field Kind ---
	switch kind {
	case reflect.Slice, reflect.Array:
		// Case: Array/Slice of strings or ints
		for i := 0; i < field.Len(); i++ {
			if !checkSingleValue(field.Index(i)) {
				return false // Array validation fails if any element is invalid
			}
		}
		return true // All elements in the array/slice are valid

	case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// Case: Single scalar value (string or int-like)
		return checkSingleValue(field)

	default:
		// Other unsupported types (e.g., map, struct)
		return false
	}
}

func ValidateNotBlank(fl validator.FieldLevel) bool {
	return len(strings.TrimSpace(fl.Field().String())) > 0
}

func ValidateHostPortList(fl validator.FieldLevel) bool {
	brokers, ok := fl.Field().Interface().([]string)
	if !ok {
		return false
	}

	for _, broker := range brokers {
		if !isValidHostPort(broker) {
			return false
		}
	}

	return true
}

func isValidHostPort(s string) bool {
	host, port, err := net.SplitHostPort(s)
	if err == nil {
		if host == "" || !isValidPort(port) {
			return false
		}
		return true
	}

	if net.ParseIP(s) != nil {
		return true
	}

	if s != "" {
		return true
	}

	return false
}

func isValidPort(portStr string) bool {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return false
	}
	return port > 0 && port <= 65535
}
