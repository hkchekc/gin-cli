package gincmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	ginclient "github.com/G-Node/gin-cli/ginclient"
	"github.com/G-Node/gin-cli/ginclient/log"
	"github.com/G-Node/gin-cli/git"
	"github.com/bbrks/wrap"
	"github.com/docker/docker/pkg/term"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

const unknownhostname = "(unknown)"

var green = color.New(color.FgGreen).SprintFunc()
var red = color.New(color.FgRed).SprintFunc()

// Die prints an error message to stderr and exits the program with status 1.
func Die(msg interface{}) {
	msgstring := fmt.Sprintf("%s", msg)
	if len(msgstring) > 0 {
		log.Write("Exiting with ERROR message: %s", msgstring)
		fmt.Fprintf(color.Error, "%s %s\n", red("[error]"), msgstring)
	} else {
		log.Write("Exiting with ERROR (no message)")
	}
	log.Close()
	os.Exit(1)
}

// Exit prints a message to stdout and exits the program with status 0.
func Exit(msg string) {
	if len(msg) > 0 {
		log.Write("Exiting with message: %s", msg)
		fmt.Println(msg)
	} else {
		log.Write("Exiting")
	}
	log.Close()
	os.Exit(0)
}

// CheckError exits the program if an error is passed to the function.
// The error message is checked for known error messages and an informative message is printed.
// Otherwise, the error message is printed to stderr.
func CheckError(err error) {
	if err != nil {
		log.Write(err.Error())
		if strings.Contains(err.Error(), "Error loading user token") {
			Die("This operation requires login.")
		}
		Die(err)
	}
}

// CheckErrorMsg exits the program if an error is passed to the function.
// Before exiting, the given msg string is printed to stderr.
func CheckErrorMsg(err error, msg string) {
	if err != nil {
		log.Write("The following error occurred:\n%sExiting with message: %s", err, msg)
		Die(msg)
	}
}

// requirelogin prompts for login if the user is not already logged in.
// It only checks if a local token exists and does not confirm its validity with the server.
// The function should be called at the start of any command that requires being logged in to run.
func requirelogin(cmd *cobra.Command, gincl *ginclient.Client, prompt bool) {
	err := gincl.LoadToken()
	if prompt {
		if err != nil {
			login(cmd, nil)
		}
		err = gincl.LoadToken()
	}
	CheckError(err)
}

func usageDie(cmd *cobra.Command) {
	cmd.Help()
	// exit without message
	Die("")
}

func printJSON(statuschan <-chan git.RepoFileStatus) (filesuccess map[string]bool) {
	filesuccess = make(map[string]bool)
	for stat := range statuschan {
		j, _ := json.Marshal(stat)
		fmt.Println(string(j))
		filesuccess[stat.FileName] = true
		if stat.Err != nil {
			filesuccess[stat.FileName] = false
		}
	}
	return
}

func printProgressWithBar(statuschan <-chan git.RepoFileStatus, nitems int) (filesuccess map[string]bool) {
	if nitems <= 0 {
		// If nitems is invalid, just print the classic progress output
		return printProgressOutput(statuschan)
	}
	ndigits := len(fmt.Sprintf("%d", nitems))
	dfmt := fmt.Sprintf("%%%dd/%%%dd", ndigits, ndigits)
	filesuccess = make(map[string]bool)
	var barratio float64
	var ncomplt int
	linewidth := termwidth()
	if linewidth > 80 {
		linewidth = 80
	}
	barwidth := linewidth - (5 + ndigits*2)
	barratio = float64(barwidth) / float64(nitems)
	outline := new(bytes.Buffer)
	outappend := func(part string) {
		if len(part) > 0 {
			outline.WriteString(part)
			outline.WriteString(" ")
		}
	}
	for stat := range statuschan {
		ncomplt++
		outline.Reset()
		outline.WriteString(" ")
		outappend(stat.State)
		outappend(stat.FileName)
		if stat.Err == nil {
			if stat.Progress == "100%" {
				outappend(green("OK"))
				filesuccess[stat.FileName] = true
			}
		} else {
			outappend(stat.Err.Error())
			filesuccess[stat.FileName] = false
		}
		newprint := outline.String()
		fmt.Printf("\r%s\r", strings.Repeat(" ", linewidth)) // clear the line
		fmt.Fprint(color.Output, newprint)
		complsigns := int(math.Floor(float64(ncomplt) * barratio))
		blocks := strings.Repeat("=", complsigns)
		blanks := strings.Repeat(" ", barwidth-complsigns)
		dprg := fmt.Sprintf(dfmt, ncomplt, nitems)
		fmt.Printf("\n [%s%s] %s\r", blocks, blanks, dprg)
	}
	if outline.Len() > 0 {
		fmt.Println()
	}
	return
}

