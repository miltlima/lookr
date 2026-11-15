package collect

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"lookr/internal/model"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type fakeEC2 struct {
	calls     map[string]int
	fail      string
	failAfter int
}

func (f *fakeEC2) DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, opts ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls["Instances"]++
	if f.fail == "Instances" && f.calls["Instances"] > f.failAfter {
		return nil, errors.New("denied")
	}
	out := &ec2.DescribeInstancesOutput{Reservations: []types.Reservation{{Instances: []types.Instance{{InstanceId: aws.String("i-1"), State: &types.InstanceState{Name: types.InstanceStateNameRunning}, Tags: []types.Tag{{Key: aws.String("Name"), Value: aws.String("web")}}, MetadataOptions: &types.InstanceMetadataOptionsResponse{HttpTokens: types.HttpTokensStateRequired}}}}}}
	if in.NextToken == nil {
		out.NextToken = aws.String("next")
	}
	if in.NextToken == nil {
		out.Reservations[0].Instances = append(out.Reservations[0].Instances, types.Instance{InstanceId: aws.String("i-stopped"), State: &types.InstanceState{Name: types.InstanceStateNameStopped}}, types.Instance{InstanceId: aws.String("i-terminated"), State: &types.InstanceState{Name: types.InstanceStateNameTerminated}})
	} else {
		out.Reservations = []types.Reservation{{Instances: []types.Instance{{InstanceId: aws.String("i-sparse")}}}}
	}
	return out, nil
}
func (f *fakeEC2) DescribeVolumes(ctx context.Context, in *ec2.DescribeVolumesInput, opts ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls["Volumes"]++
	if f.fail == "Volumes" && f.calls["Volumes"] > f.failAfter {
		return nil, errors.New("denied")
	}
	out := &ec2.DescribeVolumesOutput{Volumes: []types.Volume{{VolumeId: aws.String("vol-1"), State: types.VolumeStateAvailable}}}
	if in.NextToken == nil {
		out.NextToken = aws.String("next")
	}
	if in.NextToken == nil {
		out.Volumes = append(out.Volumes, types.Volume{VolumeId: aws.String("vol-attached"), State: types.VolumeStateInUse, Attachments: []types.VolumeAttachment{{InstanceId: aws.String("i-stopped")}}})
	} else {
		out.Volumes = []types.Volume{{VolumeId: aws.String("vol-sparse")}}
	}
	return out, nil
}
func (f *fakeEC2) DescribeSnapshots(ctx context.Context, in *ec2.DescribeSnapshotsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls["Snapshots"]++
	if f.fail == "Snapshots" && f.calls["Snapshots"] > f.failAfter {
		return nil, errors.New("denied")
	}
	out := &ec2.DescribeSnapshotsOutput{Snapshots: []types.Snapshot{{SnapshotId: aws.String("snap-1"), VolumeId: aws.String("vol-gone")}}}
	if in.NextToken == nil {
		out.NextToken = aws.String("next")
	}
	if !slices.Equal(in.OwnerIds, []string{"self"}) {
		panic("snapshot owners")
	}
	if in.NextToken == nil {
		out.Snapshots = append(out.Snapshots, types.Snapshot{SnapshotId: aws.String("snap-used"), VolumeId: aws.String("vol-1")})
	} else {
		out.Snapshots = []types.Snapshot{{SnapshotId: aws.String("snap-sparse")}}
	}
	return out, nil
}
func (f *fakeEC2) DescribeAddresses(ctx context.Context, in *ec2.DescribeAddressesInput, opts ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls["Addresses"]++
	if f.fail == "Addresses" {
		return nil, errors.New("denied")
	}
	out := &ec2.DescribeAddressesOutput{Addresses: []types.Address{{AllocationId: aws.String("eipalloc-1")}}}
	return out, nil
}
func (f *fakeEC2) DescribeNatGateways(ctx context.Context, in *ec2.DescribeNatGatewaysInput, opts ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls["NatGateways"]++
	if f.fail == "NatGateways" && f.calls["NatGateways"] > f.failAfter {
		return nil, errors.New("denied")
	}
	out := &ec2.DescribeNatGatewaysOutput{NatGateways: []types.NatGateway{{NatGatewayId: aws.String("nat-1")}}}
	if in.NextToken == nil {
		out.NextToken = aws.String("next")
	}
	if in.NextToken == nil {
		out.NatGateways = append(out.NatGateways, types.NatGateway{NatGatewayId: aws.String("nat-2")}, types.NatGateway{NatGatewayId: aws.String("nat-deleted"), State: types.NatGatewayStateDeleted})
	} else {
		out.NatGateways = []types.NatGateway{{NatGatewayId: aws.String("nat-sparse")}}
	}
	return out, nil
}
func (f *fakeEC2) DescribeSecurityGroups(ctx context.Context, in *ec2.DescribeSecurityGroupsInput, opts ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls["SecurityGroups"]++
	if f.fail == "SecurityGroups" && f.calls["SecurityGroups"] > f.failAfter {
		return nil, errors.New("denied")
	}
	out := &ec2.DescribeSecurityGroupsOutput{SecurityGroups: []types.SecurityGroup{{GroupId: aws.String("sg-1"), IpPermissions: []types.IpPermission{{IpProtocol: aws.String("tcp"), FromPort: aws.Int32(20), ToPort: aws.Int32(3400), IpRanges: []types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}}}}}}}
	if in.NextToken == nil {
		out.NextToken = aws.String("next")
	}
	if in.NextToken == nil {
		out.SecurityGroups = append(out.SecurityGroups, types.SecurityGroup{GroupId: aws.String("sg-2")})
	} else {
		out.SecurityGroups = []types.SecurityGroup{{GroupId: aws.String("sg-sparse")}}
	}
	return out, nil
}
func (f *fakeEC2) DescribeImages(ctx context.Context, in *ec2.DescribeImagesInput, opts ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	if !slices.Equal(in.Owners, []string{"self"}) {
		panic("image owners")
	}
	if f.fail == "Images" {
		return nil, errors.New("denied")
	}
	out := &ec2.DescribeImagesOutput{}
	if in.NextToken == nil {
		out.NextToken = aws.String("next")
	} else {
		out.Images = []types.Image{{BlockDeviceMappings: []types.BlockDeviceMapping{{Ebs: &types.EbsBlockDevice{SnapshotId: aws.String("snap-used")}}, {}}}}
	}
	return out, nil
}

