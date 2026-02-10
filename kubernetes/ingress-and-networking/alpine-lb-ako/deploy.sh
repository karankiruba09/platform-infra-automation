#!/bin/bash
# Deployment script for Alpine Pod with AVI LoadBalancer

echo "=== Deploying Alpine Pod with AVI LoadBalancer ==="

# Step 1: Update the static IP in the service manifest
echo ""
echo "IMPORTANT: Before proceeding, edit alpine-service.yaml and replace 'YOUR_STATIC_IP_HERE' with your actual static IP address"
echo ""
read -p "Press Enter after you've updated the static IP, or Ctrl+C to exit..."

# Step 2: Apply the Pod
echo ""
echo "Creating Alpine Pod..."
kubectl apply -f alpine-pod.yaml

# Step 3: Wait for Pod to be ready
echo ""
echo "Waiting for Pod to be ready..."
kubectl wait --for=condition=Ready pod/alpine-pod --timeout=60s

# Step 4: Apply the Service
echo ""
echo "Creating LoadBalancer Service..."
kubectl apply -f alpine-service.yaml

# Step 5: Check the status
echo ""
echo "Waiting for LoadBalancer to be provisioned by AVI..."
sleep 5

echo ""
echo "=== Deployment Status ==="
kubectl get pods -l app=alpine-app
echo ""
kubectl get svc alpine-loadbalancer

echo ""
echo "=== Checking LoadBalancer External IP ==="
kubectl get svc alpine-loadbalancer -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
echo ""

echo ""
echo "=== To monitor the service in AVI ==="
echo "1. Log into your AVI Controller"
echo "2. Navigate to Applications > Virtual Services"
echo "3. Look for the Virtual Service corresponding to alpine-loadbalancer"
echo "4. Verify it shows 'Green' health status"
echo ""
echo "=== To test the service ==="
echo "Once the LoadBalancer IP is assigned, test with:"
echo "curl http://<LOADBALANCER-IP>"
