package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate using your Lemon Squeezy license key",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "login is not wired yet in this repo snapshot")
			return nil
		},
	}
	return cmd
}
