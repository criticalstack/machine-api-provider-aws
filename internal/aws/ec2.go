package aws

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"

	infrav1 "github.com/criticalstack/machine-api-provider-aws/api/v1alpha1"
)

func LaunchInstance(ctx context.Context, cfg *aws.Config, m *infrav1.AWSMachine, userData string) (*ec2.Instance, error) {
	input := &ec2.RunInstancesInput{
		BlockDeviceMappings: convertBlockDevices(m.Spec.BlockDevices),
		ImageId:             aws.String(m.Spec.AMI),
		InstanceType:        aws.String(m.Spec.InstanceType),
		KeyName:             aws.String(m.Spec.KeyName),
		MaxCount:            aws.Int64(1),
		MinCount:            aws.Int64(1),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String("instance"),
				Tags:         convertTags(m.Spec.Tags),
			},
		},
		UserData: aws.String(userData),
	}
	if strings.HasPrefix(m.Spec.IAMInstanceProfile, "arn") {
		input.IamInstanceProfile = &ec2.IamInstanceProfileSpecification{
			Arn: aws.String(m.Spec.IAMInstanceProfile),
		}
	} else {
		input.IamInstanceProfile = &ec2.IamInstanceProfileSpecification{
			Name: aws.String(m.Spec.IAMInstanceProfile),
		}
	}
	svc := ec2.New(session.New(cfg))
	if len(m.Spec.SecurityGroupIDs) != 0 {
		input.SecurityGroupIds = aws.StringSlice(m.Spec.SecurityGroupIDs)
	}
	if len(m.Spec.SecurityGroupNames) != 0 {
		resp, err := svc.DescribeSecurityGroupsWithContext(ctx, &ec2.DescribeSecurityGroupsInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("group-name"),
					Values: aws.StringSlice(m.Spec.SecurityGroupNames),
				},
			},
		})
		if err != nil {
			return nil, err
		}
		for _, sg := range resp.SecurityGroups {
			input.SecurityGroupIds = append(input.SecurityGroupIds, sg.GroupId)
		}
	}
	sinput := &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: aws.StringSlice([]string{m.Spec.VPCID}),
			},
			{
				Name:   aws.String("state"),
				Values: aws.StringSlice([]string{ec2.SubnetStateAvailable}),
			},
		},
	}
	if m.Spec.AvailabilityZone != "" {
		sinput.Filters = append(sinput.Filters, &ec2.Filter{
			Name:   aws.String("availability-zone"),
			Values: aws.StringSlice([]string{m.Spec.AvailabilityZone}),
		})
	}
	if len(m.Spec.SubnetIDs) != 0 {
		sinput.Filters = append(sinput.Filters, &ec2.Filter{
			Name:   aws.String("subnet-id"),
			Values: aws.StringSlice(m.Spec.SubnetIDs),
		})
	}
	sresp, err := svc.DescribeSubnetsWithContext(ctx, sinput)
	if err != nil {
		return nil, err
	}
	if len(sresp.Subnets) == 0 {
		return nil, errors.Errorf("cannot determine subnet from VPC: %#v", m.Spec.VPCID)
	}
	subnets := make([]string, 0)
	for _, subnet := range sresp.Subnets {
		if aws.BoolValue(subnet.MapPublicIpOnLaunch) == m.Spec.PublicIP {
			if aws.Int64Value(subnet.AvailableIpAddressCount) < 1 {
				continue
			}
			subnets = append(subnets, aws.StringValue(subnet.SubnetId))
		}
	}
	if len(subnets) == 0 {
		return nil, errors.Errorf("cannot determine subnet from VPC: %#v", m.Spec.VPCID)
	}
	input.SubnetId = aws.String(random(subnets))
	if m.Spec.AvailabilityZone != "" {
		input.Placement = &ec2.Placement{
			AvailabilityZone: aws.String(m.Spec.AvailabilityZone),
		}
	}
	resp, err := svc.RunInstancesWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	for _, instance := range resp.Instances {
		return instance, nil
	}
	return nil, errors.New("no instances")
}

func convertBlockDevices(blockDevices []infrav1.AWSBlockDeviceMapping) []*ec2.BlockDeviceMapping {
	blockDeviceMappings := make([]*ec2.BlockDeviceMapping, 0)
	for _, b := range blockDevices {
		blockDeviceMappings = append(blockDeviceMappings, &ec2.BlockDeviceMapping{
			DeviceName: aws.String(b.DeviceName),
			Ebs: &ec2.EbsBlockDevice{
				VolumeSize: aws.Int64(b.VolumeSize),
				VolumeType: aws.String(b.VolumeType),
				Encrypted:  aws.Bool(b.Encrypted),
			},
		})
	}
	return blockDeviceMappings
}

