package gincmd

import (
	ginclient "github.com/G-Node/gin-cli/ginclient"
	"github.com/G-Node/gin-cli/git"
	"github.com/G-Node/gin-cli/util"
	"github.com/spf13/cobra"
)

func remove(cmd *cobra.Command, args []string) {
	jsonout, _ := cmd.Flags().GetBool("json")
	gincl := ginclient.New(util.Config.GinHost)
	requirelogin(cmd, gincl, !jsonout)
	if !git.IsRepo() {
		util.Die("This command must be run from inside a gin repository.")
	}
	gincl.GitHost = util.Config.GitHost
	gincl.GitUser = util.Config.GitUser
	lockchan := make(chan git.RepoFileStatus)
	go gincl.LockContent(args, lockchan)
	formatOutput(lockchan, jsonout)
	rmchan := make(chan git.RepoFileStatus)
	go gincl.RemoveContent(args, rmchan)
	formatOutput(rmchan, jsonout)
}

// RemoveContentCmd sets up the 'remove-content' subcommand
func RemoveContentCmd() *cobra.Command {
	description := "Remove the content of local files. This command will not remove the content of files that have not been already uploaded to a remote repository, even if the user specifies such files explicitly. Removed content can be retrieved from the server by using the 'get-content' command. With no arguments, removes the content of all files under the current working directory, as long as they have been safely uploaded to a remote repository.\n\nNote that after removal, placeholder files will remain in the local repository. These files appear as 'No Content' when running the 'gin ls' command."
	args := map[string]string{
		"<filenames>": "One or more directories or files to remove.",
	}
	var rmContentCmd = &cobra.Command{
		Use:                   "remove-content [--json] [<filenames>]...",
		Short:                 "Remove the content of local files that have already been uploaded",
		Long:                  formatdesc(description, args),
		Args:                  cobra.ArbitraryArgs,
		Run:                   remove,
		Aliases:               []string{"rmc"},
		DisableFlagsInUseLine: true,
	}
	rmContentCmd.Flags().Bool("json", false, "Print output in JSON format.")
	return rmContentCmd
}
