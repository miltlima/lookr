package collect

import (
	"context"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"testing"
	"time"
)

type iamFake struct {
	err               error
	ListUsersFn       func(*iam.ListUsersInput) (*iam.ListUsersOutput, error)
	GetLoginProfileFn func(*iam.GetLoginProfileInput) (*iam.GetLoginProfileOutput, error)
	ListMFADevicesFn  func(*iam.ListMFADevicesInput) (*iam.ListMFADevicesOutput, error)
	ListAccessKeysFn  func(*iam.ListAccessKeysInput) (*iam.ListAccessKeysOutput, error)
	ListGroupsFn      func(*iam.ListGroupsInput) (*iam.ListGroupsOutput, error)
	ListRolesFn       func(*iam.ListRolesInput) (*iam.ListRolesOutput, error)
	ListUserTagsFn    func(*iam.ListUserTagsInput) (*iam.ListUserTagsOutput, error)
	ListRoleTagsFn    func(*iam.ListRoleTagsInput) (*iam.ListRoleTagsOutput, error)
}

func (f iamFake) ListUsers(_ context.Context, in *iam.ListUsersInput, _ ...func(*iam.Options)) (*iam.ListUsersOutput, error) {
	if f.ListUsersFn != nil {
		return f.ListUsersFn(in)
	}
	return &iam.ListUsersOutput{Users: []types.User{{}}}, f.err
}
func (f iamFake) GetLoginProfile(_ context.Context, in *iam.GetLoginProfileInput, _ ...func(*iam.Options)) (*iam.GetLoginProfileOutput, error) {
	if f.GetLoginProfileFn != nil {
		return f.GetLoginProfileFn(in)
	}
	return &iam.GetLoginProfileOutput{}, f.err
}
func (f iamFake) ListMFADevices(_ context.Context, in *iam.ListMFADevicesInput, _ ...func(*iam.Options)) (*iam.ListMFADevicesOutput, error) {
	if f.ListMFADevicesFn != nil {
		return f.ListMFADevicesFn(in)
	}
	return &iam.ListMFADevicesOutput{}, f.err
}
func (f iamFake) ListAccessKeys(_ context.Context, in *iam.ListAccessKeysInput, _ ...func(*iam.Options)) (*iam.ListAccessKeysOutput, error) {
	if f.ListAccessKeysFn != nil {
		return f.ListAccessKeysFn(in)
	}
	return &iam.ListAccessKeysOutput{}, f.err
}
func (f iamFake) ListGroups(_ context.Context, in *iam.ListGroupsInput, _ ...func(*iam.Options)) (*iam.ListGroupsOutput, error) {
	if f.ListGroupsFn != nil {
		return f.ListGroupsFn(in)
	}
	return &iam.ListGroupsOutput{Groups: []types.Group{{}}}, f.err
}
func (f iamFake) ListRoles(_ context.Context, in *iam.ListRolesInput, _ ...func(*iam.Options)) (*iam.ListRolesOutput, error) {
	if f.ListRolesFn != nil {
		return f.ListRolesFn(in)
	}
	return &iam.ListRolesOutput{Roles: []types.Role{{}}}, f.err
}
func (f iamFake) ListUserTags(_ context.Context, in *iam.ListUserTagsInput, _ ...func(*iam.Options)) (*iam.ListUserTagsOutput, error) {
	if f.ListUserTagsFn != nil {
		return f.ListUserTagsFn(in)
	}
	return &iam.ListUserTagsOutput{}, f.err
}
func (f iamFake) ListRoleTags(_ context.Context, in *iam.ListRoleTagsInput, _ ...func(*iam.Options)) (*iam.ListRoleTagsOutput, error) {
	if f.ListRoleTagsFn != nil {
		return f.ListRoleTagsFn(in)
	}
	return &iam.ListRoleTagsOutput{}, f.err
}
func TestTask7IAMSparse(t *testing.T) {
	r, e := collectUsers(context.Background(), iamFake{}, Scope{}, time.Now())
	if e != nil || len(r) != 1 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Config["oldest_active_key_age_days"] != -1 {
		t.Fatal(r)
	}
}
func TestTask7IamError(t *testing.T) {
	ctx := context.Background()
	want := errors.New("denied")
	f := iamFake{err: want}
	_, e := collectUsers(ctx, f, Scope{}, time.Now())
	if !errors.Is(e, want) {
		t.Fatalf("got %v", e)
	}
}
func TestTask7ListUsersPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := iamFake{}
	f.ListUsersFn = func(in *iam.ListUsersInput) (*iam.ListUsersOutput, error) {
		calls++
		if calls == 1 {
			return &iam.ListUsersOutput{IsTruncated: true, Marker: aws.String("next"), Users: []types.User{{}}}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal("missing page token")
		}
		return &iam.ListUsersOutput{}, nil
	}
	_, e := collectUsers(ctx, f, Scope{}, time.Now())
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7ListMFADevicesPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := iamFake{}
	f.ListMFADevicesFn = func(in *iam.ListMFADevicesInput) (*iam.ListMFADevicesOutput, error) {
		calls++
		if calls == 1 {
			return &iam.ListMFADevicesOutput{IsTruncated: true, Marker: aws.String("next")}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal("missing page token")
		}
		return &iam.ListMFADevicesOutput{}, nil
	}
	_, e := collectUsers(ctx, f, Scope{}, time.Now())
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7ListAccessKeysPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := iamFake{}
	f.ListAccessKeysFn = func(in *iam.ListAccessKeysInput) (*iam.ListAccessKeysOutput, error) {
		calls++
		if calls == 1 {
			return &iam.ListAccessKeysOutput{IsTruncated: true, Marker: aws.String("next")}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal("missing page token")
		}
		return &iam.ListAccessKeysOutput{}, nil
	}
	_, e := collectUsers(ctx, f, Scope{}, time.Now())
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7ListUserTagsPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := iamFake{}
	f.ListUserTagsFn = func(in *iam.ListUserTagsInput) (*iam.ListUserTagsOutput, error) {
		calls++
		if calls == 1 {
			return &iam.ListUserTagsOutput{IsTruncated: true, Marker: aws.String("next")}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal("missing page token")
		}
		return &iam.ListUserTagsOutput{}, nil
	}
	_, e := collectUsers(ctx, f, Scope{}, time.Now())
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7ListGroupsPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := iamFake{}
	f.ListGroupsFn = func(in *iam.ListGroupsInput) (*iam.ListGroupsOutput, error) {
		calls++
		if calls == 1 {
			return &iam.ListGroupsOutput{IsTruncated: true, Marker: aws.String("next"), Groups: []types.Group{{}}}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal("missing page token")
		}
		return &iam.ListGroupsOutput{}, nil
	}
	_, e := collectGroups(ctx, f, Scope{})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7ListRolesPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := iamFake{}
	f.ListRolesFn = func(in *iam.ListRolesInput) (*iam.ListRolesOutput, error) {
		calls++
		if calls == 1 {
			return &iam.ListRolesOutput{IsTruncated: true, Marker: aws.String("next"), Roles: []types.Role{{}}}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal("missing page token")
		}
		return &iam.ListRolesOutput{}, nil
	}
	_, e := collectRoles(ctx, f, Scope{})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7ListRoleTagsPages(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := iamFake{}
	f.ListRoleTagsFn = func(in *iam.ListRoleTagsInput) (*iam.ListRoleTagsOutput, error) {
		calls++
		if calls == 1 {
			return &iam.ListRoleTagsOutput{IsTruncated: true, Marker: aws.String("next")}, nil
		}
		if aws.ToString(in.Marker) != "next" {
			t.Fatal("missing page token")
		}
		return &iam.ListRoleTagsOutput{}, nil
	}
	_, e := collectRoles(ctx, f, Scope{})
	if e != nil || calls != 2 {
		t.Fatalf("calls=%d error=%v", calls, e)
	}
}
func TestTask7UserCredentials(t *testing.T) {
	now := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	f := iamFake{}
	f.GetLoginProfileFn = func(*iam.GetLoginProfileInput) (*iam.GetLoginProfileOutput, error) {
		return nil, &types.NoSuchEntityException{}
	}
	f.ListAccessKeysFn = func(*iam.ListAccessKeysInput) (*iam.ListAccessKeysOutput, error) {
		return &iam.ListAccessKeysOutput{AccessKeyMetadata: []types.AccessKeyMetadata{{Status: types.StatusTypeInactive, CreateDate: aws.Time(now.Add(-100 * 24 * time.Hour))}, {Status: types.StatusTypeActive, CreateDate: aws.Time(now.Add(-10 * 24 * time.Hour))}}}, nil
	}
	f.ListMFADevicesFn = func(*iam.ListMFADevicesInput) (*iam.ListMFADevicesOutput, error) {
		return &iam.ListMFADevicesOutput{MFADevices: []types.MFADevice{{}}}, nil
	}
	r, e := collectUsers(context.Background(), f, Scope{}, now)
	if e != nil {
		t.Fatal(e)
	}
	c := r[0].Config
	if c["has_console_password"] != false || c["mfa_enabled"] != true || c["oldest_active_key_age_days"] != 10 {
		t.Fatal(c)
	}
	denied := errors.New("access denied")
	f.GetLoginProfileFn = func(*iam.GetLoginProfileInput) (*iam.GetLoginProfileOutput, error) { return nil, denied }
	r, e = collectUsers(context.Background(), f, Scope{}, now)
	if !errors.Is(e, denied) || len(r) != 0 {
		t.Fatalf("%v %v", r, e)
	}
}
func (f iamFake) GetRole(_ context.Context, in *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	return &iam.GetRoleOutput{Role: &types.Role{RoleName: in.RoleName, RoleLastUsed: &types.RoleLastUsed{LastUsedDate: aws.Time(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))}}}, f.err
}
func TestTask7RoleLastUsed(t *testing.T) {
	r, e := collectRoles(context.Background(), iamFake{}, Scope{})
	if e != nil || len(r) != 1 || r[0].Config["last_used"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("%v %v", r, e)
	}
}
