# Deploying to K3s on Proxmox VMs

This guide walks through deploying the cert-manager Porkbun webhook to a K3s cluster running on VMs hosted on Proxmox.

---

## Prerequisites

- Proxmox VE server with sufficient resources
- A domain managed by Porkbun (e.g., `noah-hood.io`)
- Porkbun API credentials (API Key and Secret Key)
- Basic familiarity with Proxmox, K3s, and kubectl

---

## Part 1: Setting Up Proxmox VMs

### VM Specifications

For a basic K3s cluster, create 1-3 VMs:

| Role | vCPUs | RAM | Disk | Recommended |
|------|-------|-----|------|-------------|
| Control Plane | 2 | 4GB | 32GB | 1 node |
| Worker | 2 | 4GB | 32GB | 0-2 nodes |

For a single-node setup (simplest), one VM with 4 vCPUs and 8GB RAM works well.

### Create VMs in Proxmox

1. **Download an ISO** (Ubuntu Server 22.04 or Debian 12 recommended)
   - Datacenter → Storage → ISO Images → Download from URL

2. **Create VM**
   ```
   General:     Name: k3s-master-01
   OS:          Select your ISO
   System:      QEMU Agent enabled
   Disks:       32GB, VirtIO Block
   CPU:         2 cores, type: host
   Memory:      4096MB
   Network:     VirtIO, bridge to your LAN
   ```

3. **Install OS**
   - Minimal server installation
   - Set static IP or use DHCP reservation
   - Enable SSH

4. **Post-install setup** (on each VM):
   ```bash
   # Update system
   sudo apt update && sudo apt upgrade -y

   # Install useful tools
   sudo apt install -y curl wget git qemu-guest-agent

   # Enable guest agent
   sudo systemctl enable --now qemu-guest-agent
   ```

---

## Part 2: Installing K3s

### Single Node Cluster (Simplest)

SSH into your VM and run:

```bash
curl -sfL https://get.k3s.io | sh -
```

Wait for K3s to start:
```bash
sudo systemctl status k3s
```

Get the kubeconfig:
```bash
sudo cat /etc/rancher/k3s/k3s.yaml
```

