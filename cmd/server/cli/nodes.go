package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func init() {
	rootCmd.AddCommand(nodesCmd)
	nodesCmd.AddCommand(listNodesCmd)
	nodesCmd.AddCommand(deleteNodeCmd)
	nodesCmd.AddCommand(expireNodeCmd)
	nodesCmd.AddCommand(renameNodeCmd)
	nodesCmd.AddCommand(setTagsCmd)
	nodesCmd.AddCommand(setRoutesCmd)
}

var nodesCmd = &cobra.Command{
	Use:   "nodes",
	Short: "Manage nodes",
	Long:  "Manage the nodes (devices) registered with headscale.",
}

var listNodesCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all nodes",
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
				fmt.Printf("{\"id\": %d, \"name\": \"%s\", \"given_name\": \"%s\", \"user\": \"%s\", \"ip_addresses\": [%s], \"online\": %v}\n",
					node.Id, node.Name, node.GivenName,
					node.User.Name,
					formatIPsJSON(node.IpAddresses),
					node.Online)
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 10, 1, 5, ' ', 0)
		fmt.Fprintf(w, "ID\tNAME\tGIVEN NAME\tUSER\tIP ADDRESS\tONLINE\tEXPIRY\n")
		for _, node := range resp.Nodes {
			expiry := ""
			if node.Expiry != nil {
				if node.Expiry.AsTime().Before(time.Now()) {
					expiry = "EXPIRED"
				} else {
					expiry = node.Expiry.AsTime().Format("2006-01-02 15:04:05")
				}
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%v\t%s\n",
				node.Id, node.Name, node.GivenName,
				node.User.Name,
				strings.Join(node.IpAddresses, ","),
				node.Online,
				expiry)
		}
		w.Flush()
		return nil
	},
}

var deleteNodeCmd = &cobra.Command{
	Use:   "delete NODE_ID",
	Short: "Delete a node",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Are you sure you want to delete node %d? Use --force to skip confirmation\n", nodeID)
			return nil
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		_, err = client.DeleteNode(ctx, &v1.DeleteNodeRequest{NodeId: nodeID})
		if err != nil {
			return fmt.Errorf("deleting node: %w", err)
		}

		fmt.Printf("Node %d deleted\n", nodeID)
		return nil
	},
}

var expireNodeCmd = &cobra.Command{
	Use:   "expire NODE_ID",
	Short: "Expire a node immediately",
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
		resp, err := client.ExpireNode(ctx, &v1.ExpireNodeRequest{NodeId: nodeID})
		if err != nil {
			return fmt.Errorf("expiring node: %w", err)
		}

		fmt.Printf("Node %d (%s) expired\n", resp.Node.Id, resp.Node.GivenName)
		return nil
	},
}

var renameNodeCmd = &cobra.Command{
	Use:   "rename NODE_ID NEW_NAME",
	Short: "Rename a node",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}
		newName := args[1]

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.RenameNode(ctx, &v1.RenameNodeRequest{NodeId: nodeID, NewName: newName})
		if err != nil {
			return fmt.Errorf("renaming node: %w", err)
		}

		fmt.Printf("Node %d renamed to %s\n", resp.Node.Id, resp.Node.GivenName)
		return nil
	},
}

var setTagsCmd = &cobra.Command{
	Use:   "set-tags NODE_ID TAGS...",
	Short: "Set tags on a node",
	Long:  "Set tags on a node. Tags must start with 'tag:' and be lowercase.",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeID, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid node ID: %w", err)
		}
		tags := args[1:]

		for _, tag := range tags {
			if !strings.HasPrefix(tag, "tag:") {
				return fmt.Errorf("tag must start with 'tag:': %s", tag)
			}
			if strings.ToLower(tag) != tag {
				return fmt.Errorf("tag must be lowercase: %s", tag)
			}
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.SetTags(ctx, &v1.SetTagsRequest{NodeId: nodeID, Tags: tags})
		if err != nil {
			return fmt.Errorf("setting tags: %w", err)
		}

		fmt.Printf("Node %d tags set to: %v\n", resp.Node.Id, resp.Node.Tags)
		return nil
	},
}

var setRoutesCmd = &cobra.Command{
	Use:   "set-routes NODE_ID ROUTES...",
	Short: "Set approved routes on a node",
	Long:  "Set approved subnet routes on a node. Routes should be CIDR prefixes like '10.0.0.0/24'.",
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
		resp, err := client.SetApprovedRoutes(ctx, &v1.SetApprovedRoutesRequest{NodeId: nodeID, Routes: routes})
		if err != nil {
			return fmt.Errorf("setting routes: %w", err)
		}

		fmt.Printf("Node %d approved routes set to: %v\n", resp.Node.Id, resp.Node.SubnetRoutes)
		return nil
	},
}

func getGRPCContext() (context.Context, *grpc.ClientConn, error) {
	addr := gRPCAddr()
	apiKey := viper.GetString("cli.api_key")
	timeout := viper.GetDuration("cli.timeout")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("connecting to gRPC server at %s: %w", addr, err)
	}

	go func() {
		<-ctx.Done()
		cancel()
		conn.Close()
	}()

	log.Debug().Str("addr", addr).Msg("Connected to gRPC server")
	return ctx, conn, nil
}

func formatIPsJSON(ips []string) string {
	if len(ips) == 0 {
		return ""
	}
	quoted := make([]string, len(ips))
	for i, ip := range ips {
		quoted[i] = fmt.Sprintf("\"%s\"", ip)
	}
	return strings.Join(quoted, ", ")
}
