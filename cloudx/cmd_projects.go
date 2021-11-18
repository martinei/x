package cloudx

import "github.com/spf13/cobra"

func NewProjectsCmd(project string) *cobra.Command {
	cmd := &cobra.Command{
		Use: "projects",
	}
	cmd.AddCommand(NewProjectsListCmd())
	cmd.AddCommand(NewProjectsCreateCmd(project))
	RegisterFlags(cmd.PersistentFlags())
	return cmd
}
