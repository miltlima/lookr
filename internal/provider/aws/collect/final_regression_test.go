package collect

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	et "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rt "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"lookr/internal/model"
	"lookr/internal/provider/aws/checks"
	"lookr/internal/provider/aws/usage"
)

func TestFinalPartialInventorySurvivesLaterError(t *testing.T) {
	for _, op := range []string{"Instances", "Volumes", "Snapshots", "NatGateways", "SecurityGroups"} {
		f := &fakeEC2{fail: op, failAfter: 1}
		var rows []model.Resource
		var err error
		switch op {
		case "Instances":
			rows, err = collectInstances(context.Background(), f, Scope{})
		case "Volumes":
			rows, err = collectVolumes(context.Background(), f, Scope{})
		case "Snapshots":
			rows, err = collectSnapshots(context.Background(), f, Scope{})
		case "NatGateways":
			rows, err = collectNatGateways(context.Background(), f, Scope{})
		case "SecurityGroups":
			rows, err = collectSecurityGroups(context.Background(), f, Scope{})
		}
		if err == nil || len(rows) == 0 {
			t.Errorf("%s erased completed page: %v %v", op, rows, err)
		}
	}
	rows, err := collectLoadBalancers(context.Background(), fakeV2{}, fakeV1{fail: "DescribeLoadBalancers"}, Scope{})
	if err == nil || len(rows) != 3 {
		t.Errorf("classic failure lost v2: %d %v", len(rows), err)
	}
	rows, err = collectLoadBalancers(context.Background(), fakeV2{fail: "DescribeLoadBalancers"}, fakeV1{}, Scope{})
	if err == nil || len(rows) != 3 {
		t.Errorf("v2 failure lost classic: %d %v", len(rows), err)
	}
}

type finalEC2 struct {
	fakeEC2
	instances []et.Instance
	volumes   []et.Volume
}

func (f *finalEC2) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{Reservations: []et.Reservation{{Instances: f.instances}}}, nil
}
func (f *finalEC2) DescribeVolumes(context.Context, *ec2.DescribeVolumesInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	return &ec2.DescribeVolumesOutput{Volumes: f.volumes}, nil
}

type finalRDS struct{}

func (finalRDS) DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return &rds.DescribeDBInstancesOutput{DBInstances: []rt.DBInstance{{}}}, nil
}

func TestFinalMissingSecurityFactsStayUnknown(t *testing.T) {
	for _, metadata := range []*et.InstanceMetadataOptionsResponse{nil, {HttpTokens: et.HttpTokensStateOptional, HttpEndpoint: et.InstanceMetadataEndpointStateDisabled}} {
		f := &finalEC2{instances: []et.Instance{{InstanceId: aws.String("i-1"), MetadataOptions: metadata}}}
		r, err := collectInstances(context.Background(), f, Scope{})
		if err != nil {
			t.Fatal(err)
		}
		for _, finding := range checks.Run(r[0], time.Now()) {
			if finding.Check == "ec2.imdsv1" {
				t.Errorf("unknown or disabled endpoint manufactured finding: %+v", metadata)
			}
		}
	}
	f := &finalEC2{volumes: []et.Volume{{VolumeId: aws.String("v")}}}
	rows, err := collectVolumes(context.Background(), f, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rows[0].Config["encrypted"]; ok {
		t.Error("nil volume encryption became observed false")
	}
	rows, err = collectDBInstances(context.Background(), finalRDS{}, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"storage_encrypted", "backup_retention_days", "publicly_accessible"} {
		if _, ok := rows[0].Config[key]; ok {
			t.Errorf("nil RDS %s became observed value", key)
		}
	}
	if fs := checks.Run(rows[0], time.Now()); len(fs) != 0 {
		t.Errorf("nil RDS config manufactured findings: %v", fs)
	}
}

func TestFinalMultiAttachUsageIndependentOfOrder(t *testing.T) {
	for _, tc := range []struct {
		ids  []string
		want model.Verdict
	}{{[]string{"stopped", "running"}, model.VerdictInUse}, {[]string{"running", "stopped"}, model.VerdictInUse}, {[]string{"stopped", "stopped"}, model.VerdictIdle}, {[]string{"stopped", "missing"}, model.VerdictUnknown}} {
		f := &finalEC2{instances: []et.Instance{{InstanceId: aws.String("stopped"), State: &et.InstanceState{Name: et.InstanceStateNameStopped}}, {InstanceId: aws.String("running"), State: &et.InstanceState{Name: et.InstanceStateNameRunning}}}}
		v := et.Volume{VolumeId: aws.String("v"), State: et.VolumeStateInUse}
		for _, id := range tc.ids {
			v.Attachments = append(v.Attachments, et.VolumeAttachment{InstanceId: aws.String(id)})
		}
		f.volumes = []et.Volume{v}
		rows, err := collectVolumes(context.Background(), f, Scope{})
		if err != nil {
			t.Fatal(err)
		}
		usage.Apply(&rows[0], usage.Result{}, time.Now(), 30)
		if rows[0].Usage.Verdict != tc.want {
			t.Errorf("%v: %+v want %s", tc.ids, rows[0].Usage, tc.want)
		}
	}
}
