package collect

import (
	"context"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func TestCollectVolumes(t *testing.T) {
	r, e := collectVolumes(context.Background(), &fakeEC2{}, Scope{"123", "us-east-1"})
	if e != nil || len(r) != 3 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Provider != "aws" || r[0].Region != "us-east-1" || r[0].AccountID != "123" || r[0].ID == "" || r[0].ARN == "" {
		t.Fatal(r[0])
	}
}
func TestCollectSnapshots(t *testing.T) {
	r, e := collectSnapshots(context.Background(), &fakeEC2{}, Scope{"123", "us-east-1"})
	if e != nil || len(r) != 3 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Provider != "aws" || r[0].Region != "us-east-1" || r[0].AccountID != "123" || r[0].ID == "" || r[0].ARN == "" {
		t.Fatal(r[0])
	}
	if r[0].Bool("source_volume_exists") || r[0].Bool("used_by_ami") {
		t.Fatal(r[0])
	}
}
func TestVolumeAttachmentAndSnapshotReferences(t *testing.T) {
	ctx := context.Background()
	r, e := collectVolumes(ctx, &fakeEC2{}, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	if r[0].Str("attached_instance_id") != "" || r[1].Str("attached_instance_id") != "i-stopped" || r[1].Str("attached_instance_state") != "stopped" {
		t.Fatal(r)
	}
	r, e = collectSnapshots(ctx, &fakeEC2{}, Scope{})
	if e != nil {
		t.Fatal(e)
	}
	if !r[1].Bool("source_volume_exists") || !r[1].Bool("used_by_ami") {
		t.Fatal(r)
	}
}

// Disabled AMIs retain their backing snapshots; the fake models the API's
// opt-in visibility so omitting IncludeDisabled loses the reference.
type disabledAMIAPI struct {
	fakeEC2
	t          *testing.T
	imageCalls int
}

func (f *disabledAMIAPI) DescribeImages(_ context.Context, in *ec2.DescribeImagesInput, _ ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	f.imageCalls++
	if !slices.Equal(in.Owners, []string{"self"}) {
		f.t.Errorf("AMI owners = %v", in.Owners)
	}
	if !aws.ToBool(in.IncludeDisabled) {
		f.t.Error("DescribeImages must include disabled AMIs on every page")
	}
	if in.NextToken == nil {
		return &ec2.DescribeImagesOutput{NextToken: aws.String("disabled-page")}, nil
	}
	out := &ec2.DescribeImagesOutput{}
	if aws.ToBool(in.IncludeDisabled) {
		out.Images = []types.Image{{ImageId: aws.String("ami-disabled"), State: types.ImageStateDisabled, BlockDeviceMappings: []types.BlockDeviceMapping{{Ebs: &types.EbsBlockDevice{SnapshotId: aws.String("snap-1")}}}}}
	}
	return out, nil
}
func TestSnapshotsRetainDisabledAMIReferences(t *testing.T) {
	api := &disabledAMIAPI{t: t}
	r, err := collectSnapshots(context.Background(), api, Scope{"123", "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if api.imageCalls != 2 {
		t.Fatalf("image pages = %d", api.imageCalls)
	}
	if r[0].ID != "snap-1" || r[0].Bool("source_volume_exists") || !r[0].Bool("used_by_ami") {
		t.Fatalf("disabled AMI backing snapshot: %+v", r[0])
	}
}
func TestSnapshotARNHasEmptyAccount(t *testing.T) {
	r, err := collectSnapshots(context.Background(), &fakeEC2{}, Scope{"123456789012", "eu-west-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got := r[0].ARN; got != "arn:aws:ec2:eu-west-1::snapshot/snap-1" {
		t.Fatalf("snapshot ARN = %q", got)
	}
	if r[0].AccountID != "123456789012" {
		t.Fatalf("resource account = %q", r[0].AccountID)
	}
}