func TestCollectInstances(t *testing.T) {
	r, e := collectInstances(context.Background(), &fakeEC2{}, Scope{"123", "us-east-1"})
	if e != nil || len(r) != 3 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Provider != "aws" || r[0].Region != "us-east-1" || r[0].AccountID != "123" || r[0].ID == "" || r[0].ARN == "" {
		t.Fatal(r[0])
	}
	if r[0].Name != "web" || !r[0].Bool("imds_v2_required") || r[0].Str("public_ip") != "" || len(r[0].Strs("security_groups")) != 0 {
		t.Fatal(r[0])
	}
}

func TestCollectAddresses(t *testing.T) {
	r, e := collectAddresses(context.Background(), &fakeEC2{}, Scope{"123", "us-east-1"})
	if e != nil || len(r) != 1 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Provider != "aws" || r[0].Region != "us-east-1" || r[0].AccountID != "123" || r[0].ID == "" || r[0].ARN == "" {
		t.Fatal(r[0])
	}
}
func TestCollectNatGateways(t *testing.T) {
	r, e := collectNatGateways(context.Background(), &fakeEC2{}, Scope{"123", "us-east-1"})
	if e != nil || len(r) != 3 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Provider != "aws" || r[0].Region != "us-east-1" || r[0].AccountID != "123" || r[0].ID == "" || r[0].ARN == "" {
		t.Fatal(r[0])
	}
}
func TestCollectSecurityGroups(t *testing.T) {
	r, e := collectSecurityGroups(context.Background(), &fakeEC2{}, Scope{"123", "us-east-1"})
	if e != nil || len(r) != 3 {
		t.Fatalf("%v %v", r, e)
	}
	if r[0].Provider != "aws" || r[0].Region != "us-east-1" || r[0].AccountID != "123" || r[0].ID == "" || r[0].ARN == "" {
		t.Fatal(r[0])
	}
	if !slices.Contains(r[0].Strs("world_open_ports"), "22") || !slices.Contains(r[0].Strs("world_open_ports"), "3389") {
		t.Fatal(r[0])
	}
}
func TestCollectorPropagatesError(t *testing.T) {
	for _, m := range []string{"Instances", "Volumes", "Snapshots", "Addresses", "NatGateways", "SecurityGroups"} {
		f := &fakeEC2{fail: m}
		var err error
		switch m {
		case "Instances":
			_, err = collectInstances(context.Background(), f, Scope{})
		case "Volumes":
			_, err = collectVolumes(context.Background(), f, Scope{})
		case "Snapshots":
			_, err = collectSnapshots(context.Background(), f, Scope{})
		case "Addresses":
			_, err = collectAddresses(context.Background(), f, Scope{})
		case "NatGateways":
			_, err = collectNatGateways(context.Background(), f, Scope{})
		case "SecurityGroups":
			_, err = collectSecurityGroups(context.Background(), f, Scope{})
		}
		if err == nil {
			t.Fatal(m)
		}
	}
}

