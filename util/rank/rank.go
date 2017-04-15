package rank

const (
	maskVersion uint32 = 0xFF000000
	maskAdmin   uint32 = 0x00FF0000
	maskMod     uint32 = 0x0000FF00
	maskUser    uint32 = 0x000000FF
)

type (
	// Rank is a bitfield
	Rank uint32
)