func convertTags(tags map[string]string) []*ec2.Tag {
	ec2tags := make([]*ec2.Tag, 0)
	for key, value := range tags {
		ec2tags = append(ec2tags, &ec2.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}
	return ec2tags
}

func TerminateInstance(ctx context.Context, cfg *aws.Config, instanceID string) error {
	svc := ec2.New(session.New(cfg))
	_, err := svc.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	})
	return err
}

func DescribeInstanceStatus(ctx context.Context, cfg *aws.Config, instanceID string) (string, error) {
	instance, exists, err := DescribeInstance(ctx, cfg, instanceID)
	if err != nil {
		return "", err
	}
	if !exists {
		return ec2.InstanceStateNameTerminated, nil
	}
	return aws.StringValue(instance.State.Name), nil
}

func DescribeSubnets(ctx context.Context, cfg *aws.Config, vpcID string) ([]string, error) {
	svc := ec2.New(session.New(cfg))
	resp, err := svc.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("vpc-id"),
				Values: []*string{
					aws.String(vpcID),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	subnets := make([]string, 0)
	for _, subnet := range resp.Subnets {
		subnets = append(subnets, aws.StringValue(subnet.SubnetId))
	}
	return subnets, nil
}

const (
	InstanceNotFound = "InvalidInstanceID.NotFound"
)

func DescribeInstance(ctx context.Context, cfg *aws.Config, instanceID string) (*ec2.Instance, bool, error) {
	svc := ec2.New(session.New(cfg))
	resp, err := svc.DescribeInstancesWithContext(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case InstanceNotFound:
				return nil, false, nil
			}
		}
		return nil, false, err
	}
	if len(resp.Reservations) > 0 && len(resp.Reservations[0].Instances) > 0 {
		return resp.Reservations[0].Instances[0], true, nil
	}
	return nil, false, nil
}

func DescribeInstanceTypes(ctx context.Context, cfg *aws.Config, instanceType, az string) (bool, error) {
	svc := ec2.New(session.New(cfg))

	resp, err := svc.DescribeInstanceTypeOfferingsWithContext(ctx, &ec2.DescribeInstanceTypeOfferingsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("instance-type"),
				Values: aws.StringSlice([]string{instanceType}),
			},
			{
				Name:   aws.String("location"),
				Values: aws.StringSlice([]string{az}),
			},
		},
		MaxResults:   aws.Int64(1000),
		LocationType: aws.String(ec2.LocationTypeAvailabilityZone),
	})
	if err != nil {
		return false, err
	}
	if len(resp.InstanceTypeOfferings) > 0 {
		return true, nil
	}
	return false, nil

}

func DescribeSubnet(ctx context.Context, cfg *aws.Config, subnetID string) (*ec2.Subnet, error) {
	svc := ec2.New(session.New(cfg))
	resp, err := svc.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("subnet-id"),
				Values: aws.StringSlice([]string{subnetID}),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Subnets) == 0 {
		return nil, errors.Errorf("cannot find subnet: %#v", subnetID)
	}
	return resp.Subnets[0], nil
}

func DescribeVolume(ctx context.Context, cfg *aws.Config, volumeID string) (*ec2.Volume, error) {
	svc := ec2.New(session.New(cfg))
	resp, err := svc.DescribeVolumesWithContext(ctx, &ec2.DescribeVolumesInput{
		VolumeIds: aws.StringSlice([]string{volumeID}),
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Volumes) == 0 {
		return nil, errors.Errorf("cannot find volume: %#v", volumeID)
	}
	return resp.Volumes[0], nil
}

func DescribeUserData(ctx context.Context, cfg *aws.Config, instanceID string) ([]byte, error) {
	svc := ec2.New(session.New(cfg))
	resp, err := svc.DescribeInstanceAttributeWithContext(ctx, &ec2.DescribeInstanceAttributeInput{
		Attribute:  aws.String("userData"),
		InstanceId: aws.String(instanceID),
	})
	if err != nil {
		return nil, err
	}
	userData := aws.StringValue(resp.UserData.Value)
	return base64.StdEncoding.DecodeString(userData)
}
