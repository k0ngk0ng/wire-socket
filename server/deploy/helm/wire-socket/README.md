# WireSocket Helm Chart

A Helm chart for deploying WireSocket VPN Server on Kubernetes.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- PV provisioner (if using persistence)

## Installation

### Add the repository (if published)

```bash
helm repo add wire-socket https://charts.example.com/wire-socket
helm repo update
```

### Install from local chart

```bash
# From the server/deploy/helm directory
helm install wire-socket ./wire-socket \
  --namespace wire-socket \
  --create-namespace \
  --set config.wireguard.endpoint="your-server-ip:51820" \
  --set config.auth.jwtSecret="your-strong-secret"
```

### Install with custom values

```bash
helm install wire-socket ./wire-socket \
  --namespace wire-socket \
  --create-namespace \
  -f my-values.yaml
```

## Configuration

See `values.yaml` for all available configuration options.

### Key Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.wireguard.endpoint` | Public endpoint for clients | `vpn.example.com:51820` |
| `config.wireguard.mode` | WireGuard mode (userspace/kernel) | `userspace` |
| `config.wireguard.subnet` | VPN subnet | `10.0.0.0/24` |
| `config.auth.jwtSecret` | JWT signing secret | `change-this-secret-in-production` |
| `config.tunnel.enabled` | Enable WebSocket tunnel | `true` |
| `service.type` | Service type | `LoadBalancer` |
| `persistence.enabled` | Enable data persistence | `true` |
| `persistence.size` | Storage size | `1Gi` |

### Example values.yaml

```yaml
config:
  wireguard:
    endpoint: "vpn.mycompany.com:51820"
    subnet: "10.100.0.0/24"
  auth:
    jwtSecret: "my-super-secret-key-change-this"

service:
  type: LoadBalancer
  annotations:
    # For AWS NLB
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"

persistence:
  enabled: true
  size: 5Gi
  storageClass: "gp2"

resources:
  limits:
    cpu: 1000m
    memory: 512Mi
  requests:
    cpu: 200m
    memory: 256Mi
```

## Post-Installation

### Initialize Database

After first installation, run the init-db job:

```bash
kubectl exec -it deploy/wire-socket -n wire-socket -- \
  /app/wire-socket-server -config /etc/wire-socket/config.yaml -init-db
```

### Get Service IP

```bash
kubectl get svc wire-socket -n wire-socket
```

## Upgrading

```bash
helm upgrade wire-socket ./wire-socket -n wire-socket -f my-values.yaml
```

## Uninstalling

```bash
helm uninstall wire-socket -n wire-socket
```

## Security Considerations

1. Always change the default `jwtSecret`
2. Use TLS for the API endpoint (via Ingress)
3. Consider using NetworkPolicies to restrict traffic
4. Store secrets in Kubernetes Secrets instead of ConfigMap for production
