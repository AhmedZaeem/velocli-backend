package cmd

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/velocli/velocli/velocli-cli/internal/api"
	"github.com/velocli/velocli/velocli-cli/internal/config"
	"github.com/velocli/velocli/velocli-cli/internal/tui"
)

func NewLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate using your Lemon Squeezy license key",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := config.NewStore()
			if err != nil {
				return err
			}

			baseURL := strings.TrimSpace(os.Getenv("VELOCLI_BACKEND_URL"))
			if baseURL == "" {
				baseURL = store.BackendURL()
			}

			client := api.NewClient(baseURL)
			res, err := tui.RunLogin(tui.LoginDeps{
				LoginFunc: client.Login,
			})
			if err != nil {
				return err
			}

			if err := store.SaveToken(res.Token, time.Now().UTC()); err != nil {
				return err
			}

			_, _ = cmd.OutOrStdout().Write([]byte("Logged in\n"))
			return nil
		},
	}
	return cmd
}

