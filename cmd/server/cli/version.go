package cli

import (
	"fmt"
	"runtime"

	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of headscale",
	Long:  "Print the version of headscale along with Go version and build information.",
	Run: func(cmd *cobra.Command, args []string) {
		outputFormat, _ := cmd.Flags().GetString("output")

		versionInfo := types.GetVersionInfo()

		switch outputFormat {
		case "json", "json-line":
			fmt.Printf("{\"version\": \"%s\", \"go_version\": \"%s\", \"os\": \"%s\", \"arch\": \"%s\", \"dirty\": %v}\n",
				versionInfo.Version,
				runtime.Version(),
				runtime.GOOS,
				runtime.GOARCH,
				versionInfo.Dirty)
		case "yaml":
			fmt.Printf("version: %s\n", versionInfo.Version)
			fmt.Printf("go_version: %s\n", runtime.Version())
			fmt.Printf("os: %s\n", runtime.GOOS)
			fmt.Printf("arch: %s\n", runtime.GOARCH)
			fmt.Printf("dirty: %v\n", versionInfo.Dirty)
		default:
			fmt.Printf("headscale version: %s\n", versionInfo.Version)
			fmt.Printf("Go version: %s\n", runtime.Version())
			fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
			if versionInfo.Dirty {
				fmt.Printf("Build: dirty (uncommitted changes)\n")
			}
		}
	},
}
