// Package num holds small numeric helpers shared across packages.
package num

import "math"

// Round rounds v to the given number of decimal places (half away from zero).
func Round(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}
