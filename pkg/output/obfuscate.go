package output

func Obfuscate(id string) string {
	if len(id) <= 8 {
		return "***"
	}

	// Find prefix (everything up to and including the first dash)
	dashIdx := -1
	for i, ch := range id {
		if ch == '-' {
			dashIdx = i
			break
		}
	}

	suffix := id[len(id)-4:]

	if dashIdx >= 0 {
		return id[:dashIdx+1] + "***" + suffix
	}

	return "***" + suffix
}
