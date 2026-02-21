package main

import (
	"log"
	"os"

	"connector/enroll"
	"connector/run"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("missing command: enroll | run")
	}

	switch os.Args[1] {
	case "enroll":
		if err := enroll.Run(); err != nil {
			log.Fatalf("enrollment failed: %v", err)
		}
		log.Println("enrollment completed successfully")

	case "run":
		if err := run.Run(); err != nil {
			log.Fatalf("connector run failed: %v", err)
		}

	default:
		log.Fatalf("unknown command: %s", os.Args[1])
	}
}
