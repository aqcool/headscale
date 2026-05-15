package cli

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func init() {
	rootCmd.AddCommand(apikeysCmd)
	apikeysCmd.AddCommand(listApiKeysCmd)
	apikeysCmd.AddCommand(createApiKeyCmd)
	apikeysCmd.AddCommand(deleteApiKeyCmd)
	apikeysCmd.AddCommand(expireApiKeyCmd)

	createApiKeyCmd.Flags().String("expiration", "", "Duration until key expires (e.g., '24h', '7d', '720h')")
	deleteApiKeyCmd.Flags().String("prefix", "", "API key prefix to delete")
	deleteApiKeyCmd.Flags().Uint64("id", 0, "API key ID to delete")
	deleteApiKeyCmd.Flags().Bool("force", false, "Force delete without confirmation")
	expireApiKeyCmd.Flags().String("prefix", "", "API key prefix to expire")
	expireApiKeyCmd.Flags().Uint64("id", 0, "API key ID to expire")
}

var apikeysCmd = &cobra.Command{
	Use:   "apikeys",
	Short: "Manage API keys",
	Long:  "Manage API keys for authenticating with the headscale API.",
}

var listApiKeysCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all API keys",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.ListApiKeys(ctx, &v1.ListApiKeysRequest{})
		if err != nil {
			return fmt.Errorf("listing API keys: %w", err)
		}

		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat == "json" || outputFormat == "json-line" {
			for _, key := range resp.ApiKeys {
				expiry := "null"
				if key.Expiration != nil {
					expiry = fmt.Sprintf("\"%s\"", key.Expiration.AsTime().Format(time.RFC3339))
				}
				fmt.Printf("{\"id\": %d, \"prefix\": \"%s\", \"expiry\": %s}\n",
					key.Id, key.Prefix, expiry)
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 10, 1, 5, ' ', 0)
		fmt.Fprintf(w, "ID\tPREFIX\tCREATED\tEXPIRY\n")
		for _, key := range resp.ApiKeys {
			expiry := ""
			if key.Expiration != nil {
				if key.Expiration.AsTime().Before(time.Now()) {
					expiry = "EXPIRED"
				} else {
					expiry = key.Expiration.AsTime().Format("2006-01-02 15:04:05")
				}
			}
			created := ""
			if key.CreatedAt != nil {
				created = key.CreatedAt.AsTime().Format("2006-01-02 15:04:05")
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", key.Id, key.Prefix, created, expiry)
		}
		w.Flush()
		return nil
	},
}

var createApiKeyCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		expirationStr, _ := cmd.Flags().GetString("expiration")

		var expiration *timestamppb.Timestamp
		if expirationStr != "" {
			dur, err := time.ParseDuration(expirationStr)
			if err != nil {
				return fmt.Errorf("parsing expiration duration: %w", err)
			}
			expiration = timestamppb.New(time.Now().Add(dur))
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.CreateApiKey(ctx, &v1.CreateApiKeyRequest{
			Expiration: expiration,
		})
		if err != nil {
			return fmt.Errorf("creating API key: %w", err)
		}

		fmt.Printf("API key created: %s\n", resp.ApiKey)
		fmt.Println("Store this key securely. It will not be shown again.")
		return nil
	},
}

var deleteApiKeyCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix, _ := cmd.Flags().GetString("prefix")
		id, _ := cmd.Flags().GetUint64("id")

		if prefix == "" && id == 0 {
			return fmt.Errorf("must provide --prefix or --id")
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			if prefix != "" {
				fmt.Printf("Are you sure you want to delete API key with prefix %s? Use --force to skip confirmation\n", prefix)
			} else {
				fmt.Printf("Are you sure you want to delete API key %d? Use --force to skip confirmation\n", id)
			}
			return nil
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		_, err = client.DeleteApiKey(ctx, &v1.DeleteApiKeyRequest{
			Prefix: prefix,
			Id:     id,
		})
		if err != nil {
			return fmt.Errorf("deleting API key: %w", err)
		}

		fmt.Println("API key deleted")
		return nil
	},
}

var expireApiKeyCmd = &cobra.Command{
	Use:   "expire",
	Short: "Expire an API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix, _ := cmd.Flags().GetString("prefix")
		id, _ := cmd.Flags().GetUint64("id")

		if prefix == "" && id == 0 {
			return fmt.Errorf("must provide --prefix or --id")
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		_, err = client.ExpireApiKey(ctx, &v1.ExpireApiKeyRequest{
			Prefix: prefix,
			Id:     id,
		})
		if err != nil {
			return fmt.Errorf("expiring API key: %w", err)
		}

		fmt.Println("API key expired")
		return nil
	},
}