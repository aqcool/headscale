package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(routesCmd)
	routesCmd.AddCommand(listRoutesCmd)
	routesCmd.AddCommand(enableRouteCmd)
	routesCmd.AddCommand(disableRouteCmd)
}

var routesCmd = &cobra.Command{
	Use:   "routes",
	Short: "Manage subnet routes",
	Long:  "Manage subnet routes advertised by nodes.",
}

var listRoutesCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all subnet routes",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.ListNodes(ctx, &v1.ListNodesRequest{})
		if err != nil {
			return fmt.Errorf("listing nodes: %w", err)
		}

		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat == "json" || outputFormat == "json-line" {
			for _, node := range resp.Nodes {
				if len(node.SubnetRoutes) > 0 {
					fmt.Printf("{\"node_id\": %d, \"node_name\": \"%s\", \"routes\": [%s]}\n",
						node.Id, node.GivenName, formatRoutesJSON(node.SubnetRoutes))
				}
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 10, 1, 5, ' ', 0)
		fmt.Fprintf(w, "NODE_ID\tNODE_NAME\tROUTES\n")
		for _, node := range resp.Nodes {
			if len(node.SubnetRoutes) > 0 {
				fmt.Fprintf(w, "%d\t%s\t%s\n",
					node.Id, node.GivenName, strings.Join(node.SubnetRoutes, ", "))
			}
		}
		w.Flush()
		return nil
	},
}

var enableRouteCmd = &cobra.Command{
	Use:   "enable NODE_ID ROUTES...",
	Short: "Enable routes on a node",
	Long:  "Approve subnet routes for a node. Routes should be CIDR prefixes.",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}
		routes := args[1:]

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.SetApprovedRoutes(ctx, &v1.SetApprovedRoutesRequest{
			NodeId: nodeID,
			Routes: routes,
		})
		if err != nil {
			return fmt.Errorf("enabling routes: %w", err)
		}

		fmt.Printf("Routes enabled for node %d: %v\n", resp.Node.Id, resp.Node.SubnetRoutes)
		return nil
	},
}

var disableRouteCmd = &cobra.Command{
	Use:   "disable NODE_ID",
	Short: "Disable all routes on a node",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.SetApprovedRoutes(ctx, &v1.SetApprovedRoutesRequest{
			NodeId: nodeID,
			Routes: []string{},
		})
		if err != nil {
			return fmt.Errorf("disabling routes: %w", err)
		}

		fmt.Printf("All routes disabled for node %d\n", resp.Node.Id)
		return nil
	},
}

func formatRoutesJSON(routes []string) string {
	if len(routes) == 0 {
		return ""
	}
	quoted := make([]string, len(routes))
	for i, r := range routes {
		quoted[i] = fmt.Sprintf("\"%s\"", r)
	}
	return strings.Join(quoted, ", ")
}