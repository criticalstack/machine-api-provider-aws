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
	"encoding/json"
	"errors"

	"github.com/aws/aws-sdk-go/aws"
	nodeutil "github.com/criticalstack/crit/pkg/kubernetes/util/node"
	machinev1 "github.com/criticalstack/machine-api/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	infrav1 "github.com/criticalstack/machine-api-provider-aws/api/v1alpha1"
	awsutil "github.com/criticalstack/machine-api-provider-aws/internal/aws"
)

// NodeReconciler reconciles a corev1.Node object and creates AWSMachine
// objects for nodes where one does not exist. This ensures that even nodes
// that were created outside of the machine-api are described by Kubernetes
// resources.
type NodeReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	config *rest.Config
}

func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	r.config = mgr.GetConfig()
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithOptions(options).
		Complete(r)
}

// +kubebuilder:rbac:groups=infrastructure.crit.sh,resources=awsmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.crit.sh,resources=awsmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=machine.crit.sh,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=machine.crit.sh,resources=configs;configs/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;create;update;patch;delete

func (r *NodeReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("node", req.NamespacedName)

	n := &corev1.Node{}
	if err := r.Get(ctx, req.NamespacedName, n); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// TODO: branch here on node NotReady and check provider api for terminated
	// machines, and delete machine if necessary (since no longer valid

	annotations := n.GetAnnotations()
	if _, ok := annotations[infrav1.NodeOwnerLabelName]; !ok {
		log.Info("awsmachine label not found")
		if err := r.ensureAWSMachineForNode(ctx, n); err != nil {
			return ctrl.Result{}, err
		}
	}
	if refData, ok := annotations[machinev1.NodeOwnerLabelName]; ok {
		var ref corev1.ObjectReference
		if err := json.Unmarshal([]byte(refData), &ref); err != nil {
			return ctrl.Result{}, err
		}
		awsRefData, ok := annotations[infrav1.NodeOwnerLabelName]
		if !ok {
			return ctrl.Result{}, errors.New("cannot find AWSMachine, missing infra annotation")
		}
		var amRef corev1.ObjectReference
		if err := json.Unmarshal([]byte(awsRefData), &amRef); err != nil {
			return ctrl.Result{}, err
		}
		//log.Info("machine label not found")
		am := &infrav1.AWSMachine{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: amRef.Name}, am); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.ensureMachineHasInfraRef(ctx, am, ref); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *NodeReconciler) ensureMachineHasInfraRef(ctx context.Context, am *infrav1.AWSMachine, ref corev1.ObjectReference) error {
	m := &machinev1.Machine{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: metav1.NamespaceSystem, Name: ref.Name}, m); err != nil {
		return err
	}
	if m.Spec.InfrastructureRef.Kind == "AWSMachine" && m.Spec.InfrastructureRef.Name == am.Name {
		return nil
	}
	m.Spec.InfrastructureRef = corev1.ObjectReference{
		APIVersion: am.APIVersion,
		Kind:       "AWSMachine",
		Name:       am.ObjectMeta.Name,
		Namespace:  am.Namespace,
	}
	if err := r.Update(ctx, m); err != nil {
		return err
	}
	return nil
}

func (r *NodeReconciler) ensureAWSMachineForNode(ctx context.Context, n *corev1.Node) error {
	log := r.Log.WithValues("node", n.Name)

	annotations := n.GetAnnotations()
	if _, ok := annotations[infrav1.NodeOwnerLabelName]; ok {
		return nil
	}
	machines := &infrav1.AWSMachineList{}
	if err := r.List(ctx, machines); err != nil {
		return err
	}
	for _, m := range machines.Items {
		if m.Spec.ProviderID != nil && *m.Spec.ProviderID == n.Spec.ProviderID {
			log.V(1).Info("node already has a machine associated with it, only needs an annotation")
			return r.setAWSMachineAnnotation(ctx, &m, n.Name)
		}
	}
	p, err := awsutil.ParseProviderID(n.Spec.ProviderID)
	if err != nil {
		return err
	}
	awscfg := &aws.Config{Region: aws.String(p.Region)}
	instance, _, err := awsutil.DescribeInstance(ctx, awscfg, p.InstanceID)
	if err != nil {
		return err
	}
	am := &infrav1.AWSMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      n.Name,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: infrav1.AWSMachineSpec{
			ProviderID: pointer.StringPtr(n.Spec.ProviderID),
		},
		Status: infrav1.AWSMachineStatus{
			Addresses: getInstanceAddresses(instance),
		},
	}
	if err := r.Create(ctx, am); err != nil {
		return err
	}
	return r.setAWSMachineAnnotation(ctx, am, n.Name)
}

func (r *NodeReconciler) setAWSMachineAnnotation(ctx context.Context, m *infrav1.AWSMachine, name string) error {
	ref := corev1.ObjectReference{
		APIVersion: m.APIVersion,
		Kind:       "AWSMachine",
		Name:       m.ObjectMeta.Name,
		Namespace:  m.Namespace,
	}
	data, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	k, err := kubernetes.NewForConfig(r.config)
	if err != nil {
		return err
	}
	return nodeutil.PatchNode(ctx, k, name, func(n *corev1.Node) {
		annotations := n.GetAnnotations()
		annotations[infrav1.NodeOwnerLabelName] = string(data)
	})
}
