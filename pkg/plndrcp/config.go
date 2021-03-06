package plndrcp

import (
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Services functions - once the service data is taken from teh configMap, these functions will interact with the data

func (s *plndrServices) addService(newSvc services) {
	s.Services = append(s.Services, newSvc)
}

func (s *plndrServices) findService(UID string) *services {
	for x := range s.Services {
		if s.Services[x].UID == UID {
			return &s.Services[x]
		}
	}
	return nil
}

func (s *plndrServices) delServiceFromUID(UID string) *plndrServices {
	// New Services list
	updatedServices := &plndrServices{}
	// Add all [BUT] the removed service
	for x := range s.Services {
		if s.Services[x].UID != UID {
			updatedServices.Services = append(updatedServices.Services, s.Services[x])
		}
	}
	// Return the updated service list (without the mentioned service)
	return updatedServices
}

func (s *plndrServices) updateServices(vip, name, uid string) string {
	newsvc := services{
		Vip:         vip,
		UID:         uid,
		ServiceName: name,
	}
	s.Services = append(s.Services, newsvc)
	b, _ := json.Marshal(s)
	return string(b)
}

// ConfigMap functions - these wrap all interactions with the kubernetes configmaps

func (plb *plndrLoadBalancerManager) GetServices(cm *v1.ConfigMap) (svcs *plndrServices, err error) {
	// Attempt to retrieve the config map
	b := cm.Data[PlunderServicesKey]
	// Unmarshall raw data into struct
	err = json.Unmarshal([]byte(b), &svcs)
	return
}

func (plb *plndrLoadBalancerManager) GetConfigMap(cm, nm string) (*v1.ConfigMap, error) {
	// Attempt to retrieve the config map
	return plb.kubeClient.CoreV1().ConfigMaps(nm).Get(plb.cloudConfigMap, metav1.GetOptions{})
}

func (plb *plndrLoadBalancerManager) CreateConfigMap(cm, nm string) (*v1.ConfigMap, error) {
	// Create new configuration map in the correct namespace
	newConfigMap := v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      plb.cloudConfigMap,
			Namespace: nm,
		},
	}
	// Return results of configMap create
	return plb.kubeClient.CoreV1().ConfigMaps(nm).Create(&newConfigMap)
}

func (plb *plndrLoadBalancerManager) UpdateConfigMap(cm *v1.ConfigMap, s *plndrServices) (*v1.ConfigMap, error) {
	// Create new configuration map in the correct namespace

	// If the cm.Data / cm.Annotations haven't been initialised
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	if cm.Annotations == nil {
		cm.Annotations = map[string]string{}
		cm.Annotations["provider"] = ProviderName
	}

	// Set ConfigMap data
	b, _ := json.Marshal(s)
	cm.Data[PlunderServicesKey] = string(b)

	// Return results of configMap create
	return plb.kubeClient.CoreV1().ConfigMaps(cm.Namespace).Update(cm)
}
