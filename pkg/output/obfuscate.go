package output

import "strings"

// Arbitrary names, IPs, and namespaces are fully masked. Only recognized AWS ID
// types retain their fixed type prefix and four trailing hex characters.
func obfuscate(id string) string {
	for _, prefix := range []string{"vol-", "eipalloc-", "i-"} {
		suffix, ok := strings.CutPrefix(id, prefix)
		if !ok || len(suffix) < 8 {
			continue
		}
		valid := true
		for _, ch := range suffix {
			if !strings.ContainsRune("0123456789abcdef", ch) {
				valid = false
				break
			}
		}
		if valid {
			return prefix + "***" + suffix[len(suffix)-4:]
		}
	}
	return "***"
}
