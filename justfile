set shell := ["bash", "-euo", "pipefail", "-c"]

# Cert-Manager Porkbun Webhook - Justfile
# =============================================================================
# Configuration
# =============================================================================
# Registry and repository settings

github_user := "hoodnoah"
repo_name := "certmanager-porkbun-webhook"
registry := "ghcr.io"
repo := github_user + "/" + repo_name
image_name := "porkbun-webhook"

# Kubernetes settings

namespace := "cert-manager"
app_label := "porkbun-webhook"

# =============================================================================
# Development - Minikube
# =============================================================================

# Start minikube cluster
minikube-start:
    minikube start

# Stop minikube cluster
minikube-stop:
    minikube stop

# Delete minikube cluster
minikube-delete:
    minikube delete

# Build image directly into minikube's Docker daemon
build-minikube:
    #!/usr/bin/env bash
    eval $(minikube docker-env --shell bash)
    docker build -t {{ image_name }}:latest .
    echo "Image built and available in minikube"

# =============================================================================
# Building & Releasing
# =============================================================================

# Build image locally
build:
    docker build -t {{ image_name }}:latest .

# Build and push to GHCR (usage: just release 0.1.0)
release version:
    echo "Building webhook image for linux/amd64..."
    docker buildx build \
        --platform linux/amd64 \
        -f Dockerfile \
        -t {{ registry }}/{{ repo }}:{{ version }} \
        -t {{ registry }}/{{ repo }}:latest \
        --push .

    echo "Tagging release in git..."
    git tag -a {{ version }} -m "Release {{ version }}"
    git push origin {{ version }}

    echo "Released {{ registry }}/{{ repo }}:{{ version }}"

# Build multi-arch and push to GHCR (usage: just release-multiarch 0.1.0)
release-multiarch version:
    echo "Building webhook image for linux/amd64 and linux/arm64..."
    docker buildx build \
        --platform linux/amd64,linux/arm64 \
        -f Dockerfile \
        -t {{ registry }}/{{ repo }}:{{ version }} \
        -t {{ registry }}/{{ repo }}:latest \
        --push .

    echo "Tagging release in git..."
    git tag -a {{ version }} -m "Release {{ version }}"
    git push origin {{ version }}

    echo "Released {{ registry }}/{{ repo }}:{{ version }} (multi-arch)"

# =============================================================================
# Kubernetes Deployment
# =============================================================================

# Install cert-manager via Helm
install-cert-manager:
    helm repo add jetstack https://charts.jetstack.io
    helm repo update
    helm install cert-manager jetstack/cert-manager \
        --namespace {{ namespace }} \
        --create-namespace \
        --set crds.enabled=true
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=cert-manager -n {{ namespace }} --timeout=120s

# Deploy all webhook manifests (for minikube with local image)
deploy-local: build-minikube
    echo "Applying manifests..."
    kubectl apply -f manifests/pki.yaml
    kubectl wait --for=condition=ready certificate -n {{ namespace }} porkbun-webhook-ca porkbun-webhook-webhook-tls --timeout=60s
    kubectl apply -f manifests/rbac.yaml
    kubectl apply -f manifests/webhook-service.yaml
    kubectl apply -f manifests/webhook-deployment.yaml
    kubectl wait --for=condition=ready pod -l app={{ app_label }} -n {{ namespace }} --timeout=90s
    kubectl apply -f manifests/apiservice.yaml
    echo "Webhook deployed successfully"

# Deploy all webhook manifests (for remote cluster with registry image)
deploy:
    echo "Applying manifests..."
    kubectl apply -f manifests/pki.yaml
    kubectl wait --for=condition=ready certificate -n {{ namespace }} porkbun-webhook-ca porkbun-webhook-webhook-tls --timeout=60s
    kubectl apply -f manifests/rbac.yaml
    kubectl apply -f manifests/webhook-service.yaml
    kubectl apply -f manifests/webhook-deployment.yaml
    kubectl wait --for=condition=ready pod -l app={{ app_label }} -n {{ namespace }} --timeout=90s
    kubectl apply -f manifests/apiservice.yaml
    echo "Webhook deployed successfully"

