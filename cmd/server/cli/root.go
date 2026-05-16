package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/juanfont/headscale-v2/internal/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

func init() {
	if len(os.Args) > 1 &&
		(os.Args[1] == "version" || os.Args[1] == "completion") {
		return
	}

	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().
		StringVarP(&cfgFile, "config", "c", "", "config file (default is /etc/headscale/config.yaml)")
	rootCmd.PersistentFlags().
		StringP("output", "o", "", "Output format. Empty for human-readable, 'json', 'json-line' or 'yaml'")
	rootCmd.PersistentFlags().
		Bool("force", false, "Disable prompts and forces the execution")

	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		cmd.SilenceUsage = false
		return err
	})
}

func initConfig() {
	if cfgFile == "" {
		cfgFile = os.Getenv("HEADSCALE_CONFIG")
	}

	var isFile bool
	if cfgFile != "" {
		isFile = true
	}

	if err := types.LoadConfig(cfgFile, isFile); err != nil {
		log.Fatalf("error loading config file %s: %v", cfgFile, err)
	}
}

func hasMachineOutputFlag() bool {
	for _, arg := range os.Args {
		if arg == "--output=json" || arg == "--output=json-line" || arg == "--output=yaml" || arg == "-ojson" || arg == "-ojson-line" || arg == "-oyaml" {
			return true
		}
	}
	return false
}

var rootCmd = &cobra.Command{
	Use:   "headscale",
	Short: "headscale - a Tailscale control server",
	Long: `
headscale is an open source implementation of the Tailscale control server

https://github.com/juanfont/headscale`,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func Execute() {
	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		outputFormat, _ := cmd.Flags().GetString("output")
		printError(err, outputFormat)
		os.Exit(1)
	}
}

func printError(err error, outputFormat string) {
	switch outputFormat {
	case "json", "json-line":
		fmt.Printf("{\"error\": %q}\n", err.Error())
	case "yaml":
		fmt.Printf("error: %q\n", err.Error())
	default:
		fmt.Printf("Error: %v\n", err)
	}
}

var prereleases = []string{"alpha", "beta", "rc", "dev"}

func isPreReleaseVersion(version string) bool {
	for _, unstable := range prereleases {
		if strings.Contains(version, unstable) {
			return true
		}
	}
	return false
}

func filterPreReleasesIfStable(versionFunc func() string) func(string) bool {
	return func(tag string) bool {
		version := versionFunc()

		if isPreReleaseVersion(version) {
			return false
		}

		for _, ignore := range prereleases {
			if strings.Contains(tag, ignore) {
				return true
			}
		}

		return false
	}
}

func checkForUpdate() {
	disableUpdateCheck := viper.GetBool("disable_check_updates")
	if disableUpdateCheck || hasMachineOutputFlag() {
		return
	}

	versionInfo := types.GetVersionInfo()
	if (runtime.GOOS == "linux" || runtime.GOOS == "darwin") &&
		!versionInfo.Dirty {
	}
}

func GetRootCmd() *cobra.Command {
	return rootCmd
}
