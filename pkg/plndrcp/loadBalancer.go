package plndrcp

import (
	"context"
	"fmt"

	"github.com/plunder-app/plndr-cloud-provider/pkg/ipam"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog"
)

type plndrServices struct {
	Services []services `json:"services"`
}

type services struct {
	Vip         string `json:"vip"`
	Port        int    `json:"port"`
	UID         string `json:"uid"`
	ServiceName string `json:"serviceName"`
}

//PlndrLoadBalancer -
type plndrLoadBalancerManager struct {
	kubeClient *kubernetes.Clientset
	nameSpace  string
	configMap  string
	//serviceCidr string
}

func newLoadBalancer(kubeClient *kubernetes.Clientset, ns, cm, serviceCidr string) cloudprovider.LoadBalancer {
	return &plndrLoadBalancerManager{
		kubeClient: kubeClient,
		nameSpace:  ns,
		configMap:  cm}
}

func (plb *plndrLoadBalancerManager) EnsureLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (lbs *v1.LoadBalancerStatus, err error) {
	return plb.syncLoadBalancer(service)
}
func (plb *plndrLoadBalancerManager) UpdateLoadBalancer(ctx context.Context, clusterName string, service *v1.Service, nodes []*v1.Node) (err error) {
	_, err = plb.syncLoadBalancer(service)
	return err
}

func (plb *plndrLoadBalancerManager) EnsureLoadBalancerDeleted(ctx context.Context, clusterName string, service *v1.Service) error {
	return plb.deleteLoadBalancer(service)
}

func (plb *plndrLoadBalancerManager) GetLoadBalancer(ctx context.Context, clusterName string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {

	// Get the err to be updated
	cm, err := plb.GetConfigMap(service.Namespace)
	if err != nil {
		return nil, true, nil
	}

	// Find the services configuraiton in the configMap
	svc, err := plb.GetServices(cm)
	if err != nil {
		return nil, false, err
	}

	for x := range svc.Services {
		if svc.Services[x].UID == string(service.UID) {
			return &v1.LoadBalancerStatus{
				Ingress: []v1.LoadBalancerIngress{
					{
						IP: svc.Services[x].Vip,
					},
				},
			}, true, nil
		}
	}
	return nil, false, nil
}

// GetLoadBalancerName returns the name of the load balancer. Implementations must treat the
// *v1.Service parameter as read-only and not modify it.
func (plb *plndrLoadBalancerManager) GetLoadBalancerName(_ context.Context, clusterName string, service *v1.Service) string {
	return getDefaultLoadBalancerName(service)
}

func getDefaultLoadBalancerName(service *v1.Service) string {
	return cloudprovider.DefaultLoadBalancerName(service)
}
func (plb *plndrLoadBalancerManager) deleteLoadBalancer(service *v1.Service) error {
	klog.Infof("deleting service '%s' (%s)", service.Name, service.UID)

	// Get the err to be updated
	cm, err := plb.GetConfigMap(service.Namespace)
	if err != nil {
		klog.Errorf("The configMap [%s] doensn't exist", PlunderConfigMap)
		return nil
	}
	// Find the services configuraiton in the configMap
	svc, err := plb.GetServices(cm)
	if err != nil {
		klog.Errorf("The service [%s] in configMap [%s] doensn't exist", service.Name, PlunderConfigMap)
		return nil
	}

	// Update the services configuration, by removing the  service
	updatedSvc := svc.delServiceFromUID(string(service.UID))
	if len(service.Status.LoadBalancer.Ingress) != 0 {
		ipam.ReleaseAddress(service.Namespace, service.Status.LoadBalancer.Ingress[0].IP)
	}
	// Update the configMap
	_, err = plb.UpdateConfigMap(cm, updatedSvc)
	return err
}

func (plb *plndrLoadBalancerManager) syncLoadBalancer(service *v1.Service) (*v1.LoadBalancerStatus, error) {

	// Get the err to be updated
	cm, err := plb.GetConfigMap("kube-system")
	if err != nil {
		// TODO - determine best course of action
		cm, err = plb.CreateConfigMap("kube-system")
		if err != nil {
			return nil, err
		}
	}

	var vip, cidrRange string
	var ok bool
	// Build cidr key
	cidrKey := fmt.Sprintf("cidr-%s", service.Namespace)
	if cidrRange, ok = cm.Data[cidrKey]; !ok {
		return nil, fmt.Errorf("No cidr configuration for namespace [%s] exists in key [%s] configmap [%s]", service.Namespace, cidrKey, plb.configMap)

	}
	vip, err = ipam.FindAvailableHost(service.Namespace, cidrRange)
	if err != nil {
		return nil, err
	}

	// Retrieve the kube-vip configuration map
	cm, err = plb.GetConfigMap(service.Namespace)
	if err != nil {
		// TODO - determine best course of action
		cm, err = plb.CreateConfigMap(service.Namespace)
		if err != nil {
			return nil, err
		}
	}

	// This function reconciles the load balancer state
	klog.Infof("syncing service '%s' (%s) with vip: %s", service.Name, service.UID, vip)

	// Find the services configuraiton in the configMap
	svc, err := plb.GetServices(cm)
	if err != nil {
		klog.Errorf("Unable to retrieve services from configMap [%s]", PlunderConfigMap)

		// TODO best course of action, currently we create a new services config
		svc = &plndrServices{}
	}

	// TODO - manage more than one set of ports
	newSvc := services{
		ServiceName: service.Name,
		UID:         string(service.UID),
		Vip:         vip,
		Port:        int(service.Spec.Ports[0].Port),
	}

	svc.addService(newSvc)

	cm, err = plb.UpdateConfigMap(cm, svc)
	if err != nil {
		return nil, err
	}

	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{
			{
				IP: vip,
			},
		},
	}, nil
}
