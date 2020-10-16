package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
)

func AttachInstance(ctx context.Context, cfg *aws.Config, groupName, instanceID string) error {
	svc := autoscaling.New(session.New(cfg))
	_, err := svc.AttachInstancesWithContext(ctx, &autoscaling.AttachInstancesInput{
		AutoScalingGroupName: aws.String(groupName),
		InstanceIds:          aws.StringSlice([]string{instanceID}),
	})
	return err
}

func DetachInstance(ctx context.Context, cfg *aws.Config, groupName, instanceID string) error {
	svc := autoscaling.New(session.New(cfg))
	_, err := svc.DetachInstancesWithContext(ctx, &autoscaling.DetachInstancesInput{
		AutoScalingGroupName: aws.String(groupName),
		InstanceIds:          aws.StringSlice([]string{instanceID}),
	})
	return err
}

func DescribeGroup(ctx context.Context, cfg *aws.Config, groupName, instanceID string) error {
	svc := autoscaling.New(session.New(cfg))
	resp, err := svc.DescribeAutoScalingGroupsWithContext(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: aws.StringSlice([]string{groupName}),
		MaxRecords:            aws.Int64(1),
	})
	if err != nil {
		return err
	}
	fmt.Printf("resp = %+v\n", resp)

	return err
}

func DescribeAutoscalingInstances(ctx context.Context, cfg *aws.Config, instanceID string) (string, error) {
	svc := autoscaling.New(session.New(cfg))
	resp, err := svc.DescribeAutoScalingInstancesWithContext(ctx, &autoscaling.DescribeAutoScalingInstancesInput{
		InstanceIds: aws.StringSlice([]string{instanceID}),
		MaxRecords:  aws.Int64(1),
	})
	if err != nil {
		return "", err
	}
	for _, instance := range resp.AutoScalingInstances {
		return aws.StringValue(instance.AutoScalingGroupName), nil
	}
	return "", nil
}
