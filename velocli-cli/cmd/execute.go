package cmd

import (
	"errors"
	"fmt"
	"os"
)

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		msg := err.Error()
		var e interface{ Unwrap() error }
		if errors.As(err, &e) && e.Unwrap() != nil {
			msg = e.Unwrap().Error()
		}
		_, _ = fmt.Fprintln(os.Stderr, msg)
		os.Exit(1)
	}
}
