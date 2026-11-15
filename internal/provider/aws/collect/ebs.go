package collect

import (
	"context"

	"lookr/internal/model"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type volumesAPI interface {
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
}
type snapshotsAPI interface {
	DescribeSnapshots(context.Context, *ec2.DescribeSnapshotsInput, ...func(*ec2.Options)) (*ec2.DescribeSnapshotsOutput, error)
}
type imagesAPI interface {
	DescribeImages(context.Context, *ec2.DescribeImagesInput, ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error)
}

func collectVolumes(ctx context.Context, api interface {
	volumesAPI
	instancesAPI
}, scope Scope) ([]model.Resource, error) {
	states := map[string]string{}
	ip := ec2.NewDescribeInstancesPaginator(api, &ec2.DescribeInstancesInput{})
	for ip.HasMorePages() {
		page, e := ip.NextPage(ctx)
		if e != nil {
			return nil, e
		}
		for _, res := range page.Reservations {
			for _, x := range res.Instances {
				if x.State != nil {
					states[aws.ToString(x.InstanceId)] = string(x.State.Name)
				}
			}
		}
	}
	var out []model.Resource
	pager := ec2.NewDescribeVolumesPaginator(api, &ec2.DescribeVolumesInput{})
	for pager.HasMorePages() {
		page, e := pager.NextPage(ctx)
		if e != nil {
			return out, e
		}
		for _, x := range page.Volumes {
			id, state := "", ""
			if len(x.Attachments) > 0 {
				id = aws.ToString(x.Attachments[0].InstanceId)
				state = attachmentState(x.Attachments, states)
			}
			cfg := map[string]any{"size_gb": int(aws.ToInt32(x.Size)), "volume_type": string(x.VolumeType), "iops": int(aws.ToInt32(x.Iops)), "throughput": int(aws.ToInt32(x.Throughput)), "encrypted": aws.ToBool(x.Encrypted), "state": string(x.State), "attached_instance_id": id, "attached_instance_state": state, "attachment_count": len(x.Attachments)}
			if x.Encrypted == nil {
				delete(cfg, "encrypted")
			}
			r := resource(scope, "aws.ebs.volume", "ebs", aws.ToString(x.VolumeId), "volume", tagsEC2(x.Tags), cfg)
			r.CreatedAt = x.CreateTime
			out = append(out, r)
		}
	}
	return out, nil
}
func collectSnapshots(ctx context.Context, api interface {
	snapshotsAPI
	volumesAPI
	imagesAPI
}, scope Scope) ([]model.Resource, error) {
	vols := map[string]bool{}
	vp := ec2.NewDescribeVolumesPaginator(api, &ec2.DescribeVolumesInput{})
	for vp.HasMorePages() {
		page, e := vp.NextPage(ctx)
		if e != nil {
			return nil, e
		}
		for _, x := range page.Volumes {
			vols[aws.ToString(x.VolumeId)] = true
		}
	}
	used := map[string]bool{}
	ip := ec2.NewDescribeImagesPaginator(api, &ec2.DescribeImagesInput{Owners: []string{"self"}, IncludeDisabled: aws.Bool(true)})
	for ip.HasMorePages() {
		page, e := ip.NextPage(ctx)
		if e != nil {
			return nil, e
		}
		for _, im := range page.Images {
			for _, b := range im.BlockDeviceMappings {
				if b.Ebs != nil {
					used[aws.ToString(b.Ebs.SnapshotId)] = true
				}
			}
		}
	}
	var out []model.Resource
	pager := ec2.NewDescribeSnapshotsPaginator(api, &ec2.DescribeSnapshotsInput{OwnerIds: []string{"self"}})
	for pager.HasMorePages() {
		page, e := pager.NextPage(ctx)
		if e != nil {
			return out, e
		}
		for _, x := range page.Snapshots {
			cfg := map[string]any{"volume_id": aws.ToString(x.VolumeId), "volume_size_gb": int(aws.ToInt32(x.VolumeSize)), "state": string(x.State), "encrypted": aws.ToBool(x.Encrypted), "description": aws.ToString(x.Description), "storage_tier": string(x.StorageTier), "source_volume_exists": vols[aws.ToString(x.VolumeId)], "used_by_ami": used[aws.ToString(x.SnapshotId)]}
			if x.Encrypted == nil {
				delete(cfg, "encrypted")
			}
			r := resource(scope, "aws.ebs.snapshot", "snapshot", aws.ToString(x.SnapshotId), "snapshot", tagsEC2(x.Tags), cfg)
			r.ARN = "arn:aws:ec2:" + scope.Region + "::snapshot/" + aws.ToString(x.SnapshotId)
			r.CreatedAt = x.StartTime
			out = append(out, r)
		}
	}
	return out, nil
}

// Every attachment must be known stopped before a shared volume is idle.
func attachmentState(attachments []types.VolumeAttachment, states map[string]string) string {
	allStopped := len(attachments) > 0
	active := ""
	for _, a := range attachments {
		state := states[aws.ToString(a.InstanceId)]
		if state != "stopped" {
			allStopped = false
		}
		if state == "running" {
			active = "running"
		} else if state == "pending" && active == "" {
			active = "pending"
		}
	}
	if active != "" {
		return active
	}
	if allStopped {
		return "stopped"
	}
	return "unknown"
}
