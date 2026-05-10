//go:build !prod

package redacted

import "fmt"

// Redacted redacts sensitive data in production logs.
// In non-production environments, it returns the string representation of the input value.
// In a production environment, it always returns the constant "[REDACTED]" to ensure sensitive information is not exposed.
func Redacted(input any) string {
	return fmt.Sprint(input)
}
