package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Prints the bunshin-go version and exits.",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("bunshin-go %s\n", version)
		},
	}
}
