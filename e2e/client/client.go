/*
Copyright 2020 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/oauth2"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	appsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	extv1beta1 "k8s.io/client-go/kubernetes/typed/extensions/v1beta1"
	rbacv1beta1 "k8s.io/client-go/kubernetes/typed/rbac/v1beta1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog"

	"github.com/GoogleCloudPlatform/gke-managed-certs/e2e/client/dns"
	"github.com/GoogleCloudPlatform/gke-managed-certs/e2e/client/managedcertificate"
	"github.com/GoogleCloudPlatform/gke-managed-certs/pkg/clients/ssl"
)

const (
	cloudSdkRootEnv = "CLOUD_SDK_ROOT"
	defaultHost     = ""
	dnsZoneEnv      = "DNS_ZONE"
	projectIDEnv    = "PROJECT_ID"
)

type Clients struct {
	ClusterRole        rbacv1beta1.ClusterRoleInterface
	ClusterRoleBinding rbacv1beta1.ClusterRoleBindingInterface
	CustomResource     apiextv1beta1.CustomResourceDefinitionInterface
	Deployment         appsv1.DeploymentInterface
	Dns                dns.Dns
	Ingress            extv1beta1.IngressInterface
	ManagedCertificate managedcertificate.ManagedCertificate
	Service            corev1.ServiceInterface
	ServiceAccount     corev1.ServiceAccountInterface
	SslCertificate     ssl.Ssl
}

func New(namespace string) (*Clients, error) {
	config, err := getRestConfig()
	if err != nil {
		return nil, err
	}

	appsClient, err := appsv1.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	coreClient, err := corev1.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	apiExtClient, err := apiextv1beta1.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	extClient, err := extv1beta1.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	rbacClient, err := rbacv1beta1.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	managedCertificateClient, err := managedcertificate.New(config, namespace)
	if err != nil {
		return nil, err
	}

	oauthClient, err := getOauthClient()
	if err != nil {
		return nil, err
	}

	projectID := os.Getenv(projectIDEnv)
	klog.Infof("projectID=%s", projectID)

	dnsZone := os.Getenv(dnsZoneEnv)
	klog.Infof("dnsZone=%s", dnsZone)

	dnsClient, err := dns.New(oauthClient, dnsZone)
	if err != nil {
		return nil, err
	}

	sslCertificateClient, err := ssl.New(oauthClient, projectID)
	if err != nil {
		return nil, err
	}

	return &Clients{
		ClusterRole:        rbacClient.ClusterRoles(),
		ClusterRoleBinding: rbacClient.ClusterRoleBindings(),
		CustomResource:     apiExtClient.CustomResourceDefinitions(),
		Deployment:         appsClient.Deployments(namespace),
		Dns:                dnsClient,
		Ingress:            extClient.Ingresses(namespace),
		ManagedCertificate: managedCertificateClient,
		Service:            coreClient.Services(namespace),
		ServiceAccount:     coreClient.ServiceAccounts(namespace),
		SslCertificate:     sslCertificateClient,
	}, nil
}

func getRestConfig() (*rest.Config, error) {
	kubeConfig := os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	c, err := clientcmd.LoadFromFile(kubeConfig)
	if err != nil {
		return nil, err
	}

	overrides := &clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: defaultHost}}
	return clientcmd.NewDefaultClientConfig(*c, overrides).ClientConfig()
}

func gcloud(command ...string) (string, error) {
	gcloudBin := fmt.Sprintf("%s/bin/gcloud", os.Getenv(cloudSdkRootEnv))
	out, err := exec.Command(gcloudBin, command...).Output()
	if err != nil {
		return "", err
	}
	return strings.Replace(string(out), "\n", "", -1), nil
}

func getOauthClient() (*http.Client, error) {
	gcloudAuthList, err := gcloud("auth", "list")
	if err != nil {
		return nil, err
	}
	klog.Infof("gcloud auth list: %s", gcloudAuthList)

	gcloudInfo, err := gcloud("info")
	if err != nil {
		return nil, err
	}
	klog.Infof("gcloud info: %s", gcloudInfo)

	gcloudConfigurations, err := gcloud("config", "configurations", "list")
	if err != nil {
		return nil, err
	}
	klog.Infof("gcloud config configurations list: %s", gcloudConfigurations)

	accessToken, err := gcloud("auth", "print-access-token")
	if err != nil {
		return nil, err
	}

	token := &oauth2.Token{AccessToken: accessToken}
	return oauth2.NewClient(oauth2.NoContext, oauth2.StaticTokenSource(token)), nil
}
