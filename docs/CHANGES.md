# Changes Made During Testing

This document describes the fixes and improvements made to get the cert-manager Porkbun webhook working end-to-end.

## Summary

The webhook was successfully tested against Let's Encrypt Staging, issuing a certificate for `test.noah-hood.io`. Several bugs in manifests and code were discovered and fixed.

---

## Manifest Fixes

### `manifests/webhook-deployment.yaml`

**Issues:**
1. Missing `namespace` field - deployment was created in `default` instead of `cert-manager`
2. `volumes` field had incorrect indentation (was under `template` instead of `template.spec`)
3. TLS certificate paths didn't match the secret keys created by cert-manager

**Changes:**
```yaml
# Added namespace
metadata:
  name: porkbun-webhook
  namespace: cert-manager  # Added

# Fixed volumes indentation (moved under spec, same level as containers)
spec:
  template:
    spec:
      containers: [...]
      volumes:        # Was incorrectly at template level
        - name: certs
          secret:
            secretName: porkbun-webhook-webhook-tls

# Fixed TLS paths
args:
  - --tls-cert-file=/tls/tls.crt      # Was: /tls/cert.pem
  - --tls-private-key-file=/tls/tls.key  # Was: /tls/key.pem
```

---

### `manifests/webhook-service.yaml`

**Issue:** Missing `namespace` field - service was created in `default` instead of `cert-manager`

**Change:**
```yaml
metadata:
  name: porkbun-webhook
  namespace: cert-manager  # Added
```

---

### `manifests/rbac.yaml`

**Issues:**
1. ServiceAccount missing `namespace` field
2. No RBAC rules to allow the webhook to read the `porkbun-credentials` secret

**Changes:**
```yaml
# Added namespace to ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: porkbun-webhook
  namespace: cert-manager  # Added

# Added new Role and RoleBinding for secret access
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: porkbun-webhook:secret-reader
  namespace: cert-manager
rules:
  - apiGroups: [""]
    resources: ["secrets"]
    resourceNames: ["porkbun-credentials"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: porkbun-webhook:secret-reader
  namespace: cert-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: porkbun-webhook:secret-reader
subjects:
  - kind: ServiceAccount
    name: porkbun-webhook
    namespace: cert-manager
```

---

### `manifests/apiservice.yaml`

**Issue:** Missing `apiVersion` field

**Change:**
```yaml
apiVersion: apiregistration.k8s.io/v1  # Added
kind: APIService
```

---

## Code Fixes

### `internal/util/util.go`

**Issue:** The `SplitFQDN` function incorrectly parsed domain names by splitting on the first dot.

For `_acme-challenge.test.noah-hood.io`:
- **Before:** domain=`test.noah-hood.io`, subdomain=`_acme-challenge`
- **After:** domain=`noah-hood.io`, subdomain=`_acme-challenge.test`

**Change:** Added new `ExtractDomainAndSubdomain` function that uses the `ResolvedZone` from cert-manager:

```go
// ExtractDomainAndSubdomain extracts the domain and subdomain from an FQDN using the resolved zone.
func ExtractDomainAndSubdomain(fqdn, zone string) (domain, subdomain string) {
    fqdn = strings.TrimSuffix(fqdn, ".")
    zone = strings.TrimSuffix(zone, ".")

    domain = zone

    if strings.HasSuffix(fqdn, "."+zone) {
        subdomain = strings.TrimSuffix(fqdn, "."+zone)
    } else if fqdn == zone {
        subdomain = ""
    } else {
        subdomain = fqdn
    }

    return domain, subdomain
}
```

---

### `internal/solver/solver.go`

**Issue:** Used the broken `SplitFQDN` function instead of utilizing `challenge.ResolvedZone`.

**Change:** Updated `Present()` and `CleanUp()` methods:

```go
// Before
splitDomain := util.SplitFQDN(challenge.ResolvedFQDN)

// After
domain, subdomain := util.ExtractDomainAndSubdomain(challenge.ResolvedFQDN, challenge.ResolvedZone)
```

---

## New Files Created

### `manifests/clusterissuer-staging.yaml`

Let's Encrypt Staging issuer for safe testing without rate limits:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-staging
spec:
  acme:
    email: hood.noah@gmail.com
    server: https://acme-staging-v02.api.letsencrypt.org/directory
    privateKeySecretRef:
      name: letsencrypt-staging-account-key
    solvers:
      - dns01:
          webhook:
            groupName: acme.noah-hood.io
            solverName: porkbun-solver
        selector:
          dnsZones:
            - "noah-hood.io"
```

---

### `manifests/test-certificate.yaml`

Test certificate resource:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: test-porkbun
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: test-noah-hood-io
  namespace: test-porkbun
spec:
  secretName: test-noah-hood-io-tls
  issuerRef:
    name: letsencrypt-staging
    kind: ClusterIssuer
  dnsNames:
    - "test.noah-hood.io"
```

---

## Test Results

- **Certificate Issued:** `test.noah-hood.io`
- **Issuer:** Let's Encrypt Staging
- **Validity:** 2026-01-16 to 2026-04-16
- **DNS Record Creation:** Successful
- **DNS Record Cleanup:** Successful
