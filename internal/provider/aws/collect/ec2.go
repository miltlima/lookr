package collect

import (
	"context"
	"sort"
	"strconv"

	"lookr/internal/model"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func tagsEC2(t []types.Tag) map[string]string {
	m := map[string]string{}
	for _, x := range t {
		m[aws.ToString(x.Key)] = aws.ToString(x.Value)
	}
	return m
}

type instancesAPI interface {
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
}

type addressesAPI interface {
	DescribeAddresses(context.Context, *ec2.DescribeAddressesInput, ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error)
}
type natGatewaysAPI interface {
	DescribeNatGateways(context.Context, *ec2.DescribeNatGatewaysInput, ...func(*ec2.Options)) (*ec2.DescribeNatGatewaysOutput, error)
}
type securityGroupsAPI interface {
	DescribeSecurityGroups(context.Context, *ec2.DescribeSecurityGroupsInput, ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
}

func collectInstances(ctx context.Context, api instancesAPI, scope Scope) ([]model.Resource, error) {
	var out []model.Resource
	pager := ec2.NewDescribeInstancesPaginator(api, &ec2.DescribeInstancesInput{})
	for pager.HasMorePages() {
		page, e := pager.NextPage(ctx)
		if e != nil {
			return out, e
		}
		for _, x := range page.Reservations {
			for _, x := range x.Instances {
				groups := []string{}
				for _, g := range x.SecurityGroups {
					groups = append(groups, aws.ToString(g.GroupId))
				}
				profile := ""
				if x.IamInstanceProfile != nil {
					profile = aws.ToString(x.IamInstanceProfile.Arn)
				}
				state := ""
				if x.State != nil {
					state = string(x.State.Name)
				}
				if state == "terminated" {
					continue
				}
				cfg := map[string]any{"instance_type": string(x.InstanceType), "state": state, "platform": string(x.Platform), "private_ip": aws.ToString(x.PrivateIpAddress), "public_ip": aws.ToString(x.PublicIpAddress), "image_id": aws.ToString(x.ImageId), "vpc_id": aws.ToString(x.VpcId), "security_groups": groups, "iam_instance_profile": profile}
				if m := x.MetadataOptions; m != nil {
					if m.HttpTokens == types.HttpTokensStateRequired || m.HttpTokens == types.HttpTokensStateOptional {
						cfg["imds_v2_required"] = m.HttpTokens == types.HttpTokensStateRequired
					}
					if m.HttpEndpoint == types.InstanceMetadataEndpointStateEnabled || m.HttpEndpoint == types.InstanceMetadataEndpointStateDisabled {
						cfg["imds_endpoint_enabled"] = m.HttpEndpoint == types.InstanceMetadataEndpointStateEnabled
					}
				}
				r := resource(scope, "aws.ec2.instance", "ec2", aws.ToString(x.InstanceId), "instance", tagsEC2(x.Tags), cfg)
				r.CreatedAt = x.LaunchTime
				out = append(out, r)
			}
		}
	}
	return out, nil
}
func collectAddresses(ctx context.Context, api addressesAPI, scope Scope) ([]model.Resource, error) {
	var out []model.Resource
	page, e := api.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
	if e != nil {
		return out, e
	}
	for _, x := range page.Addresses {
		cfg := map[string]any{"public_ip": aws.ToString(x.PublicIp), "allocation_id": aws.ToString(x.AllocationId), "association_id": aws.ToString(x.AssociationId), "instance_id": aws.ToString(x.InstanceId), "network_interface_id": aws.ToString(x.NetworkInterfaceId)}
		r := resource(scope, "aws.ec2.eip", "eip", aws.ToString(x.AllocationId), "elastic-ip", tagsEC2(x.Tags), cfg)
		out = append(out, r)
	}
	return out, nil
}
func collectNatGateways(ctx context.Context, api natGatewaysAPI, scope Scope) ([]model.Resource, error) {
	var out []model.Resource
	pager := ec2.NewDescribeNatGatewaysPaginator(api, &ec2.DescribeNatGatewaysInput{})
	for pager.HasMorePages() {
		page, e := pager.NextPage(ctx)
		if e != nil {
			return out, e
		}
		for _, x := range page.NatGateways {
			if x.State == types.NatGatewayStateDeleted {
				continue
			}
			cfg := map[string]any{"state": string(x.State), "vpc_id": aws.ToString(x.VpcId), "subnet_id": aws.ToString(x.SubnetId), "connectivity_type": string(x.ConnectivityType)}
			r := resource(scope, "aws.ec2.natgateway", "nat", aws.ToString(x.NatGatewayId), "natgateway", tagsEC2(x.Tags), cfg)
			r.CreatedAt = x.CreateTime
			out = append(out, r)
		}
	}
	return out, nil
}
func collectSecurityGroups(ctx context.Context, api securityGroupsAPI, scope Scope) ([]model.Resource, error) {
	var out []model.Resource
	pager := ec2.NewDescribeSecurityGroupsPaginator(api, &ec2.DescribeSecurityGroupsInput{})
	for pager.HasMorePages() {
		page, e := pager.NextPage(ctx)
		if e != nil {
			return out, e
		}
		for _, x := range page.SecurityGroups {
			cfg := map[string]any{"vpc_id": aws.ToString(x.VpcId), "description": aws.ToString(x.Description), "ingress_rule_count": len(x.IpPermissions), "world_open_ports": worldPorts(x.IpPermissions)}
			r := resource(scope, "aws.ec2.securitygroup", "sg", aws.ToString(x.GroupId), "security-group", tagsEC2(x.Tags), cfg)
			out = append(out, r)
		}
	}
	return out, nil
}
func worldPorts(rules []types.IpPermission) []string {
	ports := map[string]bool{}
	for _, r := range rules {
		world := false
		for _, v := range r.IpRanges {
			world = world || aws.ToString(v.CidrIp) == "0.0.0.0/0"
		}
		for _, v := range r.Ipv6Ranges {
			world = world || aws.ToString(v.CidrIpv6) == "::/0"
		}
		if !world {
			continue
		}
		proto := aws.ToString(r.IpProtocol)
		if proto == "-1" {
			ports["all"] = true
			continue
		}
		if proto != "tcp" && proto != "udp" && proto != "6" && proto != "17" {
			continue
		}
		lo, hi := aws.ToInt32(r.FromPort), aws.ToInt32(r.ToPort)
		if lo == 0 && hi == 65535 {
			ports["all"] = true
		}
		if lo != hi && !(lo == 0 && hi == 65535) {
			ports[strconv.Itoa(int(lo))+"-"+strconv.Itoa(int(hi))] = true
		}
		if lo == hi {
			ports[strconv.Itoa(int(lo))] = true
		}
		for _, p := range []int32{22, 3389} {
			if lo <= p && hi >= p {
				ports[strconv.Itoa(int(p))] = true
			}
		}
	}
	out := []string{}
	for p := range ports {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
