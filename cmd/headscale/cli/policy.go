package cli

import (
	"fmt"
	"os"
	"strings"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(policyCmd)
	policyCmd.AddCommand(getPolicyCmd)
	policyCmd.AddCommand(setPolicyCmd)
	policyCmd.AddCommand(checkPolicyCmd)
}

var policyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Manage ACL policy",
	Long:  "Manage the ACL (Access Control List) policy for headscale.",
}

var getPolicyCmd = &cobra.Command{
	Use:     "get",
	Short:   "Get the current ACL policy",
	Aliases: []string{"show"},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.GetPolicy(ctx, &v1.GetPolicyRequest{})
		if err != nil {
			return fmt.Errorf("getting policy: %w", err)
		}

		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat == "json" || outputFormat == "json-line" {
			fmt.Printf("%s\n", resp.Policy)
		} else {
			fmt.Println(resp.Policy)
		}
		return nil
	},
}

var setPolicyCmd = &cobra.Command{
	Use:   "set POLICY",
	Short: "Set the ACL policy",
	Long:  "Set the ACL policy from a file path or direct JSON input.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		policyInput := args[0]
		var policyData string

		if strings.HasPrefix(policyInput, "{") || strings.HasPrefix(policyInput, "[") {
			policyData = policyInput
		} else {
			data, err := os.ReadFile(policyInput)
			if err != nil {
				return fmt.Errorf("reading policy file: %w", err)
			}
			policyData = string(data)
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.SetPolicy(ctx, &v1.SetPolicyRequest{Policy: policyData})
		if err != nil {
			return fmt.Errorf("setting policy: %w", err)
		}

		fmt.Printf("Policy updated at %s\n", resp.UpdatedAt.AsTime().Format("2006-01-02 15:04:05"))
		return nil
	},
}

var checkPolicyCmd = &cobra.Command{
	Use:   "check FILE",
	Short: "Check if a policy file is valid",
	Long:  "Validate a policy file without applying it.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		policyFile := args[0]

		data, err := os.ReadFile(policyFile)
		if err != nil {
			return fmt.Errorf("reading policy file: %w", err)
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		_, err = client.SetPolicy(ctx, &v1.SetPolicyRequest{Policy: string(data)})
		if err != nil {
			return fmt.Errorf("policy validation failed: %w", err)
		}

		fmt.Printf("Policy file %s is valid\n", policyFile)
		return nil
	},
}