func printProgressOutput(statuschan <-chan git.RepoFileStatus) (filesuccess map[string]bool) {
	filesuccess = make(map[string]bool)
	var fname, state string
	var lastprint string
	outline := new(bytes.Buffer)
	outappend := func(part string) {
		if len(part) > 0 {
			outline.WriteString(part)
			outline.WriteString(" ")
		}
	}
	for stat := range statuschan {
		outline.Reset()
		outline.WriteString(" ")
		if stat.FileName != fname || stat.State != state {
			// New line if new file or new state
			if len(lastprint) > 0 {
				fmt.Println()
			}
			lastprint = ""
			fname = stat.FileName
			state = stat.State
		}
		outappend(stat.State)
		outappend(stat.FileName)
		if stat.Err == nil {
			if stat.Progress == "100%" {
				outappend(green("OK"))
				filesuccess[stat.FileName] = true
			} else {
				outappend(stat.Progress)
				outappend(stat.Rate)
			}
		} else {
			outappend(stat.Err.Error())
			filesuccess[stat.FileName] = false
		}
		newprint := outline.String()
		if newprint != lastprint {
			fmt.Printf("\r%s\r", strings.Repeat(" ", len(lastprint))) // clear the line
			fmt.Fprint(color.Output, newprint)
			fmt.Print("\r")
			lastprint = newprint
		}
	}
	if len(lastprint) > 0 {
		fmt.Println()
	}
	return
}

func formatOutput(statuschan <-chan git.RepoFileStatus, nitems int, jsonout bool) {
	// TODO: instead of a true/false success, add an error for every file and then group the errors by type and print a report
	var filesuccess map[string]bool
	if jsonout {
		filesuccess = printJSON(statuschan)
	} else if nitems > 0 {
		filesuccess = printProgressWithBar(statuschan, nitems)
	} else {
		filesuccess = printProgressOutput(statuschan)
	}

	// count unique file errors
	nerrors := 0
	for _, stat := range filesuccess {
		if !stat {
			nerrors++
		}
	}
	if nerrors > 0 {
		// Exit with error message and failed exit status
		var plural string
		if nerrors > 1 {
			plural = "s"
		}
		Die(fmt.Sprintf("%d operation%s failed", nerrors, plural))
	}
}

var wouter = wrap.NewWrapper()
var winner = wrap.NewWrapper()

func termwidth() int {
	width := 80
	if ws, err := term.GetWinsize(0); err == nil {
		width = int(ws.Width)
	}
	return width - 1
}

func formatdesc(desc string, args map[string]string) (fdescription string) {
	width := termwidth()
	wouter.OutputLinePrefix = "  "
	winner.OutputLinePrefix = "    "

	if len(desc) > 0 {
		fdescription = fmt.Sprintf("Description:\n\n%s", wouter.Wrap(desc, width))
	}

	if args != nil {
		argsdesc := fmt.Sprintf("Arguments:\n\n")
		for a, d := range args {
			argsdesc = fmt.Sprintf("%s%s%s\n", argsdesc, wouter.Wrap(a, width), winner.Wrap(d, width))
		}
		fdescription = fmt.Sprintf("%s\n%s", fdescription, argsdesc)
	}
	return
}

func formatexamples(examples map[string]string) (exdesc string) {
	width := termwidth()
	if examples != nil {
		for d, ex := range examples {
			exdesc = fmt.Sprintf("%s\n%s%s", exdesc, wouter.Wrap(d, width), winner.Wrap(ex, width))
		}
	}
	return
}

// SetUpCommands sets up all the subcommands for the client and returns the root command, ready to execute.
func SetUpCommands(verstr string) *cobra.Command {
	var rootCmd = &cobra.Command{
		Use:                   "gin",
		Long:                  "GIN Command Line Interface and client for the GIN services", // TODO: Add license and web info
		Version:               fmt.Sprintln(verstr),
		DisableFlagsInUseLine: true,
	}
	cobra.AddTemplateFunc("wrappedFlagUsages", wrappedFlagUsages)
	rootCmd.SetHelpTemplate(helpTemplate)
	rootCmd.SetUsageTemplate(usageTemplate)

	// Login
	rootCmd.AddCommand(LoginCmd())

	// Logout
	rootCmd.AddCommand(LogoutCmd())

	// Init repo
	rootCmd.AddCommand(InitCmd())

	// New remote
	rootCmd.AddCommand(AddServerCmd())

	// Add remote
	rootCmd.AddCommand(AddRemoteCmd())

	// Remove remote
	rootCmd.AddCommand(RemoveRemoteCmd())

	// Use remote
	rootCmd.AddCommand(UseRemoteCmd())

	// Remotes
	rootCmd.AddCommand(RemotesCmd())

	// Create repo
	rootCmd.AddCommand(CreateCmd())

	// Delete repo (unlisted)
	rootCmd.AddCommand(DeleteCmd())

	// Get repo
	rootCmd.AddCommand(GetCmd())

	// List files
	rootCmd.AddCommand(LsRepoCmd())

	// Unlock content
	rootCmd.AddCommand(UnlockCmd())

	// Lock content
	rootCmd.AddCommand(LockCmd())

	// Commit changes
	rootCmd.AddCommand(CommitCmd())

	// Upload
	rootCmd.AddCommand(UploadCmd())

	// Download
	rootCmd.AddCommand(DownloadCmd())

	// Get content
	rootCmd.AddCommand(GetContentCmd())

	// Remove content
	rootCmd.AddCommand(RemoveContentCmd())

	// Account info
	rootCmd.AddCommand(InfoCmd())

	// List repos
	rootCmd.AddCommand(ReposCmd())

	// Repo info
	rootCmd.AddCommand(RepoInfoCmd())

	// Keys
	rootCmd.AddCommand(KeysCmd())

	// Version
	rootCmd.AddCommand(VersionCmd())

	// git and annex passthrough (unlisted)
	rootCmd.AddCommand(GitCmd())
	rootCmd.AddCommand(AnnexCmd())

	return rootCmd
}