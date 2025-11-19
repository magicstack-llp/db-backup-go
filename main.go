package main

import (
	cli "github.com/magicstack-llp/db-backup-go/interface"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := cli.NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		cobra.CheckErr(err)
	}
}

