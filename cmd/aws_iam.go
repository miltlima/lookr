package cmd

import (
	"fmt"
	"lookr/deps"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var IAMCmd = &cobra.Command{
	Use:   "iam",
	Short: "Query AWS IAM groups, users, and roles in different regions",
	Run:   queryIAM,
}

func init() {
	rootCmd.AddCommand(IAMCmd)
}

func queryIAM(cmd *cobra.Command, args []string) {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Type", "Region", "Creation Time"})

	AuthRegions := deps.AuthRegions()
	for _, region := range AuthRegions {
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(region),
		})

		if err != nil {
			fmt.Println("failed to create session,", err)
			return
		}

		iamClient := iam.New(sess)

		listGroupsInput := &iam.ListGroupsInput{}
		groupsResult, err := iamClient.ListGroups(listGroupsInput)
		if err != nil {
			fmt.Println("failed to list IAM groups,", err)
			return
		}

		listUsersInput := &iam.ListUsersInput{}
		usersResult, err := iamClient.ListUsers(listUsersInput)
		if err != nil {
			fmt.Println("failed to list IAM users,", err)
			return
		}

		listRolesInput := &iam.ListRolesInput{}
		rolesResult, err := iamClient.ListRoles(listRolesInput)
		if err != nil {
			fmt.Println("failed to list IAM roles,", err)
			return
		}

		regionName := deps.GetRegionName(region)

		for _, group := range groupsResult.Groups {
			row := []string{
				*group.GroupName,
				"Group",
				regionName,
				group.CreateDate.String(),
				*group.Arn,
			}
			table.Append(row)
		}

		for _, user := range usersResult.Users {
			row := []string{
				*user.UserName,
				"User",
				regionName,
				user.CreateDate.String(),
				*user.Arn,
			}
			table.Append(row)
		}

		for _, role := range rolesResult.Roles {
			row := []string{
				*role.RoleName,
				"Role",
				regionName,
				role.CreateDate.String(),
				*role.Arn,
			}
			table.Append(row)
		}
	}
	table.Render()
}
