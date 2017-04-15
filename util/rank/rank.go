package rank

const (
	maskVersion uint32 = 0xFF000000
	maskAdmin   uint32 = 0x00FF0000
	maskMod     uint32 = 0x0000FF00
	maskUser    uint32 = 0x000000FF
)

// BaseUser creates a new user rank
func BaseUser() uint32 {
	return 0x01000001
}

// Admin creates a new Administrator rank
func Admin() uint32 {
	return 0x010F0001
}
