/*
Copyright 2023.

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
	"fmt"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	secretsstorecsiv1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
	"sigs.k8s.io/secrets-store-csi-driver/pkg/util/secretutil"

	secretsstorecsixk8siov1 "github.com/aramase/secrets-store-controller/api/v1"
	"github.com/aramase/secrets-store-controller/pkg/k8s"
	"github.com/aramase/secrets-store-controller/pkg/provider"
)

const (
	// FilePermission is the permission to be used for the staging target path
	FilePermission os.FileMode = 0644

	// CSIPodName is the name of the pod that the mount is created for
	CSIPodName = "csi.storage.k8s.io/pod.name"
	// CSIPodNamespace is the namespace of the pod that the mount is created for
	CSIPodNamespace = "csi.storage.k8s.io/pod.namespace"
	// CSIPodUID is the UID of the pod that the mount is created for
	CSIPodUID = "csi.storage.k8s.io/pod.uid"
	// CSIPodServiceAccountName is the name of the pod service account that the mount is created for
	CSIPodServiceAccountName = "csi.storage.k8s.io/serviceAccount.name"
	// CSIPodServiceAccountTokens is the service account tokens of the pod that the mount is created for
	CSIPodServiceAccountTokens = "csi.storage.k8s.io/serviceAccount.tokens" //nolint
)

// SecretProviderReconciler reconciles a SecretProvider object
type SecretProviderReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	TokenClient     *k8s.TokenClient
	ProviderClients *provider.PluginClientBuilder
}

//+kubebuilder:rbac:groups=secrets-store.csi.x-k8s.io,resources=secretproviders,verbs=get;list;watch
//+kubebuilder:rbac:groups=secrets-store.csi.x-k8s.io,resources=secretproviders/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=secrets-store.csi.x-k8s.io,resources=secretproviders/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources="serviceaccounts/token",verbs=create

// Reconcile is called for a SecretProvider object
func (r *SecretProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling secret provider", "name", req.NamespacedName.String())

	// get the secret provider object
	sp := &secretsstorecsixk8siov1.SecretProvider{}
	if err := r.Get(ctx, req.NamespacedName, sp); err != nil {
		logger.Error(err, "failed to get secret provider", "name", req.NamespacedName.String())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// validate the secret provider object
	if sp.Spec.ServiceAccountName == "" {
		err := fmt.Errorf("service account name is empty")
		logger.Error(err, "failed to validate secret provider", "name", req.NamespacedName.String())
		return ctrl.Result{}, err
	}
	if sp.Spec.SecretProviderClassName == "" {
		err := fmt.Errorf("secret provider class name is empty")
		logger.Error(err, "failed to validate secret provider", "name", req.NamespacedName.String())
		return ctrl.Result{}, err
	}

	// get the secret provider class object
	spc := &secretsstorecsiv1.SecretProviderClass{}
	if err := r.Get(ctx, client.ObjectKey{Name: sp.Spec.SecretProviderClassName}, spc); err != nil {
		logger.Error(err, "failed to get secret provider class", "name", sp.Spec.SecretProviderClassName)
		return ctrl.Result{}, err
	}

	// get the service account token
	serviceAccountTokenAttrs, err := r.TokenClient.SecretProviderServiceAccountTokenAttrs(sp.Namespace, sp.Spec.ServiceAccountName, sp.Spec.TokenRequests)
	if err != nil {
		logger.Error(err, "failed to get service account token", "name", sp.Spec.ServiceAccountName)
		return ctrl.Result{}, err
	}

	// this is to mimic the parameters sent from CSI driver to the provider
	parameters := make(map[string]string)
	if spc.Spec.Parameters != nil {
		parameters = spc.Spec.Parameters
	}

	parameters[CSIPodName] = "unknown"
	parameters[CSIPodNamespace] = req.Namespace
	parameters[CSIPodUID] = "unknown"
	parameters[CSIPodServiceAccountName] = sp.Spec.ServiceAccountName

	for k, v := range serviceAccountTokenAttrs {
		parameters[k] = v
	}

	paramsJSON, err := json.Marshal(parameters)
	if err != nil {
		logger.Error(err, "failed to marshal parameters", "parameters", parameters)
		return ctrl.Result{}, err
	}

	// TODO(aramase): handle node publish secrets
	var secretsJSON []byte
	nodePublishSecretData := make(map[string]string)
	secretsJSON, err = json.Marshal(nodePublishSecretData)
	if err != nil {
		logger.Error(err, "failed to marshal node publish secrets", "nodePublishSecretData", nodePublishSecretData)
		return ctrl.Result{}, err
	}
	// TODO(aramase): consider making this optional
	permissionJSON, err := json.Marshal(FilePermission)
	if err != nil {
		logger.Error(err, "failed to marshal file permission", "filePermission", FilePermission)
		return ctrl.Result{}, err
	}

	providerName := string(spc.Spec.Provider)
	providerClient, err := r.ProviderClients.Get(ctx, providerName)
	if err != nil {
		logger.Error(err, "failed to get provider client", "provider", providerName)
		return ctrl.Result{}, err
	}

	// TODO(aramase): handle object versions
	_, files, err := provider.MountContent(ctx, providerClient, string(paramsJSON), string(secretsJSON), string(permissionJSON), map[string]string{})
	if err != nil {
		logger.Error(err, "failed to get secrets from provider", "provider", providerName)
		return ctrl.Result{}, err
	}

	for _, secretObj := range spc.Spec.SecretObjects {
		secretName := strings.TrimSpace(secretObj.SecretName)
		if err := secretutil.ValidateSecretObject(*secretObj); err != nil {
			logger.Error(err, "failed to validate secret object", "secretName", secretName)
			return ctrl.Result{}, err
		}

		secretType := secretutil.GetSecretType(strings.TrimSpace(secretObj.Type))
		var datamap map[string][]byte
		if datamap, err = secretutil.GetSecretData(secretObj.Data, secretType, files); err != nil {
			logger.Error(err, "failed to get secret data", "secretName", secretName)
			return ctrl.Result{}, err
		}

		// TODO(aramase): make this run in the background so that we don't fail creating other secrets
		if err := r.createOrUpdateSecret(ctx, secretName, req.Namespace, datamap, secretType); err != nil {
			logger.Error(err, "failed to create or update secret", "secretName", secretName)
			return ctrl.Result{}, err
		}
	}

	// set the next rotation time
	return ctrl.Result{RequeueAfter: sp.Spec.RotationPollInterval.Duration}, nil
}

func (r *SecretProviderReconciler) createOrUpdateSecret(ctx context.Context, name, namespace string, datamap map[string][]byte, secretType corev1.SecretType) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				"created-by": "github.com/aramase/secrets-store-controller",
			},
		},
		Type: secretType,
		Data: datamap,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// if already exists, update the data
		secret.Data = datamap
		return nil
	})
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&secretsstorecsixk8siov1.SecretProvider{}).
		Complete(r)
}
