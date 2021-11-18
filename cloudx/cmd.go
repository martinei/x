package cloudx

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func NewRootCommand(project string, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloud",
		Short: fmt.Sprintf("Run and manage Ory %s in Ory Cloud", project),
	}

	cmdName := strings.ToLower(project + " cloud")

	cmd.AddCommand(NewAuthCmd())
	cmd.AddCommand(NewAuthLogoutCmd())
	cmd.AddCommand(NewProxyCommand(project, cmdName, version))
	cmd.AddCommand(NewTunnelCommand(project, cmdName, version))
	cmd.AddCommand(NewDeployCmd(project))
	cmd.AddCommand(NewProjectsCmd(project))
	return cmd
}
