package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func init() {
	rootCmd.AddCommand(preauthkeysCmd)
	preauthkeysCmd.AddCommand(listPreAuthKeysCmd)
	preauthkeysCmd.AddCommand(createPreAuthKeyCmd)
	preauthkeysCmd.AddCommand(deletePreAuthKeyCmd)
	preauthkeysCmd.AddCommand(expirePreAuthKeyCmd)

	createPreAuthKeyCmd.Flags().Bool("reusable", false, "Make the key reusable")
	createPreAuthKeyCmd.Flags().Bool("ephemeral", false, "Create an ephemeral key")
	createPreAuthKeyCmd.Flags().String("expiration", "", "Duration until key expires (e.g., '24h', '7d')")
	createPreAuthKeyCmd.Flags().String("tags", "", "Comma-separated list of ACL tags (e.g., 'tag:server,tag:web')")
}

var preauthkeysCmd = &cobra.Command{
	Use:     "preauthkeys",
	Short:   "Manage pre-authentication keys",
	Aliases: []string{"preauth", "pak"},
	Long:    "Manage pre-authentication keys for registering nodes without user interaction.",
}

var listPreAuthKeysCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all pre-authentication keys",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.ListPreAuthKeys(ctx, &v1.ListPreAuthKeysRequest{})
		if err != nil {
			return fmt.Errorf("listing pre-auth keys: %w", err)
		}

		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat == "json" || outputFormat == "json-line" {
			for _, key := range resp.PreAuthKeys {
				expiry := "null"
				if key.Expiration != nil {
					expiry = fmt.Sprintf("\"%s\"", key.Expiration.AsTime().Format(time.RFC3339))
				}
				fmt.Printf("{\"id\": %d, \"key\": \"%s\", \"reusable\": %v, \"ephemeral\": %v, \"used\": %v, \"expiry\": %s}\n",
					key.Id, key.Key, key.Reusable, key.Ephemeral, key.Used, expiry)
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 10, 1, 5, ' ', 0)
		fmt.Fprintf(w, "ID\tKEY\tREUSABLE\tEPHEMERAL\tUSED\tEXPIRY\tTAGS\n")
		for _, key := range resp.PreAuthKeys {
			expiry := ""
			if key.Expiration != nil {
				expiry = key.Expiration.AsTime().Format("2006-01-02 15:04:05")
				if key.Expiration.AsTime().Before(time.Now()) {
					expiry = "EXPIRED"
				}
			}
			used := "false"
			if key.Used {
				used = "true"
			}
			tags := strings.Join(key.AclTags, ",")
			fmt.Fprintf(w, "%d\t%s\t%v\t%v\t%s\t%s\t%s\n",
				key.Id, key.Key, key.Reusable, key.Ephemeral, used, expiry, tags)
		}
		w.Flush()
		return nil
	},
}

var createPreAuthKeyCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a pre-authentication key",
	RunE: func(cmd *cobra.Command, args []string) error {
		reusable, _ := cmd.Flags().GetBool("reusable")
		ephemeral, _ := cmd.Flags().GetBool("ephemeral")
		expirationStr, _ := cmd.Flags().GetString("expiration")
		tagsStr, _ := cmd.Flags().GetString("tags")

		var expiration *timestamppb.Timestamp
		if expirationStr != "" {
			dur, err := time.ParseDuration(expirationStr)
			if err != nil {
				return fmt.Errorf("parsing expiration duration: %w", err)
			}
			expiration = timestamppb.New(time.Now().Add(dur))
		}

		var tags []string
		if tagsStr != "" {
			tags = strings.Split(tagsStr, ",")
			for _, tag := range tags {
				tag = strings.TrimSpace(tag)
				if !strings.HasPrefix(tag, "tag:") {
					return fmt.Errorf("tag must start with 'tag:': %s", tag)
				}
				if strings.ToLower(tag) != tag {
					return fmt.Errorf("tag must be lowercase: %s", tag)
				}
			}
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.CreatePreAuthKey(ctx, &v1.CreatePreAuthKeyRequest{
			Reusable:   reusable,
			Ephemeral:  ephemeral,
			Expiration: expiration,
			AclTags:    tags,
		})
		if err != nil {
			return fmt.Errorf("creating pre-auth key: %w", err)
		}

		fmt.Printf("Pre-auth key created: %s\n", resp.PreAuthKey.Key)
		return nil
	},
}

var deletePreAuthKeyCmd = &cobra.Command{
	Use:   "delete KEY_ID",
	Short: "Delete a pre-authentication key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		keyID, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid key ID: %w", err)
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Are you sure you want to delete pre-auth key %d? Use --force to skip confirmation\n", keyID)
			return nil
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		_, err = client.DeletePreAuthKey(ctx, &v1.DeletePreAuthKeyRequest{Id: keyID})
		if err != nil {
			return fmt.Errorf("deleting pre-auth key: %w", err)
		}

		fmt.Printf("Pre-auth key %d deleted\n", keyID)
		return nil
	},
}

var expirePreAuthKeyCmd = &cobra.Command{
	Use:   "expire KEY_ID",
	Short: "Expire a pre-authentication key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		keyID, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid key ID: %w", err)
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		_, err = client.ExpirePreAuthKey(ctx, &v1.ExpirePreAuthKeyRequest{Id: keyID})
		if err != nil {
			return fmt.Errorf("expiring pre-auth key: %w", err)
		}

		fmt.Printf("Pre-auth key %d expired\n", keyID)
		return nil
	},
}
