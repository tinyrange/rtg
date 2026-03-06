package main

type player struct {
	name  string
	score int
}

func averageScore(players []player) int {
	total := 0
	for _, p := range players {
		total += p.score
	}
	return total / len(players)
}
