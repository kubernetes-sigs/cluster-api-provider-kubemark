/*
Copyright 2022 The Kubernetes Authors.

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
	"sync"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cluster-api/controllers/remote"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	infrav1 "github.com/kubernetes-sigs/cluster-api-provider-kubemark/api/v1alpha4"
)

const (
	healthCheckRequestTimeout         = 5 * time.Second
	healthCheckUnhealthyThreshold     = 10
	initialCacheSyncTimeout           = 5 * time.Minute
	computeClusterSecretKubeConfigKey = "kubeconfig"

	defaultClientTimeout = 10 * time.Second
)

var (
	// ErrClusterLocked is returned in methods that require cluster-level locking
	// if the cluster is already locked by another concurrent call.
	ErrClusterLocked = errors.New("compute cluster is locked; another process is creating the client")

	// Pointer to clientCreate func; required to inject test fake client creator.
	clientCreator = createClient

	// Pointer to cache.New func; required to inject test fake client creator.
	cacheCreator = cache.New

	// healthCheckPollInterval defines the interval between each health check; it is defined as a global var so it can be changed in unit tests.
	healthCheckPollInterval = 10 * time.Second

	// unsafeSkipAPIServerCheck prevents health check to check API server liveness; it is used in unit tests.
	unsafeSkipAPIServerCheck = false
)

// ComputeClusterTracker manages clients for ComputeCluster
//
// Kubemark provider creates pods on a compute cluster.
// A compute cluster can be the management cluster itself, or another kubernetes cluster.
//
// In order to access compute clusters it is required a controller runtime client, and
// each controller runtime client implies a cache and informers.
//
// Considering that each reconcile requires a client, and we have a couple of reconcilers
// running with concurrency 10 by default, in order to avoid memory consumption problems
// it is necessary to keep a single client and share it across all reconcile loops.
//
// This is what ComputeClusterTracker is for.
//
// ComputeClusterTracker implementation is heavily inspired from the Cluster API's ClusterClass tracker.
type ComputeClusterTracker struct {
	client client.Client

	// clusterAccessorsLock is used to lock the access to the clusterAccessors map.
	clusterAccessorsLock sync.RWMutex
	// clusterAccessors is the map of clusterAccessors by secrets containing kubeconfig for accessing to the ComputeCluster.
	// NOTE: In case two secrets have the same kubeconfig the system will create two accessor instead of one, but
	// this is considered acceptable for the goals of the kubemark provider.
	clusterAccessors map[client.ObjectKey]*clusterAccessor
	// clusterLock is a per-cluster lock used whenever we're locking for a specific cluster.
	clusterLock *keyedMutex
}

// NewComputeClusterTracker returns a new ComputeClusterTracker instance.
func NewComputeClusterTracker(manager ctrl.Manager) *ComputeClusterTracker {
	return &ComputeClusterTracker{
		client:           manager.GetClient(),
		clusterAccessors: make(map[client.ObjectKey]*clusterAccessor),
		clusterLock:      newKeyedMutex(),
	}
}

// ComputeClusterObject defines an object that allows to get a ComputeCluster definition from the object's spec.
type ComputeClusterObject interface {
	metav1.Object
	// GetComputeCluster gets the ComputeCluster definition from the object's spec.
	GetComputeCluster() *infrav1.ComputeClusterSpec
}

// ComputeCluster contains info about a compute cluster.
type ComputeCluster struct {
	client.Client

	// The namespace where to host kubemark pods.
	Namespace string

	// IsManagementCluster is true when the ComputeCluster is the management cluster itself
	IsManagementCluster bool
}

// GetFor returns a ComputeCluster for a list of ComputeClusterObjects.
// the first object with a ComputeClusterSpec defined takes precedence on the others.
// e.g. KubemarkCluster with ComputeClusterSpec, KubemarkMachine with ComputeClusterSpec --> ComputeClusterSpec from KubemarkCluster is considered.
// e.g. KubemarkCluster without ComputeClusterSpec, KubemarkMachine with ComputeClusterSpec --> ComputeClusterSpec from KubemarkMachine is considered.
func (t *ComputeClusterTracker) GetFor(ctx context.Context, objs ...ComputeClusterObject) (*ComputeCluster, error) {
	// Gets the namespace and the ComputeClusterSpec from the first of the ComputeClusterObjects
	// defining those values.
	var namespace string
	var computeCluster *infrav1.ComputeClusterSpec
	for _, o := range objs {
		if o == nil {
			continue
		}
		namespace = o.GetNamespace()
		computeCluster = o.GetComputeCluster()
		if computeCluster != nil {
			break
		}
	}

	// If no one of the ComputeClusterObject has a ComputeCluster configuration defined, then
	// the management cluster will be used to host kubemark pods into the same name namespace of the namespace of the first object.
	if computeCluster == nil {
		return &ComputeCluster{
			Client:              t.client,
			Namespace:           namespace,
			IsManagementCluster: true,
		}, nil
	}

	// Gets the accessor for the compute cluster reference by the kubeconfig contained in the secret
	// the ComputeClusterSpec reference to.
	accessor, err := t.getClusterAccessor(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      computeCluster.KubeConfigSecretRef.Name,
	})
	if err != nil {
		return nil, err
	}

	// Computes the namespace where to host kubemark pods;
	// the namespace defined in the ComputeClusterSpec will take precedence on the namespace defined
	// in the kubeconfig; "default" will be used if both are empty.
	computeClusterNamespace := corev1.NamespaceDefault
	if accessor.kubeConfigNamespace != "" {
		computeClusterNamespace = accessor.kubeConfigNamespace
	}
	if computeCluster.Namespace != "" {
		computeClusterNamespace = computeCluster.Namespace
	}

	return &ComputeCluster{
		Client:              accessor.client,
		Namespace:           computeClusterNamespace,
		IsManagementCluster: false,
	}, nil
}

// clusterAccessor represents the combination of a delegating client and a cache for a compute cluster.
type clusterAccessor struct {
	cache  *stoppableCache
	client client.Client

	kubeConfigNamespace string
}

// loadAccessor loads a clusterAccessor.
func (t *ComputeClusterTracker) loadAccessor(secret client.ObjectKey) (*clusterAccessor, bool) {
	t.clusterAccessorsLock.RLock()
	defer t.clusterAccessorsLock.RUnlock()

	accessor, ok := t.clusterAccessors[secret]
	return accessor, ok
}

// storeAccessor stores a clusterAccessor.
func (t *ComputeClusterTracker) storeAccessor(secret client.ObjectKey, accessor *clusterAccessor) {
	t.clusterAccessorsLock.Lock()
	defer t.clusterAccessorsLock.Unlock()

	t.clusterAccessors[secret] = accessor
}

// getClusterAccessor returns a clusterAccessor for the kubeconfig contained in the secret.
// It first tries to return an already-created clusterAccessor.
// It then falls back to create a new clusterAccessor if needed.
// If there is already another go routine trying to create a clusterAccessor
// for the same secret, an error is returned.
func (t *ComputeClusterTracker) getClusterAccessor(ctx context.Context, secret client.ObjectKey) (*clusterAccessor, error) {
	log := ctrl.LoggerFrom(ctx, "secret", klog.KRef(secret.Namespace, secret.Name))
	ctx = ctrl.LoggerInto(ctx, log)

	// If the clusterAccessor already exists, return early.
	if accessor, ok := t.loadAccessor(secret); ok {
		return accessor, nil
	}

	// clusterAccessor doesn't exist yet, we might have to initialize one.
	// Lock on the secret to ensure only one clusterAccessor is initialized
	// for the secret at the same time.
	// Return an error if another go routine already tries to create a clusterAccessor.
	if ok := t.clusterLock.TryLock(secret); !ok {
		return nil, errors.Wrapf(ErrClusterLocked, "failed to get lock for the the compute server secret")
	}
	defer t.clusterLock.Unlock(secret)

	// Until we got the secret lock a different goroutine might have initialized the clusterAccessor
	// for this secret successfully already. If this is the case we return it.
	if accessor, ok := t.loadAccessor(secret); ok {
		return accessor, nil
	}

	// We are the go routine who has to initialize the clusterAccessor.
	log.V(4).Info("Creating new compute server accessor")
	accessor, err := t.newClusterAccessor(ctx, secret)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create compute server accessor referenced by secret %s", klog.KRef(secret.Namespace, secret.Name))
	}

	log.V(4).Info("Storing new compute server accessor")
	t.storeAccessor(secret, accessor)
	return accessor, nil
}

// newClusterAccessor creates a new clusterAccessor.
func (t *ComputeClusterTracker) newClusterAccessor(ctx context.Context, secret client.ObjectKey) (*clusterAccessor, error) {
	// Get a rest config for the back cluster
	kubeConfigSecret := &corev1.Secret{}
	kubeConfigSecretKey := client.ObjectKey{Namespace: secret.Namespace, Name: secret.Name}
	if err := t.client.Get(ctx, kubeConfigSecretKey, kubeConfigSecret); err != nil {
		return nil, errors.Wrap(err, "failed to get secret")
	}

	kubeConfigData, ok := kubeConfigSecret.Data[computeClusterSecretKubeConfigKey]
	if !ok {
		return nil, errors.New("invalid secret: 'kubeconfig' key is missing")
	}

	kubeConfig, err := clientcmd.Load(kubeConfigData)
	if err != nil {
		return nil, errors.Wrap(err, "invalid secret: error loading kubeconfig")
	}

	kubeConfigCurrentContext, ok := kubeConfig.Contexts[kubeConfig.CurrentContext]
	if !ok {
		return nil, errors.New("invalid secret: failed to get CurrentContext from the kubeconfig")
	}

	// generate REST config to access the compute cluster
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigData)
	if err != nil {
		return nil, errors.Wrap(err, "error creating REST config for the kubeconfig")
	}
	config.Timeout = defaultClientTimeout
	config.UserAgent = remote.DefaultClusterAPIUserAgent("cluster-api-kubemark-manager")

	// Create a client and a mapper for the cluster.
	c, mapper, err := clientCreator(t.client.Scheme(), config, secret)
	if err != nil {
		return nil, errors.Wrap(err, "error creating client")
	}

	// Create the cache for the remote cluster
	cacheOptions := cache.Options{
		Scheme: t.client.Scheme(),
		Mapper: mapper,
	}
	remoteCache, err := cacheCreator(config, cacheOptions)
	if err != nil {
		return nil, errors.Wrap(err, "error creating cache")
	}

	cacheCtx, cacheCtxCancel := context.WithCancel(ctx)

	// We need to be able to stop the cache's shared informers, so wrap this in a stoppableCache.
	cache := &stoppableCache{
		Cache:      remoteCache,
		cancelFunc: cacheCtxCancel,
	}

	// Start the cache!!!
	go cache.Start(cacheCtx) //nolint:errcheck

	// Wait until the cache is initially synced
	cacheSyncCtx, cacheSyncCtxCancel := context.WithTimeout(ctx, initialCacheSyncTimeout)
	defer cacheSyncCtxCancel()
	if !cache.WaitForCacheSync(cacheSyncCtx) {
		cache.Stop()
		return nil, errors.Errorf("error waiting for cache: %v", cacheCtx.Err().Error())
	}

	// Start cluster healthcheck!!!
	go t.healthCheckCluster(cacheCtx, secret, config)

	delegatingClient, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader:     cache,
		Client:          c,
		UncachedObjects: []client.Object{
			// TODO: Disable Cache for ConfigMap, Sercrets, Pods and shift to using PartialObjectMetadata for Get/List. Same in Main.
			// &corev1.ConfigMap{},
			// &corev1.Secret{},
			// &corev1.Pod{},
		},
	})
	if err != nil {
		return nil, err
	}

	return &clusterAccessor{
		cache:               cache,
		client:              delegatingClient,
		kubeConfigNamespace: kubeConfigCurrentContext.Namespace,
	}, nil
}

// createClient creates a client and a mapper based on a rest.Config.
func createClient(scheme *runtime.Scheme, config *rest.Config, secret client.ObjectKey) (client.Client, meta.RESTMapper, error) {
	// Create a mapper for it
	mapper, err := apiutil.NewDynamicRESTMapper(config)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error creating dynamic rest mapper for remote secret %q", secret.String())
	}

	// Create the client for the remote secret
	c, err := client.New(config, client.Options{Scheme: scheme, Mapper: mapper})
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error creating client for remote secret %q", secret.String())
	}
	return c, mapper, nil
}

// healthCheckCluster will poll the cluster's API at the path given and, if there are
// `unhealthyThreshold` consecutive failures, will deem the cluster unhealthy.
// Once the cluster is deemed unhealthy, the cluster's cache is stopped and removed.
func (t *ComputeClusterTracker) healthCheckCluster(ctx context.Context, secret client.ObjectKey, config *rest.Config) {
	unhealthyCount := 0

	// This gets us a client that can make raw http(s) calls to the remote apiserver. We only need to create it once
	// and we can reuse it inside the polling loop.
	codec := runtime.NoopEncoder{Decoder: scheme.Codecs.UniversalDecoder()}
	cfg := rest.CopyConfig(config)
	cfg.NegotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{Serializer: codec})
	restClient, restClientErr := rest.UnversionedRESTClientFor(cfg)

	runHealthCheckWithThreshold := func() (bool, error) {
		if restClientErr != nil {
			return false, restClientErr
		}

		inUse := false
		machineList := &infrav1.KubemarkMachineList{}
		if err := t.client.List(ctx, machineList); err != nil {
			return false, errors.Wrapf(err, "failed to health check secret")
		}
		for _, km := range machineList.Items {
			if km.Spec.ComputeCluster != nil && km.Namespace == secret.Namespace && km.Spec.ComputeCluster.KubeConfigSecretRef.Name == secret.Name {
				inUse = true
				break
			}
		}
		if !inUse {
			return false, errors.New("No more consumers for this compute cluster")
		}

		// if the API server check is disabled, return.
		if unsafeSkipAPIServerCheck {
			return false, nil
		}

		// An error here means there was either an issue connecting or the API returned an error.
		// If no error occurs, reset the unhealthy counter.
		path := "/"
		_, err := restClient.Get().AbsPath(path).Timeout(healthCheckRequestTimeout).DoRaw(ctx)
		if err != nil {
			if apierrors.IsUnauthorized(err) {
				// Unauthorized means that the underlying kubeconfig is not authorizing properly anymore, which
				// usually is the result of automatic kubeconfig refreshes, meaning that we have to throw away the
				// clusterAccessor and rely on the creation of a new one (with a refreshed kubeconfig)
				return false, err
			}
			unhealthyCount++
		} else {
			unhealthyCount = 0
		}

		if unhealthyCount >= healthCheckUnhealthyThreshold {
			// Cluster is now considered unhealthy.
			return false, err
		}

		return false, nil
	}

	err := wait.PollImmediateUntil(healthCheckPollInterval, runHealthCheckWithThreshold, ctx.Done())
	// An error returned implies the health check has failed a sufficient number of
	// times for the cluster to be considered unhealthy
	// NB. we are ignoring ErrWaitTimeout because this error happens when the channel is close, that in this case
	// happens when the cache is explicitly stopped.
	if err != nil && err != wait.ErrWaitTimeout {
		// TODO: fix log
		// t.log.Error(err, "Error health checking cluster", "Cluster", klog.KRef(in.cluster.Namespace, in.cluster.Name))
		t.deleteAccessor(ctx, secret)
	}
}

// deleteAccessor stops a clusterAccessor's cache and removes the clusterAccessor from the tracker.
func (t *ComputeClusterTracker) deleteAccessor(_ context.Context, secret client.ObjectKey) {
	t.clusterAccessorsLock.Lock()
	defer t.clusterAccessorsLock.Unlock()

	a, exists := t.clusterAccessors[secret]
	if !exists {
		return
	}

	a.cache.Stop()
	delete(t.clusterAccessors, secret)
}

// keyedMutex is a mutex locking on the key provided to the Lock function.
// Only one caller can hold the lock for a specific key at a time.
// A second Lock call if the lock is already held for a key returns false.
type keyedMutex struct {
	locksMtx sync.Mutex
	locks    map[client.ObjectKey]struct{}
}

// newKeyedMutex creates a new keyed mutex ready for use.
func newKeyedMutex() *keyedMutex {
	return &keyedMutex{
		locks: make(map[client.ObjectKey]struct{}),
	}
}

// TryLock locks the passed in key if it's not already locked.
// A second Lock call if the lock is already held for a key returns false.
// In the ClusterCacheTracker case the key is the ObjectKey for a cluster.
func (k *keyedMutex) TryLock(key client.ObjectKey) bool {
	k.locksMtx.Lock()
	defer k.locksMtx.Unlock()

	// Check if there is already a lock for this key (e.g. Cluster).
	if _, ok := k.locks[key]; ok {
		// There is already a lock, return false.
		return false
	}

	// Lock doesn't exist yet, create the lock.
	k.locks[key] = struct{}{}

	return true
}

// Unlock unlocks the key.
func (k *keyedMutex) Unlock(key client.ObjectKey) {
	k.locksMtx.Lock()
	defer k.locksMtx.Unlock()

	// Remove the lock if it exists.
	delete(k.locks, key)
}

// stoppableCache embeds cache.Cache and combines it with a stop channel.
type stoppableCache struct {
	cache.Cache

	lock       sync.Mutex
	stopped    bool
	cancelFunc context.CancelFunc
}

// Stop cancels the cache.Cache's context, unless it has already been stopped.
func (cc *stoppableCache) Stop() {
	cc.lock.Lock()
	defer cc.lock.Unlock()

	if cc.stopped {
		return
	}

	cc.stopped = true
	cc.cancelFunc()
}
