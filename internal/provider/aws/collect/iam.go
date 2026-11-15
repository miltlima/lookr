package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"lookr/internal/model"
	"time"
)

type userAPI interface {
	ListUsers(context.Context, *iam.ListUsersInput, ...func(*iam.Options)) (*iam.ListUsersOutput, error)
	GetLoginProfile(context.Context, *iam.GetLoginProfileInput, ...func(*iam.Options)) (*iam.GetLoginProfileOutput, error)
	ListMFADevices(context.Context, *iam.ListMFADevicesInput, ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error)
	ListAccessKeys(context.Context, *iam.ListAccessKeysInput, ...func(*iam.Options)) (*iam.ListAccessKeysOutput, error)
	ListUserTags(context.Context, *iam.ListUserTagsInput, ...func(*iam.Options)) (*iam.ListUserTagsOutput, error)
}
type groupAPI interface {
	ListGroups(context.Context, *iam.ListGroupsInput, ...func(*iam.Options)) (*iam.ListGroupsOutput, error)
}
type roleAPI interface {
	GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	ListRoles(context.Context, *iam.ListRolesInput, ...func(*iam.Options)) (*iam.ListRolesOutput, error)
	ListRoleTags(context.Context, *iam.ListRoleTagsInput, ...func(*iam.Options)) (*iam.ListRoleTagsOutput, error)
}

func iamTime(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.UTC().Format(time.RFC3339)
}
func collectUsers(ctx context.Context, c userAPI, s Scope, now time.Time) ([]model.Resource, error) {
	s.Region = "global"
	var result []model.Resource
	in := &iam.ListUsersInput{}
	for {
		o, e := c.ListUsers(ctx, in)
		if e != nil {
			return result, e
		}
		for _, v := range o.Users {
			password := true
			_, e := c.GetLoginProfile(ctx, &iam.GetLoginProfileInput{UserName: v.UserName})
			if e != nil {
				var absent *types.NoSuchEntityException
				if errors.As(e, &absent) {
					password = false
				} else {
					return result, e
				}
			}
			mfa := false
			mi := &iam.ListMFADevicesInput{UserName: v.UserName}
			for {
				m, e := c.ListMFADevices(ctx, mi)
				if e != nil {
					return result, e
				}
				mfa = mfa || len(m.MFADevices) > 0
				if !m.IsTruncated {
					break
				}
				mi.Marker = m.Marker
			}
			age := -1
			ki := &iam.ListAccessKeysInput{UserName: v.UserName}
			for {
				k, e := c.ListAccessKeys(ctx, ki)
				if e != nil {
					return result, e
				}
				for _, key := range k.AccessKeyMetadata {
					if key.Status == types.StatusTypeActive && key.CreateDate != nil {
						days := int(now.Sub(*key.CreateDate).Hours() / 24)
						if days < 0 {
							days = 0
						}
						if days > age {
							age = days
						}
					}
				}
				if !k.IsTruncated {
					break
				}
				ki.Marker = k.Marker
			}
			tags := map[string]string{}
			ti := &iam.ListUserTagsInput{UserName: v.UserName}
			for {
				t, e := c.ListUserTags(ctx, ti)
				if e != nil {
					return result, e
				}
				for _, x := range t.Tags {
					tags[aws.ToString(x.Key)] = aws.ToString(x.Value)
				}
				if !t.IsTruncated {
					break
				}
				ti.Marker = t.Marker
			}
			r := resource(s, "aws.iam.user", "iam", aws.ToString(v.UserName), "", tags, map[string]any{"path": aws.ToString(v.Path), "password_last_used": iamTime(v.PasswordLastUsed), "has_console_password": password, "mfa_enabled": mfa, "oldest_active_key_age_days": age})
			r.ARN = aws.ToString(v.Arn)
			r.CreatedAt = v.CreateDate
			result = append(result, r)
		}
		if !o.IsTruncated {
			break
		}
		in.Marker = o.Marker
	}
	return result, nil
}
func collectGroups(ctx context.Context, c groupAPI, s Scope) ([]model.Resource, error) {
	s.Region = "global"
	var result []model.Resource
	in := &iam.ListGroupsInput{}
	for {
		o, e := c.ListGroups(ctx, in)
		if e != nil {
			return result, e
		}
		for _, v := range o.Groups {
			r := resource(s, "aws.iam.group", "iam", aws.ToString(v.GroupName), "", nil, map[string]any{"path": aws.ToString(v.Path)})
			r.ARN = aws.ToString(v.Arn)
			r.CreatedAt = v.CreateDate
			result = append(result, r)
		}
		if !o.IsTruncated {
			break
		}
		in.Marker = o.Marker
	}
	return result, nil
}
func collectRoles(ctx context.Context, c roleAPI, s Scope) ([]model.Resource, error) {
	s.Region = "global"
	var result []model.Resource
	in := &iam.ListRolesInput{}
	for {
		o, e := c.ListRoles(ctx, in)
		if e != nil {
			return result, e
		}
		for _, v := range o.Roles {
			d, e := c.GetRole(ctx, &iam.GetRoleInput{RoleName: v.RoleName})
			if e != nil {
				return result, e
			}
			if d.Role != nil {
				v = *d.Role
			}
			tags := map[string]string{}
			ti := &iam.ListRoleTagsInput{RoleName: v.RoleName}
			for {
				t, e := c.ListRoleTags(ctx, ti)
				if e != nil {
					return result, e
				}
				for _, x := range t.Tags {
					tags[aws.ToString(x.Key)] = aws.ToString(x.Value)
				}
				if !t.IsTruncated {
					break
				}
				ti.Marker = t.Marker
			}
			last := ""
			if v.RoleLastUsed != nil {
				last = iamTime(v.RoleLastUsed.LastUsedDate)
			}
			r := resource(s, "aws.iam.role", "iam", aws.ToString(v.RoleName), "", tags, map[string]any{"path": aws.ToString(v.Path), "last_used": last})
			r.ARN = aws.ToString(v.Arn)
			r.CreatedAt = v.CreateDate
			result = append(result, r)
		}
		if !o.IsTruncated {
			break
		}
		in.Marker = o.Marker
	}
	return result, nil
}
