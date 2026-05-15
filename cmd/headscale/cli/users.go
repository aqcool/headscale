package cli

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	v1 "github.com/juanfont/headscale-v2/api/proto/v1"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(usersCmd)
	usersCmd.AddCommand(listUsersCmd)
	usersCmd.AddCommand(createUserCmd)
	usersCmd.AddCommand(deleteUserCmd)
	usersCmd.AddCommand(renameUserCmd)

	createUserCmd.Flags().String("display-name", "", "Display name for the user")
	createUserCmd.Flags().String("email", "", "Email address for the user")
}

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage users",
	Long:  "Manage the users of the headscale control server.",
}

var listUsersCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all users",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.ListUsers(ctx, &v1.ListUsersRequest{})
		if err != nil {
			return fmt.Errorf("listing users: %w", err)
		}

		outputFormat, _ := cmd.Flags().GetString("output")
		if outputFormat == "json" || outputFormat == "json-line" {
			for _, user := range resp.Users {
				fmt.Printf("{\"id\": %d, \"name\": \"%s\", \"display_name\": \"%s\", \"email\": \"%s\"}\n",
					user.Id, user.Name, user.DisplayName, user.Email)
			}
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 10, 1, 5, ' ', 0)
		fmt.Fprintf(w, "ID\tNAME\tDISPLAY NAME\tEMAIL\tCREATED\n")
		for _, user := range resp.Users {
			created := ""
			if user.CreatedAt != nil {
				created = user.CreatedAt.AsTime().Format("2006-01-02 15:04:05")
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
				user.Id, user.Name, user.DisplayName, user.Email, created)
		}
		w.Flush()
		return nil
	},
}

var createUserCmd = &cobra.Command{
	Use:   "create NAME",
	Short: "Create a new user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		displayName, _ := cmd.Flags().GetString("display-name")
		email, _ := cmd.Flags().GetString("email")

		if displayName == "" {
			displayName = name
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)
		resp, err := client.CreateUser(ctx, &v1.CreateUserRequest{
			Name:        name,
			DisplayName: displayName,
			Email:       email,
		})
		if err != nil {
			return fmt.Errorf("creating user: %w", err)
		}

		fmt.Printf("User %q created (ID: %d)\n", resp.User.Name, resp.User.Id)
		return nil
	},
}

var deleteUserCmd = &cobra.Command{
	Use:   "delete USER",
	Short: "Delete a user",
	Long:  "Delete a user. USER can be a name or ID.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userArg := args[0]

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Are you sure you want to delete user %q? Use --force to skip confirmation\n", userArg)
			return nil
		}

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)

		var userID uint64
		if id, err := strconv.ParseUint(userArg, 10, 64); err == nil {
			userID = id
		} else {
			resp, err := client.ListUsers(ctx, &v1.ListUsersRequest{Name: userArg})
			if err != nil {
				return fmt.Errorf("finding user: %w", err)
			}
			if len(resp.Users) == 0 {
				return fmt.Errorf("user %q not found", userArg)
			}
			userID = resp.Users[0].Id
		}

		_, err = client.DeleteUser(ctx, &v1.DeleteUserRequest{Id: userID})
		if err != nil {
			return fmt.Errorf("deleting user: %w", err)
		}

		fmt.Printf("User %q deleted\n", userArg)
		return nil
	},
}

var renameUserCmd = &cobra.Command{
	Use:   "rename OLD_NAME NEW_NAME",
	Short: "Rename a user",
	Long:  "Rename a user. OLD_NAME can be a name or ID.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldName := args[0]
		newName := args[1]

		ctx, conn, err := getGRPCContext()
		if err != nil {
			return err
		}
		defer conn.Close()

		client := v1.NewHeadscaleServiceClient(conn)

		var oldID uint64
		if id, err := strconv.ParseUint(oldName, 10, 64); err == nil {
			oldID = id
		} else {
			resp, err := client.ListUsers(ctx, &v1.ListUsersRequest{Name: oldName})
			if err != nil {
				return fmt.Errorf("finding user: %w", err)
			}
			if len(resp.Users) == 0 {
				return fmt.Errorf("user %q not found", oldName)
			}
			oldID = resp.Users[0].Id
		}

		resp, err := client.RenameUser(ctx, &v1.RenameUserRequest{
			OldId:   oldID,
			NewName: newName,
		})
		if err != nil {
			return fmt.Errorf("renaming user: %w", err)
		}

		fmt.Printf("User %q renamed to %q\n", oldName, resp.User.Name)
		return nil
	},
}