package aws

import (
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
)

var re = regexp.MustCompile(`[a-z]$`)

func ParseRegionFromAZ(s string) string {
	return string(re.ReplaceAll([]byte(s), []byte("")))
}

func LookupRegion() (string, error) {
	return ec2metadata.New(session.New()).Region()
}

var providerIDRegex = regexp.MustCompile("^[^:]+://.*[^/]$")

type ProviderID struct {
	AvailabilityZone string
	Region           string
	InstanceID       string
}

func ParseProviderID(s string) (*ProviderID, error) {
	if !VerifyProviderID(s) {
		return nil, errors.Errorf("invalid ProviderID: %q", s)
	}
	ss := strings.Split(s, "/")
	p := &ProviderID{
		AvailabilityZone: ss[len(ss)-2],
		InstanceID:       ss[len(ss)-1],
	}
	p.Region = ParseRegionFromAZ(p.AvailabilityZone)
	return p, nil
}

func VerifyProviderID(s string) bool {
	return providerIDRegex.MatchString(s)
}

func init() {
	rand.Seed(time.Now().Unix())
}

func random(ss []string) string {
	return ss[rand.Intn(len(ss))]
}