func assertContract(t *testing.T, r model.Resource, typ, short, id, arn string, keys []string) {
	t.Helper()
	if r.Type != typ || r.Short != short || r.ID != id || r.ARN != arn {
		t.Fatal(r)
	}
	got := []string{}
	for k := range r.Config {
		got = append(got, k)
	}
	slices.Sort(got)
	slices.Sort(keys)
	if !reflect.DeepEqual(got, keys) {
		t.Fatalf("config keys %v, want %v", got, keys)
	}
	if !reflect.DeepEqual(r.Usage, model.Usage{}) || !reflect.DeepEqual(r.Cost, model.Cost{}) || len(r.Findings) != 0 {
		t.Fatal(r)
	}
}
func TestSparseAndContract(t *testing.T) {
	ctx := context.Background()
	s := Scope{"123", "us-east-1"}
	cases := []struct {
		typ, short, id, kind string
		keys                 []string
		fn                   func(context.Context, *fakeEC2, Scope) ([]model.Resource, error)
	}{
		{"aws.ec2.instance", "ec2", "i-sparse", "instance", []string{"instance_type", "state", "platform", "private_ip", "public_ip", "image_id", "vpc_id", "security_groups", "iam_instance_profile"}, func(c context.Context, f *fakeEC2, s Scope) ([]model.Resource, error) {
			return collectInstances(c, f, s)
		}},
		{"aws.ebs.volume", "ebs", "vol-sparse", "volume", []string{"size_gb", "volume_type", "iops", "throughput", "state", "attached_instance_id", "attached_instance_state", "attachment_count"}, func(c context.Context, f *fakeEC2, s Scope) ([]model.Resource, error) { return collectVolumes(c, f, s) }},
		{"aws.ebs.snapshot", "snapshot", "snap-sparse", "snapshot", []string{"volume_id", "volume_size_gb", "state", "description", "storage_tier", "source_volume_exists", "used_by_ami"}, func(c context.Context, f *fakeEC2, s Scope) ([]model.Resource, error) {
			return collectSnapshots(c, f, s)
		}},
		{"aws.ec2.eip", "eip", "eipalloc-1", "elastic-ip", []string{"public_ip", "allocation_id", "association_id", "instance_id", "network_interface_id"}, func(c context.Context, f *fakeEC2, s Scope) ([]model.Resource, error) {
			return collectAddresses(c, f, s)
		}},
		{"aws.ec2.natgateway", "nat", "nat-sparse", "natgateway", []string{"state", "vpc_id", "subnet_id", "connectivity_type"}, func(c context.Context, f *fakeEC2, s Scope) ([]model.Resource, error) {
			return collectNatGateways(c, f, s)
		}},
		{"aws.ec2.securitygroup", "sg", "sg-sparse", "security-group", []string{"vpc_id", "description", "ingress_rule_count", "world_open_ports"}, func(c context.Context, f *fakeEC2, s Scope) ([]model.Resource, error) {
			return collectSecurityGroups(c, f, s)
		}},
	}
	for _, c := range cases {
		t.Run(c.short, func(t *testing.T) {
			r, e := c.fn(ctx, &fakeEC2{}, s)
			if e != nil {
				t.Fatal(e)
			}
			x := r[len(r)-1]
			wantARN := "arn:aws:ec2:us-east-1:123:" + c.kind + "/" + c.id
			if c.typ == "aws.ebs.snapshot" {
				wantARN = "arn:aws:ec2:us-east-1::snapshot/snap-sparse"
			}
			assertContract(t, x, c.typ, c.short, c.id, wantARN, c.keys)
			if x.Name != c.id || len(x.Tags) != 0 {
				t.Fatal(x)
			}
		})
	}
}

func TestWorldOpenPorts(t *testing.T) {
	for _, tc := range []struct {
		name, proto, cidr string
		lo, hi            int32
		want              []string
	}{
		{"ssh", "tcp", "0.0.0.0/0", 22, 22, []string{"22"}},
		{"web range", "tcp", "0.0.0.0/0", 80, 90, []string{"80-90"}}, {"range", "tcp", "0.0.0.0/0", 0, 65535, []string{"22", "3389", "all"}}, {"allipv6", "-1", "::/0", 0, 0, []string{"all"}}, {"private", "tcp", "10.0.0.0/8", 22, 22, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := types.IpPermission{IpProtocol: aws.String(tc.proto), FromPort: aws.Int32(tc.lo), ToPort: aws.Int32(tc.hi)}
			if tc.cidr == "::/0" {
				p.Ipv6Ranges = []types.Ipv6Range{{CidrIpv6: aws.String(tc.cidr)}}
			} else {
				p.IpRanges = []types.IpRange{{CidrIp: aws.String(tc.cidr)}}
			}
			if got := worldPorts([]types.IpPermission{p}); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%v != %v", got, tc.want)
			}
		})
	}
}
func TestErrorsPreserveCompletedResources(t *testing.T) {
	ctx := context.Background()
	s := Scope{}
	for _, m := range []string{"Instances", "Volumes", "Snapshots", "NatGateways", "SecurityGroups", "Images"} {
		f := &fakeEC2{fail: m, failAfter: 1}
		var r []model.Resource
		var e error
		switch m {
		case "Instances":
			r, e = collectInstances(ctx, f, s)
		case "Volumes":
			r, e = collectVolumes(ctx, f, s)
		case "Snapshots", "Images":
			r, e = collectSnapshots(ctx, f, s)
		case "NatGateways":
			r, e = collectNatGateways(ctx, f, s)
		case "SecurityGroups":
			r, e = collectSecurityGroups(ctx, f, s)
		}
		if e == nil || (m == "Images" && r != nil) || (m != "Images" && len(r) == 0) {
			t.Fatalf("%s: %v %v", m, r, e)
		}
	}
}
