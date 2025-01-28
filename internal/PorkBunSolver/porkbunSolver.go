package porkbunsolver

import (
	// external

	"strings"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook"
	cmacme "github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/go-logr/logr"
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
func (s *PorkBunSolver) Initialize(_ *rest.Config, _ <-chan struct{}) error {
	return nil
}

func (s *PorkBunSolver) Name() string {
	return "porkbun"
}

// ctor for PorkBunSolver
func NewPorkBunSolver(logger logr.Logger, kubeClient kubernetes.Interface, porkBunClient *porkbunclient.PorkBunClient) *PorkBunSolver {
	return &PorkBunSolver{
		logger:        logger,
		KubeClient:    kubeClient,
		PorkBunClient: porkBunClient,
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
	if err := s.PorkBunClient.DeleteDNSRecord(domain, subdomain); err != nil {
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