# Deploy ClusterIssuers
deploy-issuers:
    kubectl apply -f manifests/clusterissuer-staging.yaml
    kubectl apply -f manifests/clusterissuer.yaml

# Undeploy webhook (keeps cert-manager)
undeploy:
    -kubectl delete -f manifests/apiservice.yaml
    -kubectl delete -f manifests/webhook-deployment.yaml
    -kubectl delete -f manifests/webhook-service.yaml
    -kubectl delete -f manifests/rbac.yaml
    -kubectl delete -f manifests/pki.yaml

# Redeploy webhook (rebuild and restart)
redeploy: undeploy deploy-local

# Apply Porkbun credentials secret
apply-secret:
    kubectl apply -f secret/porkbun-credentials.yaml

# =============================================================================
# Testing
# =============================================================================

# Deploy test certificate (staging)
test-staging:
    kubectl apply -f manifests/test-certificate.yaml
    @echo "Watching certificate status..."
    kubectl get certificate -n test-porkbun -w

# Clean up test certificate
test-cleanup:
    -kubectl delete -f manifests/test-certificate.yaml
    -kubectl delete namespace test-porkbun

# Run full end-to-end test
test-e2e: deploy-local apply-secret deploy-issuers test-staging

# =============================================================================
# Debugging & Logs
# =============================================================================

# Tail webhook logs
logs:
    kubectl logs -n {{ namespace }} -l app={{ app_label }} -f

# Tail webhook logs (last 50 lines)
logs-tail:
    kubectl logs -n {{ namespace }} -l app={{ app_label }} --tail=50

# Tail cert-manager controller logs
logs-cm:
    kubectl logs -n {{ namespace }} -l app=cert-manager -f

# Describe webhook pod
describe:
    kubectl describe pod -n {{ namespace }} -l app={{ app_label }}

# Get webhook pod status
status:
    @echo "=== Webhook Pod ==="
    kubectl get pods -n {{ namespace }} -l app={{ app_label }}
    @echo ""
    @echo "=== API Service ==="
    kubectl get apiservices | grep acme
    @echo ""
    @echo "=== ClusterIssuers ==="
    kubectl get clusterissuers
    @echo ""
    @echo "=== Certificates ==="
    kubectl get certificates -A

# Test API endpoint
test-api:
    kubectl get --raw "/apis/acme.noah-hood.io/v1alpha1"

# Get all challenges
challenges:
    kubectl get challenges -A
    @echo ""
    @echo "For details: kubectl describe challenge <name> -n <namespace>"

# Get all orders
orders:
    kubectl get orders -A

# Shell into webhook pod
shell:
    kubectl exec -it -n {{ namespace }} $(kubectl get pods -n {{ namespace }} -l app={{ app_label }} -o jsonpath='{.items[0].metadata.name}') -- /bin/sh

# =============================================================================
# Utilities
# =============================================================================

# Check DNS propagation for a domain (usage: just check-dns test.noah-hood.io)
check-dns domain:
    dig TXT _acme-challenge.{{ domain }} +short

# Restart webhook deployment
restart:
    kubectl rollout restart deployment {{ app_label }} -n {{ namespace }}

# Get events in cert-manager namespace
events:
    kubectl get events -n {{ namespace }} --sort-by='.lastTimestamp'

# Port-forward webhook for local debugging
port-forward:
    kubectl port-forward -n {{ namespace }} svc/{{ app_label }} 8443:443

# =============================================================================
# Full Workflow Shortcuts
# =============================================================================

# Full setup: minikube + cert-manager + webhook + issuers
setup: minikube-start install-cert-manager apply-secret deploy-local deploy-issuers
    @echo "Setup complete! Run 'just test-staging' to test certificate issuance."

# Clean everything and start fresh
reset: minikube-delete setup

# Show available commands
help:
    @just --list
