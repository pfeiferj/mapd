package cli

import (
	"fmt"

	"github.com/manifoldco/promptui"
)

func interactive() {
	prompt := promptui.Select{
		Label: "Select Action",
		Items: []string{"Settings", "Download Maps"},
	}

	_, result, err := prompt.Run()

	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return
	}

	switch result {
	case "Settings":
		settings()
	}

}
