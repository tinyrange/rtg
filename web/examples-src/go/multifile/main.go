package main

import "fmt"

func main() {
	team := []player{
		{name: "Ada", score: 7},
		{name: "Ken", score: 9},
		{name: "Rob", score: 8},
	}

	fmt.Println("average score:", averageScore(team))
}
