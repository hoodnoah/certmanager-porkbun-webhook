# How the Cert-Manager Porkbun Webhook Works

This document explains the architecture and flow of the cert-manager Porkbun webhook for DNS-01 ACME challenges.

---

## Overview

When you want to obtain a TLS certificate from Let's Encrypt (or another ACME provider), you need to prove you control the domain. There are two main challenge types:

1. **HTTP-01**: Place a file at `http://yourdomain/.well-known/acme-challenge/`
2. **DNS-01**: Create a TXT record at `_acme-challenge.yourdomain`

DNS-01 is required for:
- Wildcard certificates (`*.yourdomain.com`)
- Domains without public HTTP access
- Internal/private domains

This webhook implements DNS-01 validation using the Porkbun DNS API.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                        │
│                                                                  │
│  ┌──────────────────┐      ┌──────────────────────────────────┐ │
│  │  Your Application │      │         cert-manager             │ │
│  │                   │      │  ┌────────────────────────────┐  │ │
│  │  Needs TLS cert   │─────▶│  │ Certificate Controller     │  │ │
│  │  for domain       │      │  │ - Watches Certificate CRs  │  │ │
│  └──────────────────┘      │  │ - Creates Orders/Challenges │  │ │
│                             │  └─────────────┬──────────────┘  │ │
│                             │                │                  │ │
│                             │                ▼                  │ │
│                             │  ┌────────────────────────────┐  │ │
│                             │  │ ACME Challenge Controller  │  │ │
│                             │  │ - Calls webhook via API    │  │ │
│                             │  └─────────────┬──────────────┘  │ │
│                             └────────────────┼─────────────────┘ │
│                                              │                   │
│                                              ▼                   │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                   Porkbun Webhook                          │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌──────────────────┐   │  │
│  │  │ API Server  │  │   Solver    │  │  Porkbun Client  │   │  │
│  │  │ (HTTPS:443) │─▶│  Present()  │─▶│  CreateDNS()     │───┼──┼──▶ Porkbun API
│  │  │             │  │  CleanUp()  │  │  DeleteDNS()     │   │  │
│  │  └─────────────┘  └─────────────┘  └──────────────────┘   │  │
│  └───────────────────────────────────────────────────────────┘  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                         ┌─────────────────────┐
                         │    Porkbun DNS      │
                         │  _acme-challenge.   │
                         │  test.noah-hood.io  │
                         │  TXT "validation"   │
                         └─────────────────────┘
                                    │
                                    ▼
                         ┌─────────────────────┐
                         │   Let's Encrypt     │
                         │  Validates TXT      │
                         │  Issues Certificate │
                         └─────────────────────┘
```

---

## Components

### 1. Certificate Resource

You create a `Certificate` resource that declares what certificate you want:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-app-tls
  namespace: my-app
spec:
  secretName: my-app-tls-secret    # Where to store the cert
  issuerRef:
    name: letsencrypt-prod         # Which issuer to use
    kind: ClusterIssuer
  dnsNames:
    - "app.noah-hood.io"           # Domain(s) for the cert
    - "api.noah-hood.io"
```

### 2. ClusterIssuer

Defines how to obtain certificates from Let's Encrypt:

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: your-email@example.com
    privateKeySecretRef:
      name: letsencrypt-account-key
    solvers:
      - dns01:
          webhook:
            groupName: acme.noah-hood.io    # Webhook's API group
            solverName: porkbun-solver       # Solver name
        selector:
          dnsZones:
            - "noah-hood.io"                 # Which zones this handles
```

### 3. Porkbun Webhook

A Kubernetes deployment that:
- Runs as an HTTPS server on port 443
- Registers itself as a Kubernetes API extension
- Implements the cert-manager webhook Solver interface
- Communicates with Porkbun's DNS API

---

## Certificate Issuance Flow

### Step 1: Certificate Request

You create a `Certificate` resource. Cert-manager detects it and creates an `Order`.

```
Certificate (created) → Order (created) → Challenge (created)
```

### Step 2: Challenge Dispatch

Cert-manager determines which solver to use based on the `ClusterIssuer` configuration. For `noah-hood.io`, it routes to the Porkbun webhook.

### Step 3: Present (DNS Record Creation)

Cert-manager calls the webhook's `Present()` method via the Kubernetes API:

```
POST /apis/acme.noah-hood.io/v1alpha1/porkbun-solver

