package porkbunsolver

import (
	// external

	"context"
	"fmt"
	"strings"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook"
	cmacme "github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/go-logr/logr"

	// k8s
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	// internal
	porkbunclient "github.com/hoodnoah/certmanager-porkbun-webhook/internal/PorkBunClient"
)

type PorkBunSolver struct {
	logger        logr.Logger
	KubeClient    kubernetes.Interface         // interacts with the kubernetes client
	PorkBunClient *porkbunclient.PorkBunClient // interacts with the PorkBun API
}

var _ webhook.Solver = &PorkBunSolver{}

// dummy inialization function; no need to get secrets from kubernetes
func (s *PorkBunSolver) Initialize(kubeConfig *rest.Config, stopCh <-chan struct{}) error {
	// build kubeClient
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to build kube client: %w", err)
	}
	s.KubeClient = kubeClient

	// retrieve porkbun credentials from a known Secret; namespace will be hardcoded as will secret
	secretName := "porkbun-credentials"
	secretNameSpace := "cert-manager"

	secret, err := kubeClient.CoreV1().Secrets(secretNameSpace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to retrieve porkbun credentials: %w", err)
	}

	apiKey := string(secret.Data["PORKBUN_API_KEY"])
	secretKey := string(secret.Data["PORKBUN_SECRET_KEY"])

	if apiKey == "" {
		return fmt.Errorf("missing required field PORKBUN_API_KEY in secret %s/%s", secretNameSpace, secretName)
	}

	if secretKey == "" {

		return fmt.Errorf("missing required field PORKBUN_SECRET_KEY in secret %s/%s", secretNameSpace, secretName)
	}

	// create porkbun client
	s.PorkBunClient = porkbunclient.NewPorkBunClient(s.logger, apiKey, secretKey)
	s.logger.Info("PorkBunSolver initialized successfully with PorkBun credentials from Kubernetes secrets.")

	return nil
}

func (s *PorkBunSolver) Name() string {
	return "porkbun"
}

// ctor for PorkBunSolver
func NewPorkBunSolver(logger logr.Logger, kubeClient kubernetes.Interface) *PorkBunSolver {
	return &PorkBunSolver{
		logger:     logger,
		KubeClient: kubeClient,
	}
}

func (s *PorkBunSolver) Present(cr *cmacme.ChallengeRequest) error {
	// Parse domain and subdomain
	domain, subdomain := parseDomainAndSubdomain(cr.ResolvedFQDN)
	s.logger.Info("Parsed domain and subdomain for Present", "domain", domain, "subdomain", subdomain)

	// call porkbun client to create DNS record
	if err := s.PorkBunClient.CreateDNSRecord(domain, cr.Key, subdomain); err != nil {
		s.logger.Error(err, "Failed to create DNS record; Present")
		return err
	}

	return nil
}

func (s *PorkBunSolver) CleanUp(cr *cmacme.ChallengeRequest) error {
	// parse domain and subdomain
	domain, subdomain := parseDomainAndSubdomain(cr.ResolvedFQDN)
	s.logger.Info("Parsed domain and subdomain for CleanUp", "domain", domain, "subdomain", subdomain)

	// call porkbun client to delete DNS record
	if err := s.PorkBunClient.DeleteDNSRecord(domain, cr.Key, subdomain); err != nil {
		s.logger.Error(err, "Failed to delete DNS record; CleanUp")
		return err
	}

	return nil
}

// helper to extract the domain and subdomain from a fully-qualified domain name
func parseDomainAndSubdomain(fqdn string) (string, string) {
	fqdn = strings.TrimSuffix(fqdn, ".") // remove trailing dot
	parts := strings.SplitN(fqdn, ".", 2)
	if len(parts) < 2 {
		return fqdn, "" // fallback
	}

	return parts[1], parts[0]
}
