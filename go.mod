module github.com/criticalstack/machine-api-provider-aws

go 1.14

require (
	github.com/aws/aws-sdk-go v1.33.21
	github.com/criticalstack/crit v1.0.3
	github.com/criticalstack/machine-api v1.0.1
	github.com/go-logr/logr v0.1.0
	github.com/go-openapi/spec v0.19.3
	github.com/labstack/gommon v0.3.0
	github.com/onsi/ginkgo v1.12.1
	github.com/onsi/gomega v1.10.1
	github.com/pkg/errors v0.9.1
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	k8s.io/utils v0.0.0-20200619165400-6e3d28b6ed19
	sigs.k8s.io/controller-runtime v0.6.0
)