{
  "ResolvedFQDN": "_acme-challenge.app.noah-hood.io.",
  "ResolvedZone": "noah-hood.io.",
  "Key": "abc123-validation-token",
  ...
}
```

The webhook:
1. Extracts domain (`noah-hood.io`) and subdomain (`_acme-challenge.app`)
2. Calls Porkbun API to create TXT record
3. Returns success

### Step 4: Validation

Let's Encrypt queries public DNS for the TXT record:

```
dig TXT _acme-challenge.app.noah-hood.io
```

If the record matches the expected value, validation succeeds.

### Step 5: Certificate Issuance

Let's Encrypt issues the certificate. Cert-manager stores it in the specified Secret.

### Step 6: CleanUp (DNS Record Deletion)

Cert-manager calls the webhook's `CleanUp()` method to remove the TXT record:

```
POST /apis/acme.noah-hood.io/v1alpha1/porkbun-solver (cleanup)
```

---

## Webhook Implementation Details

### Solver Interface

The webhook implements cert-manager's `webhook.Solver` interface:

```go
type Solver interface {
    Name() string
    Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error
    Present(challenge *ChallengeRequest) error
    CleanUp(challenge *ChallengeRequest) error
}
```

### Initialize()

Called once at startup:
1. Creates Kubernetes client
2. Fetches Porkbun credentials from `porkbun-credentials` Secret
3. Configures the Porkbun API client

### Present()

Called when a challenge needs to be created:
1. Receives challenge with FQDN, zone, and validation key
2. Parses domain/subdomain from FQDN using zone
3. Creates TXT record via Porkbun API

### CleanUp()

Called after challenge validation:
1. Receives same challenge information
2. Deletes TXT record via Porkbun API

---

## Kubernetes Resources

### APIService

Registers the webhook as a Kubernetes API extension:

```yaml
apiVersion: apiregistration.k8s.io/v1
kind: APIService
metadata:
  name: v1alpha1.acme.noah-hood.io
spec:
  group: acme.noah-hood.io
  version: v1alpha1
  service:
    name: porkbun-webhook
    namespace: cert-manager
```

This tells Kubernetes: "Route requests to `/apis/acme.noah-hood.io/v1alpha1/*` to the `porkbun-webhook` service."

### RBAC

The webhook needs permissions to:
- Read authentication ConfigMaps (for API authentication)
- Delegate authentication to the API server
- Read the `porkbun-credentials` Secret

Cert-manager needs permission to:
- Create resources in the `acme.noah-hood.io` API group

### PKI (TLS Certificates)

The webhook runs over HTTPS. Cert-manager manages the webhook's own TLS certificates:

```
Self-Signed Issuer
       │
       ▼
    Root CA
       │
       ▼
   CA Issuer
       │
       ▼
Webhook TLS Cert
```

---

## Porkbun API Integration

The webhook uses a custom Porkbun Go client (`github.com/hoodnoah/porkbun/pkg`).

### Authentication

Porkbun requires two credentials:
- **API Key**: `pk1_...`
- **Secret Key**: `sk1_...`

These are stored in a Kubernetes Secret and loaded at startup.

### DNS Operations

**Create TXT Record:**
```
POST https://porkbun.com/api/json/v3/dns/createByNameType/{domain}/TXT/{subdomain}
{
  "apikey": "pk1_...",
  "secretapikey": "sk1_...",
  "content": "validation-token"
}
```

**Delete TXT Record:**
```
POST https://porkbun.com/api/json/v3/dns/deleteByNameType/{domain}/TXT/{subdomain}
```

---

## Troubleshooting

### Check Certificate Status
```bash
kubectl get certificate -A
kubectl describe certificate <name> -n <namespace>
```

### Check Challenges
```bash
kubectl get challenges -A
kubectl describe challenge <name> -n <namespace>
```

### Check Webhook Logs
```bash
kubectl logs -n cert-manager -l app=porkbun-webhook
```

### Check API Service
```bash
kubectl get apiservices | grep acme
kubectl get --raw "/apis/acme.noah-hood.io/v1alpha1"
```

### Verify DNS Record
```bash
dig TXT _acme-challenge.yourdomain.com +short
```

---

## Common Issues

| Symptom | Cause | Solution |
|---------|-------|----------|
| APIService shows `False` | Service not in correct namespace | Add `namespace: cert-manager` to service |
| "secrets forbidden" error | Missing RBAC for secret access | Add Role/RoleBinding for secret-reader |
| "Invalid domain" from Porkbun | Wrong domain parsing | Use `ResolvedZone` for correct splitting |
| Challenge stuck "pending" | DNS not propagated | Wait for propagation (up to 5 min) |
| Webhook CrashLoopBackOff | TLS cert paths wrong | Use `tls.crt`/`tls.key` not `cert.pem`/`key.pem` |
