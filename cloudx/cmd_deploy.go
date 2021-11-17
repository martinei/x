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
a new Ory Cloud Project or update an existing Ory Cloud Project. You can set the
Ory Cloud Project using the --project flag:

	%[1]s deploy --project my-project-slug

If you do not have a project yet, this command will create one for you for free with
a default configuration and set it as the default project on your local machine.

If you do not have a project yet set in your context, but you do have Ory Cloud Projects
in your account, this command will prompt you to choose one and will also ask you
if you want to set it as the default project on your local machine.

This command allows you to easily import a configuration (JSON and YAML) from your file system

    %[1]s deploy /path/to/config.yaml
    %[1]s deploy /path/to/config.json
    %[1]s deploy file:///path/to/config.json

a remote source

    %[1]s deploy https://example.org/path/to/config.json
    %[1]s deploy https://example.org/path/to/config.yaml

and base64 encoded JSON:

	%[1]s deploy base64://...

`, project),
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}


	return cmd
}
