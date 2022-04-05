package service

import (
	"github.com/urfave/cli"

	"github.com/rocket-pool/smartnode/shared/utils/api"
	cliutils "github.com/rocket-pool/smartnode/shared/utils/cli"
)

// Register subcommands
func RegisterSubcommands(command *cli.Command, name string, aliases []string) {
	command.Subcommands = append(command.Subcommands, cli.Command{
		Name:    name,
		Aliases: aliases,
		Usage:   "Manage the Rocket Pool deposit queue",
		Subcommands: []cli.Command{
			{
				Name:      "terminate-data-folder",
				Aliases:   []string{"t"},
				Usage:     "Deletes the data folder including the wallet file, password file, and all validator keys - don't use this unless you have a very good reason to do it (such as switching from Prater to Mainnet)",
				UsageText: "rocketpool api service terminate-data-folder",
				Action: func(c *cli.Context) error {

					// Validate args
					if err := cliutils.ValidateArgCount(c, 0); err != nil {
						return err
					}

					// Run
					api.PrintResponse(terminateDataFolder(c))
					return nil

				},
			},

			{
				Name:      "create-fee-recipient-file",
				Aliases:   []string{"f"},
				Usage:     "Creates the fee recipient file for the validator container",
				UsageText: "rocketpool api service create-fee-recipient-file",
				Action: func(c *cli.Context) error {

					// Validate args
					if err := cliutils.ValidateArgCount(c, 0); err != nil {
						return err
					}

					// Run
					api.PrintResponse(createFeeRecipientFile(c))
					return nil

				},
			},
		},
	})
}
