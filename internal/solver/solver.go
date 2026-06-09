package solver

import (
	// stdlib
	"context"
	"fmt"
	"log"
	"os"
	"sync"

	// internal
	"github.com/hoodnoah/certmanager-porkbun-webhook/internal/util"

	// external
	porkbun "github.com/hoodnoah/porkbun/pkg"

	// external -- cert-manager
	"github.com/cert-manager/cert-manager/pkg/acme/webhook"
	acme "github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"

	// external -- kubernetes
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// constants
// will need to be defined in the kubernetes cluster at creation time
const (
	secretName          = "porkbun-credentials"
	secretNamespace     = "cert-manager"
	apiKeySecretName    = "PORKBUN_API_KEY"
	secretKeySecretName = "PORKBUN_SECRET_KEY"
)

// Solver struct.
// Must implement the `github.com/cert-manager/cert-manager/pkg/acme/webhook.Solver` interface.
type porkbunSolver struct {
	logger       *log.Logger       // simple logger; helps w/ debugging
	name         string            // identifier for the solver instance. Useful for debugging, etc.
	pbClient     *porkbun.PorkBun  // interactivity with the Porkbun API
	txtRecords   map[string]string // map of fqdn -> record ID; allows delete-by-id
	sync.RWMutex                   // prevent race conditions in asynchronous operations
}

// assert that porkbunSolver implements the required interface
var _ webhook.Solver = &porkbunSolver{}

// constructor for the porkbunSolver
func NewPorkbunSolver(logger *log.Logger, name string) *porkbunSolver {
	pbClient := porkbun.NewPorkbun("", "") // keys will be collected from cluster, and set in the Initialize() method

	return &porkbunSolver{
		logger:   logger,
		name:     name,
		pbClient: pbClient,
		RWMutex:  sync.RWMutex{},
	}
}

// returns the name of the solver instance for identification/debugging purposes
func (p *porkbunSolver) Name() string {
	return p.name
}

// initializes the porkbun struct; allows the collection of
// secrets from the cluster, as req'd for initializing the porkbun API wrapper
// i.e. `apiKey` and `secretKey`
//
// Credentials are sourced in priority order:
//  1. Ambient credentials: the PORKBUN_API_KEY and PORKBUN_SECRET_KEY
//     environment variables, if BOTH are set. This path is used by the
//     cert-manager DNS01 conformance suite, whose test fixture calls
//     Initialize against an ephemeral apiserver before any secrets exist
//     in it. It also serves as an escape hatch for running the webhook
//     with credentials injected via the deployment spec.
//  2. The `porkbun-credentials` secret in the `cert-manager` namespace,
//     fetched from the cluster (production path; unchanged behavior).
func (p *porkbunSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	// 1: ambient credentials from the environment
	apiKeyEnv, secretKeyEnv := os.Getenv(apiKeySecretName), os.Getenv(secretKeySecretName)
	if apiKeyEnv != "" && secretKeyEnv != "" {
		p.pbClient.SetAPIKey(apiKeyEnv)
		p.pbClient.SetSecretKey(secretKeyEnv)

		p.logger.Printf("Loaded api key, secret key from environment variables %s, %s; skipping in-cluster secret lookup.", apiKeySecretName, secretKeySecretName)
		p.logger.Print("Ready to handle ACME challenges.")

		return nil
	}

	// 2: fetch credentials from the cluster
	// create a Kubernetes clientset; gives access to the cluster API at runtime
	clientSet, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		p.logger.Printf("Failed to create a Kubernetes clientset for secret collection with error %v", err)
		return err
	}

	// fetch the secret from the cluster, using the const name, namespace from above
	secret, err := clientSet.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		p.logger.Printf("Failed to fetch secrets from Kubernetes clientset with error %v", err)
		return err
	}

	// extract the individual secret values, using the const secret names from above
	apiKey, ok := secret.Data[apiKeySecretName]
	if !ok {
		p.logger.Printf("Failed to fetch the api key from secret %s.", apiKeySecretName)
		return fmt.Errorf("failed to get api key from %s secret %s in namespace %s", apiKeySecretName, secretName, secretNamespace)
	}

	secretKey, ok := secret.Data[secretKeySecretName]
	if !ok {
		p.logger.Printf("Failed to fetch the secret key from secret %s", secretKeySecretName)
		return fmt.Errorf("failed to get secret key from %s secret %s in namespace %s", secretKeySecretName, secretName, secretNamespace)
	}

	p.logger.Print("Successfully loaded api key, secret key.")

	// set keys in the PorkBun struct
	p.pbClient.SetAPIKey(string(apiKey))
	p.pbClient.SetSecretKey(string(secretKey))

	p.logger.Print("Successfully set up porkbun API wrapper client with api key, secret key.")
	p.logger.Print("Ready to handle ACME challenges.")

	return nil
}

// When a challenge request is provided by `cert-manager`,
// the details of the request are extracted and translated into a format that the Porkbun client
// can make use of. Then the record is created, after checking if the record exists first.
func (p *porkbunSolver) Present(challenge *acme.ChallengeRequest) error {
	// lock the solver, prevent race conditions
	p.Lock()
	defer p.Unlock()

	p.logger.Printf("Received ACME challenge for %s...", challenge.ResolvedFQDN)

	// extract domain and subdomain from challenge
	// ResolvedZone contains the actual registered domain (e.g., "noah-hood.io.")
	// ResolvedFQDN contains the full challenge FQDN (e.g., "_acme-challenge.test.noah-hood.io.")
	domain, subdomain := util.ExtractDomainAndSubdomain(challenge.ResolvedFQDN, challenge.ResolvedZone)
	p.logger.Printf("domain: %s, subdomain: %s", domain, subdomain)

	// create record with helper fn
	if err := util.CreateDNSRecordByNameTypeIfNotExists(p.pbClient, domain, subdomain, challenge.Key); err != nil {
		p.logger.Printf("failed to create DNS Record by name type for %s with error %v", challenge.ResolvedFQDN, err)
		return err
	}

	return nil
}

// delete DNS record after challenge is satisfied
func (p *porkbunSolver) CleanUp(challenge *acme.ChallengeRequest) error {
	// lock the solver, prevent race conditions
	p.Lock()
	defer p.Unlock()

	p.logger.Printf("Received cleanup call for %s...", challenge.ResolvedFQDN)

	// extract domain and subdomain from challenge
	domain, subdomain := util.ExtractDomainAndSubdomain(challenge.ResolvedFQDN, challenge.ResolvedZone)
	p.logger.Printf("domain: %s, subdomain: %s", domain, subdomain)

	// delete record with helper fn
	if err := util.DeleteDNSRecordByNameTypeIfExists(p.pbClient, domain, subdomain, challenge.Key); err != nil {
		p.logger.Printf("failed to delete DNS Record by name type for %s with error %v", challenge.ResolvedFQDN, err)
		return err
	}

	return nil
}
