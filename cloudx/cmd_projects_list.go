package cloudx

import (
	"github.com/spf13/cobra"

	"github.com/ory/x/cmdx"
)

func NewProjectsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			h, err := NewSnakeCharmer(cmd)
			if err != nil {
				return err
			}

			projects, err := h.ListProjects()
			if err != nil {
				return err
			}

			cmdx.PrintTable(cmd, &outputProjectCollection{projects})
			return nil
		},
	}

	cmdx.RegisterFormatFlags(cmd.Flags())
	return cmd
}
