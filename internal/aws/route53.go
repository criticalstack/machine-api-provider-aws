package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
)

type Route53Client struct {
	*route53.Route53

	limit *rate.Limiter
}

func NewRoute53Client(cfg *aws.Config) *Route53Client {
	return &Route53Client{
		Route53: route53.New(session.New(cfg)),
		limit:   rate.NewLimiter(5, 5),
	}
}

func (r *Route53Client) LookupZoneID(ctx context.Context, name string) (string, error) {
	zones := make(map[string]string)
	if err := r.ListHostedZonesPagesWithContext(ctx, &route53.ListHostedZonesInput{}, func(page *route53.ListHostedZonesOutput, lastPage bool) bool {
		for _, zone := range page.HostedZones {
			zones[strings.TrimSuffix(aws.StringValue(zone.Name), ".")] = strings.TrimPrefix(aws.StringValue(zone.Id), "/hostedzone/")
		}
		return !lastPage
	}); err != nil {
		return "", err
	}
	parts := strings.SplitAfterN(name, ".", 2)
	if len(parts) != 2 {
		return "", errors.Errorf("invalid name: %#v", name)
	}
	zone, ok := zones[parts[1]]
	if !ok {
		return "", errors.Errorf("cannot determine HostedZoneId for name: %#v", name)
	}
	return zone, nil
}

func (r *Route53Client) List(ctx context.Context, hostedZoneID, name string) ([]string, error) {
	addrs := make([]string, 0)
	if err := r.ListResourceRecordSetsPagesWithContext(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(hostedZoneID),
		StartRecordName: aws.String(name),
	}, func(page *route53.ListResourceRecordSetsOutput, lastPage bool) bool {
		for _, rs := range page.ResourceRecordSets {
			if strings.TrimSuffix(aws.StringValue(rs.Name), ".") != name {
				return false
			}
			for _, rr := range rs.ResourceRecords {
				addrs = append(addrs, aws.StringValue(rr.Value))
			}
		}
		return !lastPage
	}); err != nil {
		return nil, err
	}
	return addrs, nil
}

func (r *Route53Client) Update(ctx context.Context, hostedZoneID, name string, addrs []string) error {
	records := make([]*route53.ResourceRecord, 0)
	for _, addr := range addrs {
		records = append(records, &route53.ResourceRecord{
			Value: aws.String(addr)},
		)
	}
	if err := r.limit.Wait(ctx); err != nil {
		return err
	}
	_, err := r.ChangeResourceRecordSetsWithContext(ctx, &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("UPSERT"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name:            aws.String(name),
						ResourceRecords: records,
						TTL:             aws.Int64(60),
						Type:            aws.String("A"),
					},
				},
			},
		},
		HostedZoneId: aws.String(hostedZoneID),
	})
	return err
}

func ParseDomain(name string) (string, error) {
	parts := strings.SplitAfterN(name, ".", 2)
	if len(parts) != 2 {
		return "", errors.Errorf("invalid name: %#v", name)
	}
	return parts[1], nil
}
