/*
Copyright 2020 Critical Stack, LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ec2"
	machinev1 "github.com/criticalstack/machine-api/api/v1alpha1"
	mapierrors "github.com/criticalstack/machine-api/errors"
	"github.com/criticalstack/machine-api/util"
	"github.com/criticalstack/machine-api/util/patch"
	"github.com/go-logr/logr"
	"github.com/labstack/gommon/log"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	infrav1 "github.com/criticalstack/machine-api-provider-aws/api/v1alpha1"
	"github.com/criticalstack/machine-api-provider-aws/internal"
	awsutil "github.com/criticalstack/machine-api-provider-aws/internal/aws"
)

// AWSMachineReconciler reconciles a AWSMachine object
type AWSMachineReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	config *rest.Config
}

func (r *AWSMachineReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	r.config = mgr.GetConfig()
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&infrav1.AWSMachine{}).
		Watches(
			&source.Kind{Type: &machinev1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("AWSMachine")),
			},
		).
		Complete(r)
}

// +kubebuilder:rbac:groups=infrastructure.crit.sh,resources=awsmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.crit.sh,resources=awsmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=machine.crit.sh,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=machine.crit.sh,resources=configs;configs/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete

func (r *AWSMachineReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.Background()
	log := r.Log.WithValues("awsmachine", req.NamespacedName)

	am := &infrav1.AWSMachine{}
	if err := r.Get(ctx, req.NamespacedName, am); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deleted machines
	if !am.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := r.reconcileDelete(ctx, am); err != nil {
			log.Error(err, "cannot delete node, may already be deleted")
		}
		controllerutil.RemoveFinalizer(am, infrav1.MachineFinalizer)
		if err := r.Update(ctx, am); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if am.Status.FailureMessage != nil {
		log.Info("machine has failure reason/message", "reason", am.Status.FailureReason, "message", am.Status.FailureMessage)
		return ctrl.Result{}, nil
	}

	m, err := util.GetOwnerMachine(ctx, r.Client, am.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if m == nil {
		log.Info("Machine Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("machine", m.Name)

	// Patch any changes to Machine object on each reconciliation.
	patchHelper, err := patch.NewHelper(am, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, am); err != nil {
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// If the AWSMachine doesn't have a finalizer, add one.
	controllerutil.AddFinalizer(am, infrav1.MachineFinalizer)

	if am.Spec.ProviderID != nil {
		log.Info("machine already exists")
		if err := r.reconcileStatus(ctx, am); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	cfg := &machinev1.Config{}
	if err := r.Get(ctx, client.ObjectKey{Name: m.Spec.ConfigRef.Name, Namespace: m.Namespace}, cfg); err != nil {
		return ctrl.Result{}, err
	}

	if !cfg.Status.Ready {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	s := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: *cfg.Status.DataSecretName, Namespace: m.Namespace}, s); err != nil {
		return ctrl.Result{}, err
	}
	userData, ok := s.Data["cloud-config"]
	if !ok {
		return ctrl.Result{}, errors.Errorf("secret %q missing cloud-config", *cfg.Status.DataSecretName)
	}

	awscfg := &aws.Config{Region: aws.String(am.Spec.Region)}

	if am.Spec.SecretRef != nil {
		s := &corev1.Secret{}
		if err := r.Get(ctx, client.ObjectKey{Name: am.Spec.SecretRef.Name, Namespace: m.Namespace}, s); err != nil {
			return ctrl.Result{}, err
		}
		id := string(s.Data["AWS_ACCESS_KEY_ID"])
		secret := string(s.Data["AWS_SECRET_ACCESS_KEY"])
		if id != "" && secret != "" {
			awscfg.Credentials = credentials.NewStaticCredentials(id, secret, "")
		}
	}
	data, err := internal.Gzip(userData)
	if err != nil {
		return ctrl.Result{}, err
	}
	instance, err := awsutil.LaunchInstance(ctx, awscfg, am, base64.StdEncoding.EncodeToString(data))
	if err != nil {
		m.Status.SetFailure(mapierrors.CreateMachineError, err.Error())
		return ctrl.Result{}, err
	}
	am.Spec.ProviderID = pointer.StringPtr(fmt.Sprintf("aws:///%s/%s", aws.StringValue(instance.Placement.AvailabilityZone), aws.StringValue(instance.InstanceId)))
	am.Status.Addresses = getInstanceAddresses(instance)
	am.Status.Ready = true
	if err := r.reconcileStatus(ctx, am); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func getInstanceAddresses(instance *ec2.Instance) machinev1.MachineAddresses {
	addresses := make([]machinev1.MachineAddress, 0)
	for _, eni := range instance.NetworkInterfaces {
		privateDNSAddress := machinev1.MachineAddress{
			Type:    machinev1.MachineInternalDNS,
			Address: aws.StringValue(eni.PrivateDnsName),
		}
		privateIPAddress := machinev1.MachineAddress{
			Type:    machinev1.MachineInternalIP,
			Address: aws.StringValue(eni.PrivateIpAddress),
		}
		addresses = append(addresses, privateDNSAddress, privateIPAddress)

		// An elastic IP is attached if association is non nil pointer
		if eni.Association != nil {
			publicDNSAddress := machinev1.MachineAddress{
				Type:    machinev1.MachineExternalDNS,
				Address: aws.StringValue(eni.Association.PublicDnsName),
			}
			publicIPAddress := machinev1.MachineAddress{
				Type:    machinev1.MachineExternalIP,
				Address: aws.StringValue(eni.Association.PublicIp),
			}
			addresses = append(addresses, publicDNSAddress, publicIPAddress)
		}
	}
	return addresses
}

func (r *AWSMachineReconciler) reconcileDelete(ctx context.Context, am *infrav1.AWSMachine) error {
	p, err := awsutil.ParseProviderID(*am.Spec.ProviderID)
	if err != nil {
		return err
	}
	awscfg := &aws.Config{Region: aws.String(p.Region)}
	state, err := awsutil.DescribeInstanceStatus(ctx, awscfg, p.InstanceID)
	if err != nil {
		return err
	}
	switch state {
	case ec2.InstanceStateNamePending:
		return errors.Wrapf(&mapierrors.RequeueAfterError{RequeueAfter: 10 * time.Second}, "machine %q pending, waiting until ready to delete", am.Name)
	case ec2.InstanceStateNameStopping:
		return errors.Wrapf(&mapierrors.RequeueAfterError{RequeueAfter: 10 * time.Second}, "machine %q stopping, waiting until stopped to delete", am.Name)
	case ec2.InstanceStateNameRunning, ec2.InstanceStateNameStopped:
		log.Info("terminate running instance", "InstanceID", p.InstanceID)
		if err := awsutil.TerminateInstance(ctx, awscfg, p.InstanceID); err != nil {
			return err
		}
		return errors.Wrapf(&mapierrors.RequeueAfterError{RequeueAfter: 10 * time.Second}, "machine %q terminating", am.Name)
	case ec2.InstanceStateNameShuttingDown:
		return errors.Wrapf(&mapierrors.RequeueAfterError{RequeueAfter: 10 * time.Second}, "machine %q terminating", am.Name)
	case ec2.InstanceStateNameTerminated:
		return nil
	default:
		return errors.Errorf("machine %q has unknown state %q", am.Name, state)
	}
}

func (r *AWSMachineReconciler) reconcileStatus(ctx context.Context, am *infrav1.AWSMachine) error {
	p, err := awsutil.ParseProviderID(*am.Spec.ProviderID)
	if err != nil {
		return err
	}
	awscfg := &aws.Config{Region: aws.String(p.Region)}
	state, err := awsutil.DescribeInstanceStatus(ctx, awscfg, p.InstanceID)
	if err != nil {
		return err
	}
	am.Status.InstanceState = state
	if !am.Status.Ready {
		instance, _, err := awsutil.DescribeInstance(ctx, awscfg, p.InstanceID)
		if err != nil {
			//m.Status.SetFailure(mapierrors.CreateMachineError, err.Error())
			return err
		}
		am.Status.Addresses = getInstanceAddresses(instance)
		am.Status.Ready = true
	}
	return nil
}
