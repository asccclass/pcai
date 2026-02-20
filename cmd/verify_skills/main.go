package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/asccclass/pcai/internal/skillloader"
)

func main() {
	home, _ := os.Getwd()
	skillsDir := filepath.Join(home, "skills")
	fmt.Printf("Loading skills from: %s\n", skillsDir)

	loadedSkills, err := skillloader.LoadSkills(skillsDir)
	if err != nil {
		log.Fatalf("Failed to load skills: %v", err)
	}

	fmt.Printf("Loaded %d skills:\n", len(loadedSkills))
	for _, s := range loadedSkills {
		fmt.Printf("- Name: %s\n", s.Name)
		fmt.Printf("  Desc: %s\n", s.Description)
		fmt.Printf("  Cmd : %s\n", s.Command)
		fmt.Printf("  Params: %v\n", s.Params)
		fmt.Println("---")
	}
}
