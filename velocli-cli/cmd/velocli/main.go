package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/velocli/velocli/velocli-cli/internal/commands"
)

func main() {
	root := &cobra.Command{
		Use:   "velocli",
		Short: "Scaffold premium Flutter projects from encrypted bricks",
	}

	root.AddCommand(
		commands.NewStartCmd(),
		commands.NewCreateCmd(),
		commands.NewLoginCmd(),
		commands.NewScreenshotCmd(),
		commands.NewCompletionCmd(root),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}