Copy this to your local machine as `~/.kube/config` (update the `server` address to your VM's IP).

### Multi-Node Cluster

**On the first node (control plane):**
```bash
curl -sfL https://get.k3s.io | sh -s - server --cluster-init
```

Get the node token:
```bash
sudo cat /var/lib/rancher/k3s/server/node-token
```

**On additional control plane nodes:**
```bash
curl -sfL https://get.k3s.io | sh -s - server \
  --server https://<first-node-ip>:6443 \
  --token <node-token>
```

**On worker nodes:**
```bash
curl -sfL https://get.k3s.io | sh -s - agent \
  --server https://<control-plane-ip>:6443 \
  --token <node-token>
```

### Verify Cluster

```bash
kubectl get nodes
```

Expected output:
```
NAME            STATUS   ROLES                  AGE   VERSION
k3s-master-01   Ready    control-plane,master   5m    v1.28.x+k3s1
```

---

## Part 3: Installing Cert-Manager

K3s comes with Traefik as the default ingress controller, but we need cert-manager for certificate management.

### Install cert-manager via Helm

```bash
# Install Helm if not present
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Add Jetstack repo
helm repo add jetstack https://charts.jetstack.io
helm repo update

# Install cert-manager
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true

# Verify installation
kubectl get pods -n cert-manager
```

Wait for all pods to be Running:
```
cert-manager-xxxxxxxxx-xxxxx              1/1     Running
cert-manager-cainjector-xxxxxxxxx-xxxxx   1/1     Running
cert-manager-webhook-xxxxxxxxx-xxxxx      1/1     Running
```

---

## Part 4: Building and Pushing the Webhook Image

The webhook image is published to GitHub Container Registry (GHCR). You can either use a pre-built release or build and push your own.

### Using the Justfile (Recommended)

The project includes a `justfile` for common tasks. From your development machine:

```bash
# Enter the nix development shell (includes just, go, docker, etc.)
nix develop

# Build and push a release to GHCR
just release 0.1.0
```

This will:
1. Build the image for `linux/amd64`
2. Push to `ghcr.io/hoodnoah/certmanager-porkbun-webhook:0.1.0` and `:latest`
3. Create and push a git tag `v0.1.0`

For multi-architecture support (amd64 + arm64):
```bash
just release-multiarch 0.1.0
```

### Manual Build and Push

If you prefer manual commands:

```bash
# Authenticate with GHCR
echo $GITHUB_TOKEN | docker login ghcr.io -u hoodnoah --password-stdin

# Build and push
docker buildx build \
    --platform linux/amd64 \
    -t ghcr.io/hoodnoah/certmanager-porkbun-webhook:0.1.0 \
    -t ghcr.io/hoodnoah/certmanager-porkbun-webhook:latest \
    --push .
```

### Update Deployment Manifest

Update `manifests/webhook-deployment.yaml` to use the GHCR image:

```yaml
spec:
  template:
    spec:
      containers:
        - name: porkbun-webhook
          image: ghcr.io/hoodnoah/certmanager-porkbun-webhook:latest
          imagePullPolicy: Always
```

Since the repository is public, the GHCR package is also public—no image pull secrets are required. K3s can pull the image directly.

---

## Part 5: Deploying the Webhook

### Create Porkbun Credentials Secret

```bash
# Encode your credentials
echo -n "pk1_your_api_key_here" | base64
echo -n "sk1_your_secret_key_here" | base64
```

Edit `secret/porkbun-credentials.yaml` with your encoded values, then apply:

```bash
kubectl apply -f secret/porkbun-credentials.yaml
```

### Apply Manifests

Apply in order:

```bash
# 1. PKI infrastructure
kubectl apply -f manifests/pki.yaml

# Wait for certificates
kubectl get certificates -n cert-manager
# Should show READY=True for porkbun-webhook-ca and porkbun-webhook-webhook-tls

# 2. RBAC
kubectl apply -f manifests/rbac.yaml

# 3. Service
kubectl apply -f manifests/webhook-service.yaml

# 4. Deployment
kubectl apply -f manifests/webhook-deployment.yaml

# Wait for pod
kubectl get pods -n cert-manager -l app=porkbun-webhook
# Should show 1/1 Running

# 5. API Service
kubectl apply -f manifests/apiservice.yaml

# Verify API service
kubectl get apiservices | grep acme
# Should show True
```

### Create ClusterIssuers

```bash
# Staging (for testing)
kubectl apply -f manifests/clusterissuer-staging.yaml

# Production (for real certificates)
kubectl apply -f manifests/clusterissuer.yaml

# Verify
kubectl get clusterissuers
```

---

## Part 6: Using Certificates with Your Applications

### Method 1: Certificate Resource

Create a Certificate that will be stored in a Secret:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: my-app-tls
  namespace: default
spec:
  secretName: my-app-tls-secret
  issuerRef:
    name: letsencrypt-prod  # or letsencrypt-staging for testing
    kind: ClusterIssuer
  dnsNames:
    - "app.yourdomain.com"
    - "api.yourdomain.com"
```

Then reference the secret in your Ingress or application.

### Method 2: Ingress Annotation (Automatic)

With Traefik (K3s default), add an annotation to auto-generate certificates:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-app-ingress
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
spec:
  tls:
    - hosts:
        - app.yourdomain.com
      secretName: my-app-tls-secret
  rules:
    - host: app.yourdomain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: my-app
                port:
                  number: 80
```

Cert-manager will automatically:
1. Detect the annotation
2. Create a Certificate resource
3. Perform DNS-01 validation via the Porkbun webhook
4. Store the certificate in `my-app-tls-secret`
5. Traefik will use this certificate for HTTPS

---

## Part 7: Exposing Services to the Internet

### Option A: Port Forwarding (Simple)

On your router, forward ports 80 and 443 to your K3s node's IP.

### Option B: Cloudflare Tunnel (No Port Forwarding)

If you don't want to expose ports:

```bash
# Install cloudflared
curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o cloudflared
chmod +x cloudflared
sudo mv cloudflared /usr/local/bin/

# Authenticate
cloudflared tunnel login

# Create tunnel
cloudflared tunnel create k3s-tunnel

# Configure tunnel (create config.yaml)
cat > ~/.cloudflared/config.yaml <<EOF
tunnel: <tunnel-id>
credentials-file: /root/.cloudflared/<tunnel-id>.json
ingress:
  - hostname: app.yourdomain.com
    service: http://localhost:80
  - service: http_status:404
EOF

# Run tunnel
cloudflared tunnel run k3s-tunnel
```

Note: With Cloudflare, you can use HTTP-01 challenges instead of DNS-01 since Cloudflare proxies the traffic.

### Option C: Tailscale/WireGuard (Private Access)

For private/internal services, use a VPN mesh like Tailscale.

---

## Part 8: Complete Example

Here's a full example deploying a simple web app with TLS:

### 1. Deploy an Application

```yaml
# app.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hello-world
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: hello-world
  template:
    metadata:
      labels:
        app: hello-world
    spec:
      containers:
        - name: hello-world
          image: nginx:alpine
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: hello-world
  namespace: default
spec:
  selector:
    app: hello-world
  ports:
    - port: 80
      targetPort: 80
```

### 2. Create Ingress with TLS

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: hello-world-ingress
  namespace: default
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-staging"  # Use staging first!
spec:
  tls:
    - hosts:
        - hello.yourdomain.com
      secretName: hello-world-tls
  rules:
    - host: hello.yourdomain.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: hello-world
                port:
                  number: 80
```

### 3. Apply and Verify

```bash
kubectl apply -f app.yaml
kubectl apply -f ingress.yaml

# Watch certificate progress
kubectl get certificate -w

# Check events
kubectl describe certificate hello-world-tls
```

### 4. Switch to Production

Once staging works, update the annotation to `letsencrypt-prod` and delete the staging certificate:

```bash
kubectl delete certificate hello-world-tls
kubectl delete secret hello-world-tls
# Update ingress annotation, then reapply
kubectl apply -f ingress.yaml
```

---

## Troubleshooting

### DNS Resolution

Ensure your domain points to your cluster:
```bash
dig A app.yourdomain.com
```

For internal/home networks, you might need:
- Split-horizon DNS
- Local DNS server (Pi-hole, AdGuard)
- Host file entries for testing

### Certificate Not Issuing

```bash
# Check certificate status
kubectl describe certificate <name>

# Check challenges
kubectl get challenges -A
kubectl describe challenge <name>

# Check webhook logs
kubectl logs -n cert-manager -l app=porkbun-webhook

# Check cert-manager logs
kubectl logs -n cert-manager -l app=cert-manager
```

### Webhook Not Responding

```bash
# Test API endpoint
kubectl get --raw "/apis/acme.noah-hood.io/v1alpha1"

# Check API service
kubectl get apiservices v1alpha1.acme.noah-hood.io -o yaml
```

---

## Production Checklist

- [ ] K3s cluster running with sufficient resources
- [ ] cert-manager installed and healthy
- [ ] Webhook image pushed to GHCR (`just release <version>`)
- [ ] `webhook-deployment.yaml` updated with GHCR image reference
- [ ] Porkbun credentials secret created
- [ ] All webhook manifests applied
- [ ] API service showing `True`
- [ ] Staging ClusterIssuer tested successfully
- [ ] Production ClusterIssuer created
- [ ] DNS records pointing to cluster
- [ ] Ingress controller configured
- [ ] Firewall/port forwarding configured (if needed)

---

## Justfile Quick Reference

The project includes a `justfile` with helpful commands. Run `just help` to see all available commands.

| Command | Description |
|---------|-------------|
| `just release <version>` | Build and push image to GHCR (linux/amd64) |
| `just release-multiarch <version>` | Build and push for amd64 + arm64 |
| `just build` | Build image locally |
| `just deploy` | Deploy all manifests to current cluster |
| `just undeploy` | Remove webhook from cluster |
| `just logs` | Tail webhook logs |
| `just status` | Show webhook, API service, and certificate status |
| `just test-api` | Test the webhook API endpoint |
| `just challenges` | List all ACME challenges |
| `just check-dns <domain>` | Check DNS propagation for a domain |

---

## Resource Links

- [K3s Documentation](https://docs.k3s.io/)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [Porkbun API Documentation](https://porkbun.com/api/json/v3/documentation)
- [Traefik Documentation](https://doc.traefik.io/traefik/)
- [Let's Encrypt Rate Limits](https://letsencrypt.org/docs/rate-limits/)
- [GitHub Container Registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
