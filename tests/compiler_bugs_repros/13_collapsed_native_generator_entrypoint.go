package main

type profile int

const (
	profileDefault profile = iota
	profileTiny
)

func generateWithProfile(p profile) int {
	switch p {
	case profileTiny:
		return 1
	default:
		return 0
	}
}

func main() {
	_, _ = generateWithProfile(profileDefault), generateWithProfile(profileTiny) // Helperized control-flow drift shape.
}
