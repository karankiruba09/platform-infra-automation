# Alpine Pod with AVI LoadBalancer Setup

## Overview
This setup creates:
1. A Pod running Alpine Linux with a simple HTTP server
2. A LoadBalancer Service that integrates with AVI (NSX Advanced Load Balancer) using a static IP

## Prerequisites
- Kubernetes cluster with AKO (AVI Kubernetes Operator) installed and configured
- kubectl access to the cluster
- A static IP address available in the subnet configured in AKO
- AVI Controller accessible and configured
- AKO configured with appropriate network, VIP network, and subnet settings

## Files
- `alpine-pod.yaml` - Pod definition with Alpine container
- `alpine-service.yaml` - LoadBalancer Service definition
- `deploy.sh` - Automated deployment script

## Deployment Steps

### Option 1: Using the deployment script

1. **Edit the Service manifest** to add your static IP:
   ```bash
   # Open alpine-service.yaml and replace YOUR_STATIC_IP_HERE with your actual IP
   nano alpine-service.yaml
   ```

2. **Run the deployment script**:
   ```bash
   ./deploy.sh
   ```

### Option 2: Manual deployment

1. **Update the static IP** in `alpine-service.yaml`:
   ```yaml
   loadBalancerIP: "192.168.1.100"  # Your actual static IP
   ```

2. **Deploy the Pod**:
   ```bash
   kubectl apply -f alpine-pod.yaml
   ```

3. **Verify Pod is running**:
   ```bash
   kubectl get pods -l app=alpine-app
   kubectl logs alpine-pod
   ```

4. **Deploy the Service**:
   ```bash
   kubectl apply -f alpine-service.yaml
   ```

5. **Check Service status**:
   ```bash
   kubectl get svc alpine-loadbalancer
   kubectl describe svc alpine-loadbalancer
   ```

## Verifying AVI Integration

### Check LoadBalancer Status
```bash
# Watch for External IP assignment
kubectl get svc alpine-loadbalancer -w

# Get detailed service information
kubectl describe svc alpine-loadbalancer
```

### Verify in AVI Controller

1. Log into your AVI Controller UI
2. Navigate to **Applications** > **Virtual Services**
3. Find the Virtual Service named after your service (usually `<namespace>-alpine-loadbalancer`)
4. Check the health status - it should show **Green** with:
   - Pool status: UP
   - Pool members: 1/1 UP

### Common AKO Annotations

The service uses AKO (AVI Kubernetes Operator) annotations. Key annotations available:

```yaml
metadata:
  annotations:
    # Enable AVI Virtual Service (required for AKO)
    ako.vmware.com/enable-virtual-service: "true"
    
    # Static IP assignment (required for your use case)
    ako.vmware.com/load-balancer-ip: "192.168.1.100"
    
    # Application profile (optional, default is System-L4-Application)
    ako.vmware.com/application-profile: "System-HTTP"
    
    # Network profile (optional)
    ako.vmware.com/network-profile: "System-TCP-Proxy"
    
    # Health monitor type (optional, default is TCP)
    ako.vmware.com/health-monitor-type: "HTTP"
    
    # Health monitor path for HTTP checks (optional)
    ako.vmware.com/health-monitor-path: "/"
    
    # Custom health monitor ports (optional)
    ako.vmware.com/health-monitor-port: "8080"
```

### AKO-Specific Notes

- AKO must be installed and running in your cluster
- The static IP must be from a network/subnet configured in AKO's values.yaml
- AKO creates CRDs like `HostRule` and `HTTPRule` for advanced configurations
- Check AKO pod logs for Virtual Service creation: `kubectl logs -n avi-system <ako-pod>`

## Testing the Deployment

Once the LoadBalancer IP is assigned:

```bash
# Get the External IP
EXTERNAL_IP=$(kubectl get svc alpine-loadbalancer -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

# Test connectivity
curl http://$EXTERNAL_IP

# Expected output: Directory listing from Python HTTP server
```

## Troubleshooting

### Check AKO Installation
```bash
# Verify AKO is running
kubectl get pods -n avi-system

# Check AKO logs
kubectl logs -n avi-system -l app=ako -f

# Verify AKO configuration
kubectl get configmap -n avi-system avi-k8s-config -o yaml
```

### LoadBalancer stuck in Pending
```bash
# Check service events
kubectl describe svc alpine-loadbalancer

# Check AKO logs for errors
kubectl logs -n avi-system -l app=ako --tail=100

# Verify AKO can reach AVI Controller
# Check AKO configmap has correct controller IP and credentials

# Verify the static IP is in the correct subnet configured in AKO
```

### Health check shows Red/Yellow in AVI
```bash
# Check Pod status
kubectl get pods -l app=alpine-app
kubectl logs alpine-pod

# Verify port is listening in the container
kubectl exec alpine-pod -- netstat -tulpn | grep 8080

# Check service endpoints
kubectl get endpoints alpine-loadbalancer

# Verify AKO health monitor configuration
kubectl describe svc alpine-loadbalancer | grep -i health
```

### Static IP not being assigned
- Verify the IP is within the range configured in AKO's values.yaml (subnetIP/subnetPrefix)
- Check that the IP is not already in use
- Ensure AKO has the correct VIP network configured
- Review AKO pod logs: `kubectl logs -n avi-system -l app=ako`
- Verify AVI Controller has the IP pool configured correctly

### AKO not creating Virtual Service
```bash
# Check if AKO is watching the correct namespace
kubectl get configmap -n avi-system avi-k8s-config -o yaml | grep namespaceSelector

# Verify service has correct annotations
kubectl get svc alpine-loadbalancer -o yaml | grep ako.vmware.com

# Check AKO can authenticate to AVI Controller
kubectl logs -n avi-system -l app=ako | grep -i "logged in"
```

## Cleanup

To remove all resources:

```bash
kubectl delete -f alpine-service.yaml
kubectl delete -f alpine-pod.yaml
```

## Notes

- The Alpine container runs a simple Python HTTP server on port 8080
- The Service exposes this on port 80 via the LoadBalancer
- AVI will create a Virtual Service and pool automatically
- Health checks are performed by AVI based on the configured monitor
- The static IP must be pre-allocated in your AVI IP pool
