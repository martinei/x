package cloudx

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewDeployCmd(project string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: fmt.Sprintf("Deploy Ory %[1]s to a new Ory Cloud Project or update an existing one", project),
		Long: fmt.Sprintf(`Deploy Ory %[1]s in seconds to Ory Cloud. This command allows you to both create
a new Ory Cloud Project or update an existing Ory Cloud Project.

To create a new Ory Cloud Project and deploy Ory %[1]s to it, run:

	%[1]s deploy --create --config=<path to config file>

To update an existing Ory Cloud Project, use the --project flag:

	%[1]s deploy --project my-project-slug

If you omit the --project flag, the command will look for existing Ory Cloud Projects
and either prompt to choose one or create a new project.

This command allows you to easily import a configuration (JSON and YAML) from your file system

    %[1]s deploy --config=/path/to/config.yaml
    %[1]s deploy --config=/path/to/config.json
    %[1]s deploy --config=file:///path/to/config.json

a remote source

    %[1]s deploy --config=https://example.org/path/to/config.json
    %[1]s deploy --config=https://example.org/path/to/config.yaml

and base64 encoded (YAML and JSON both supported):

	%[1]s deploy --config=base64://...`, project),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	return cmd
}
