//go:build conformance

// This file is excluded from ordinary builds and test runs because importing
// the cert-manager test fixture panics in init() if the envtest binaries
// (etcd, kube-apiserver, kubectl) aren't present — before any t.Skip can
// fire. Run it explicitly with:
//
//	just test-conformance
//
// or see .gitea/workflows/ci.yaml for the raw invocation.
package main

import (
	"log"
	"os"
	"strings"
	"testing"
	"time"

	// the cert-manager DNS01 conformance fixture; package name is `dns`
	dns "github.com/cert-manager/cert-manager/test/acme"

	"github.com/hoodnoah/certmanager-porkbun-webhook/internal/solver"
)

// TestRunsSuite executes the upstream cert-manager DNS01 conformance suite
// against the Porkbun solver. It makes REAL Porkbun API calls: it creates a
// TXT record named `cert-manager-dns01-tests.<zone>`, polls authoritative DNS
// until it propagates, then deletes it and polls until it's gone.
//
// Requirements (all via environment variables):
//   - TEST_ZONE_NAME:     the zone to test against, e.g. "noah-hood.io." —
//     a trailing dot is appended if missing.
//   - PORKBUN_API_KEY:    Porkbun API key (consumed by solver.Initialize's
//     ambient-credential path).
//   - PORKBUN_SECRET_KEY: Porkbun secret key.
//
// The fixture requires envtest binaries (etcd, kube-apiserver, kubectl)
// located via the TEST_ASSET_ETCD, TEST_ASSET_KUBE_APISERVER, and
// TEST_ASSET_KUBECTL environment variables (cert-manager's test wrapper
// does NOT honor KUBEBUILDER_ASSETS); see the CI workflow or the
// `test-conformance` justfile recipe.
//
// If TEST_ZONE_NAME or the credentials are unset, the test is skipped.
func TestRunsSuite(t *testing.T) {
	zone := os.Getenv("TEST_ZONE_NAME")
	if zone == "" {
		t.Skip("TEST_ZONE_NAME not set; skipping DNS01 conformance suite")
	}
	if os.Getenv("PORKBUN_API_KEY") == "" || os.Getenv("PORKBUN_SECRET_KEY") == "" {
		t.Skip("PORKBUN_API_KEY / PORKBUN_SECRET_KEY not set; skipping DNS01 conformance suite")
	}

	// the fixture requires the resolved zone (and thus the derived FQDN) to
	// end with a dot
	if !strings.HasSuffix(zone, ".") {
		zone += "."
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)
	porkbunSolverInstance := solver.NewPorkbunSolver(logger, "porkbun-solver")

	fixture := dns.NewFixture(porkbunSolverInstance,
		dns.SetResolvedZone(zone),
		// the solver reads ambient credentials (env vars) in Initialize
		dns.SetAllowAmbientCredentials(true),
		// the solver ignores per-challenge config, but the fixture requires
		// a non-nil JSON config to pass validation
		dns.SetConfig(map[string]string{}),
		// default is 2m; give authoritative propagation some headroom
		dns.SetPropagationLimit(5*time.Minute),
		// strict mode additionally runs TestExtendedDeletingOneRecordRetainsOthers,
		// which verifies that two challenges can coexist at the same FQDN and
		// that cleaning one up retains the other — the apex + wildcard
		// certificate scenario. Requires content-aware create and
		// delete-by-ID semantics in the solver.
		dns.SetStrict(true),
	)

	fixture.RunConformance(t)
}
