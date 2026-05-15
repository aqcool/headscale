package cli

import (
	"fmt"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(debugCmd)
	debugCmd.AddCommand(debugCreateNodeCmd)
}

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug commands",
	Long:  "Debug and development commands for headscale.",
}

var debugCreateNodeCmd = &cobra.Command{
	Use:   "create-node USER_NAME NODE_NAME",
	Short: "Create a test node",
	Long:  "Create a test node for debugging purposes. Not for production use.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		userName := args[0]
		nodeName := args[1]

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.DebugCreateNode(ctx, &v1.DebugCreateNodeRequest{
			User: userName,
			Name: nodeName,
		})
		if err != nil {
			return fmt.Errorf("creating debug node: %w", err)
		}

		fmt.Printf("Debug node created: ID=%d, Name=%s\n", resp.Node.Id, resp.Node.GivenName)
		return nil
	},
}
