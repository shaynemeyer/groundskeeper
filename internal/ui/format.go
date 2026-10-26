// Package ui holds formatting and terminal styles shared by every command.
package ui

import "fmt"

// Bytes formats n in decimal units, as Finder does: "12.4 GB".
func Bytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value, exp := float64(n)/unit, 0
	for value >= unit && exp < 4 {
		value /= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", value, "kMGTP"[exp])
}
